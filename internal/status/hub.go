package status

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"nodestatus-go/internal/store"
)

const (
	maxHistoryPoints      = 1800
	historySampleInterval = 2 * time.Second
	historyFlushInterval  = 5 * time.Minute
)

type Options struct {
	Interval         time.Duration
	PingInterval     time.Duration
	ReconnectTimeout time.Duration
}

type ServerItem struct {
	ID         int            `json:"id"`
	Username   string         `json:"username"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Location   string         `json:"location"`
	Region     string         `json:"region"`
	Order      int            `json:"order"`
	LastActive *int64         `json:"last_active,omitempty"`
	Status     map[string]any `json:"status"`
}

type Hub struct {
	store   *store.Store
	options Options

	upgrader websocket.Upgrader
	done     chan struct{}

	mu              sync.Mutex
	servers         map[string]*ServerItem
	serversPub      []ServerItem
	conns           map[string]*websocket.Conn
	timers          map[string]*time.Timer
	bannedUntil     map[string]time.Time
	history         map[string][]store.BandwidthPoint
	resourceHistory map[string][]store.ResourcePoint
	traffic         map[string]trafficTotals
	aggregate       map[string]historyAggregate
	lastActive      map[string]int64
}

type authMessage struct {
	Username string `msgpack:"username"`
	Password string `msgpack:"password"`
}

type trafficTotals struct {
	rx float64
	tx float64
}

type historyAggregate struct {
	serverID                                    int
	time                                        int64
	cpu, memoryUsed, memoryTotal                float64
	networkIn, networkOut, networkRx, networkTx float64
	count                                       int
}

func NewHub(st *store.Store, opts Options) (*Hub, error) {
	if opts.Interval <= 0 {
		opts.Interval = 1500 * time.Millisecond
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = 30 * time.Second
	}
	if opts.ReconnectTimeout <= 0 {
		opts.ReconnectTimeout = 120 * time.Second
	}
	h := &Hub{
		store: st, options: opts, done: make(chan struct{}),
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		servers:  map[string]*ServerItem{}, conns: map[string]*websocket.Conn{}, timers: map[string]*time.Timer{},
		bannedUntil: map[string]time.Time{}, history: map[string][]store.BandwidthPoint{}, resourceHistory: map[string][]store.ResourcePoint{},
		traffic: map[string]trafficTotals{}, aggregate: map[string]historyAggregate{}, lastActive: map[string]int64{},
	}
	if err := h.RefreshAll(context.Background(), false); err != nil {
		return nil, err
	}
	go h.loop()
	return h, nil
}

func (h *Hub) Close() {
	close(h.done)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.conns {
		c.Close()
	}
	for _, t := range h.timers {
		t.Stop()
	}
}

func (h *Hub) ServersPub() []ServerItem {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]ServerItem{}, h.serversPub...)
}

func (h *Hub) ServeConnect(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	go h.handleAgent(conn, clientIP(r))
}

func (h *Hub) ServePublic(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	go func() {
		defer conn.Close()
		send := func() error {
			payload := map[string]any{"servers": h.ServersPub(), "updated": time.Now().Unix()}
			return conn.WriteJSON(payload)
		}
		if err := send(); err != nil {
			return
		}
		ticker := time.NewTicker(h.options.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-h.done:
				return
			case <-ticker.C:
				if err := send(); err != nil {
					return
				}
			}
		}
	}()
}

func (h *Hub) History(ctx context.Context, username string, rng int, metric Metric) (any, error) {
	since := time.Now().Add(-time.Duration(rng) * time.Second)
	if metric == MetricResource {
		stored, err := h.store.ReadResourceHistory(ctx, username, since)
		if err != nil {
			return nil, err
		}
		h.mu.Lock()
		memory := append([]store.ResourcePoint{}, h.resourceHistory[username]...)
		h.mu.Unlock()
		return DownsampleResource(MergeResource(stored, memory), rng), nil
	}
	stored, err := h.store.ReadBandwidthHistory(ctx, username, since)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	memory := append([]store.BandwidthPoint{}, h.history[username]...)
	h.mu.Unlock()
	return DownsampleBandwidth(MergeBandwidth(stored, memory), rng, metric), nil
}

func (h *Hub) RefreshAll(ctx context.Context, disconnect bool) error {
	servers, err := h.store.ListServers(ctx)
	if err != nil {
		return err
	}
	next := map[string]store.Server{}
	for _, srv := range servers {
		if !srv.Disabled {
			next[srv.Username] = srv
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for username, srv := range next {
		item := h.servers[username]
		if item == nil {
			item = &ServerItem{Username: username, Status: map[string]any{}}
			h.servers[username] = item
		}
		copyServer(item, srv)
		if last, ok := h.lastActive[username]; ok {
			item.LastActive = &last
		} else {
			item.LastActive = nil
		}
	}
	for username := range h.servers {
		if _, ok := next[username]; !ok {
			delete(h.servers, username)
			delete(h.history, username)
			delete(h.resourceHistory, username)
			delete(h.aggregate, username)
			delete(h.traffic, username)
			delete(h.lastActive, username)
		}
	}
	if disconnect {
		for username, conn := range h.conns {
			conn.Close()
			delete(h.conns, username)
		}
	}
	h.rebuildPublicLocked()
	return nil
}

func (h *Hub) RefreshServer(ctx context.Context, username string, disconnect bool) error {
	srv, err := h.store.GetServer(ctx, username)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if srv == nil || srv.Disabled {
		delete(h.servers, username)
		delete(h.history, username)
		delete(h.resourceHistory, username)
		delete(h.aggregate, username)
		delete(h.traffic, username)
		delete(h.lastActive, username)
	} else {
		item := h.servers[username]
		if item == nil {
			item = &ServerItem{Username: username, Status: map[string]any{}}
			h.servers[username] = item
		}
		copyServer(item, *srv)
		if last, ok := h.lastActive[username]; ok {
			item.LastActive = &last
		}
	}
	if disconnect {
		if conn := h.conns[username]; conn != nil {
			conn.Close()
			delete(h.conns, username)
		}
	}
	h.rebuildPublicLocked()
	return nil
}

func (h *Hub) handleAgent(conn *websocket.Conn, address string) {
	defer conn.Close()
	conn.WriteMessage(websocket.TextMessage, []byte("Authentication required"))
	_, buf, err := conn.ReadMessage()
	if err != nil {
		return
	}
	if h.isBanned(address) {
		conn.WriteMessage(websocket.TextMessage, []byte("You are banned. Please try connecting after 60 / 120 seconds"))
		return
	}
	var auth authMessage
	if err := msgpack.Unmarshal(buf, &auth); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Please check your login details."))
		h.ban(address, 120*time.Second)
		return
	}
	auth.Username = strings.TrimSpace(auth.Username)
	auth.Password = strings.TrimSpace(auth.Password)
	srv, err := h.store.GetServer(context.Background(), auth.Username)
	if err != nil || srv == nil || srv.Disabled || !store.ComparePassword(srv.Password, auth.Password) {
		conn.WriteMessage(websocket.TextMessage, []byte("Wrong username and/or password."))
		h.ban(address, 120*time.Second)
		return
	}
	conn.WriteMessage(websocket.TextMessage, []byte("Authentication successful. Access granted."))
	conn.WriteMessage(websocket.TextMessage, []byte("You are connecting via: "+ipType(address)))
	h.connected(auth.Username, conn)
	_ = h.store.ResolveEvent(context.Background(), auth.Username)
	for {
		_, buf, err := conn.ReadMessage()
		if err != nil {
			h.disconnected(auth.Username, conn)
			return
		}
		var payload map[string]any
		if err := msgpack.Unmarshal(buf, &payload); err == nil {
			h.updateStatus(auth.Username, payload)
		}
	}
}

func (h *Hub) connected(username string, conn *websocket.Conn) {
	h.mu.Lock()
	old := h.conns[username]
	if timer := h.timers[username]; timer != nil {
		timer.Stop()
		delete(h.timers, username)
	}
	h.conns[username] = conn
	h.mu.Unlock()
	if old != nil && old != conn {
		old.Close()
	}
}

func (h *Hub) disconnected(username string, conn *websocket.Conn) {
	h.mu.Lock()
	if h.conns[username] != conn {
		h.mu.Unlock()
		return
	}
	delete(h.conns, username)
	delete(h.traffic, username)
	if item := h.servers[username]; item != nil {
		item.Status = map[string]any{}
	}
	now := time.Now().UnixMilli()
	h.pushHistoryLocked(username, store.BandwidthPoint{Time: now})
	h.pushResourceLocked(username, store.ResourcePoint{Time: now})
	h.rebuildPublicLocked()
	timer := time.AfterFunc(h.options.ReconnectTimeout, func() {
		_ = h.store.CreateEvent(context.Background(), username)
		h.mu.Lock()
		delete(h.timers, username)
		h.mu.Unlock()
	})
	h.timers[username] = timer
	h.mu.Unlock()
}

func (h *Hub) updateStatus(username string, payload map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	item := h.servers[username]
	if item == nil {
		return
	}
	item.Status = payload
	last := time.Now().Unix()
	item.LastActive = &last
	h.lastActive[username] = last
	h.rebuildPublicLocked()
}

func (h *Hub) loop() {
	sample := time.NewTicker(historySampleInterval)
	flush := time.NewTicker(historyFlushInterval)
	defer sample.Stop()
	defer flush.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-sample.C:
			h.SampleHistory()
		case <-flush.C:
			h.FlushHistory()
		}
	}
}

func (h *Hub) SampleHistory() {
	now := time.Now().UnixMilli()
	h.mu.Lock()
	defer h.mu.Unlock()
	for username := range h.conns {
		item := h.servers[username]
		if item == nil || len(item.Status) == 0 {
			continue
		}
		if _, ok := item.Status["network_in"]; !ok {
			if _, ok := item.Status["network_out"]; !ok {
				if _, ok := item.Status["network_rx"]; !ok {
					if _, ok := item.Status["network_tx"]; !ok {
						continue
					}
				}
			}
		}
		traffic := h.trafficDelta(username, item.Status)
		point := store.BandwidthPoint{
			Time: now, In: fptr(number(item.Status["network_in"])), Out: fptr(number(item.Status["network_out"])),
			Rx: fptr(traffic.rx), Tx: fptr(traffic.tx),
		}
		h.pushHistoryLocked(username, point)
		h.pushResourceLocked(username, store.ResourcePoint{
			Time: now, CPU: fptr(number(item.Status["cpu"])), MemoryUsed: fptr(number(item.Status["memory_used"])),
			MemoryTotal: fptr(number(item.Status["memory_total"])), NetworkIn: point.In, NetworkOut: point.Out,
			NetworkRx: point.Rx, NetworkTx: point.Tx,
		})
		agg := h.aggregate[username]
		agg.serverID = item.ID
		agg.time = now
		agg.cpu += number(item.Status["cpu"])
		agg.memoryUsed += number(item.Status["memory_used"])
		agg.memoryTotal += number(item.Status["memory_total"])
		agg.networkIn += *point.In
		agg.networkOut += *point.Out
		agg.networkRx += *point.Rx
		agg.networkTx += *point.Tx
		agg.count++
		h.aggregate[username] = agg
	}
}

func (h *Hub) FlushHistory() {
	h.mu.Lock()
	inputs := []store.HistoryInput{}
	for username, item := range h.aggregate {
		if item.count == 0 || item.serverID == 0 {
			continue
		}
		count := float64(item.count)
		inputs = append(inputs, store.HistoryInput{
			ServerID: item.serverID, CreatedAt: time.UnixMilli(item.time),
			CPU: item.cpu / count, MemoryUsed: item.memoryUsed / count, MemoryTotal: item.memoryTotal / count,
			NetworkIn: item.networkIn / count, NetworkOut: item.networkOut / count, NetworkRx: item.networkRx, NetworkTx: item.networkTx,
		})
		delete(h.aggregate, username)
	}
	h.mu.Unlock()
	_ = h.store.CreateHistories(context.Background(), inputs)
}

func (h *Hub) pushHistoryLocked(username string, point store.BandwidthPoint) {
	h.history[username] = append(h.history[username], point)
	if len(h.history[username]) > maxHistoryPoints {
		h.history[username] = h.history[username][1:]
	}
}

func (h *Hub) pushResourceLocked(username string, point store.ResourcePoint) {
	h.resourceHistory[username] = append(h.resourceHistory[username], point)
	if len(h.resourceHistory[username]) > maxHistoryPoints {
		h.resourceHistory[username] = h.resourceHistory[username][1:]
	}
}

func (h *Hub) trafficDelta(username string, status map[string]any) trafficTotals {
	current := trafficTotals{rx: number(status["network_rx"]), tx: number(status["network_tx"])}
	prev, ok := h.traffic[username]
	h.traffic[username] = current
	if !ok {
		return trafficTotals{}
	}
	if current.rx < prev.rx {
		current.rx = prev.rx
	}
	if current.tx < prev.tx {
		current.tx = prev.tx
	}
	return trafficTotals{rx: current.rx - prev.rx, tx: current.tx - prev.tx}
}

func (h *Hub) rebuildPublicLocked() {
	h.serversPub = h.serversPub[:0]
	for _, item := range h.servers {
		b, _ := json.Marshal(item)
		var copy ServerItem
		_ = json.Unmarshal(b, &copy)
		h.serversPub = append(h.serversPub, copy)
	}
	sort.Slice(h.serversPub, func(i, j int) bool { return h.serversPub[i].Order > h.serversPub[j].Order })
}

func (h *Hub) isBanned(address string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	until, ok := h.bannedUntil[address]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(h.bannedUntil, address)
		return false
	}
	return true
}

func (h *Hub) ban(address string, d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bannedUntil[address] = time.Now().Add(d)
}

func copyServer(item *ServerItem, srv store.Server) {
	item.ID = srv.ID
	item.Username = srv.Username
	item.Name = srv.Name
	item.Type = srv.Type
	item.Location = srv.Location
	item.Region = srv.Region
	item.Order = srv.Order
	if item.Status == nil {
		item.Status = map[string]any{}
	}
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func ipType(address string) string {
	ip := net.ParseIP(address)
	if ip != nil && ip.To4() != nil {
		return "IPv4"
	}
	return "IPv6"
}

func number(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	default:
		return 0
	}
}

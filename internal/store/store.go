package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type Store struct {
	db    *sql.DB
	mu    sync.RWMutex
	order map[int]int
}

type Server struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
	Region   string `json:"region"`
	Disabled bool   `json:"disabled"`
	Order    int    `json:"order"`
}

type ServerInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
	Region   string `json:"region"`
	Disabled bool   `json:"disabled"`
}

type Event struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Resolved  bool   `json:"resolved"`
	CreatedAt any    `json:"created_at"`
	UpdatedAt any    `json:"updated_at"`
}

type BandwidthPoint struct {
	Time int64    `json:"time"`
	In   *float64 `json:"in"`
	Out  *float64 `json:"out"`
	Rx   *float64 `json:"rx,omitempty"`
	Tx   *float64 `json:"tx,omitempty"`
}

type ResourcePoint struct {
	Time        int64    `json:"time"`
	CPU         *float64 `json:"cpu"`
	MemoryUsed  *float64 `json:"memory_used"`
	MemoryTotal *float64 `json:"memory_total"`
	NetworkIn   *float64 `json:"network_in"`
	NetworkOut  *float64 `json:"network_out"`
	NetworkRx   *float64 `json:"network_rx"`
	NetworkTx   *float64 `json:"network_tx"`
}

type HistoryInput struct {
	ServerID    int
	CreatedAt   time.Time
	CPU         float64
	MemoryUsed  float64
	MemoryTotal float64
	NetworkIn   float64
	NetworkOut  float64
	NetworkRx   float64
	NetworkTx   float64
}

func Open(path string) (*Store, error) {
	path = sqlitePath(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, order: map[int]int{}}
	if err := s.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS servers (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			password TEXT NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			location TEXT NOT NULL,
			region TEXT NOT NULL,
			disabled BOOLEAN NOT NULL DEFAULT false,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS servers_username_key ON servers(username)`,
		`CREATE TABLE IF NOT EXISTS options (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS options_name_key ON options(name)`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			resolved BOOLEAN NOT NULL DEFAULT false,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS server_histories (
			id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			cpu REAL NOT NULL,
			memory_used REAL NOT NULL,
			memory_total REAL NOT NULL,
			network_in REAL NOT NULL,
			network_out REAL NOT NULL,
			network_rx REAL NOT NULL,
			network_tx REAL NOT NULL,
			CONSTRAINT server_histories_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE ON UPDATE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS server_histories_server_id_created_at_idx ON server_histories(server_id, created_at)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return s.reloadOrder(ctx)
}

func (s *Store) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, password, name, type, location, region, disabled FROM servers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var item Server
		if err := rows.Scan(&item.ID, &item.Username, &item.Password, &item.Name, &item.Type, &item.Location, &item.Region, &item.Disabled); err != nil {
			return nil, err
		}
		item.Order = s.orderFor(item.ID)
		servers = append(servers, item)
	}
	return servers, rows.Err()
}

func (s *Store) GetServer(ctx context.Context, username string) (*Server, error) {
	var item Server
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password, name, type, location, region, disabled FROM servers WHERE username = ?`, username).
		Scan(&item.ID, &item.Username, &item.Password, &item.Name, &item.Type, &item.Location, &item.Region, &item.Disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.Order = s.orderFor(item.ID)
	return &item, nil
}

func (s *Store) CreateServer(ctx context.Context, input ServerInput) error {
	if err := validateNewServer(input); err != nil {
		return err
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := dbTime(time.Now())
	res, err := tx.ExecContext(ctx, `INSERT INTO servers (username, password, name, type, location, region, disabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, input.Username, hash, input.Name, input.Type, input.Location, input.Region, input.Disabled, now, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ids := append(s.orderedIDs(), int(id))
	if err := setOrder(ctx, tx, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.setOrderCache(ids)
	return nil
}

func (s *Store) BulkCreateServers(ctx context.Context, inputs []ServerInput) error {
	if len(inputs) == 0 {
		return nil
	}
	hashes := make([]string, len(inputs))
	for i, input := range inputs {
		if err := validateNewServer(input); err != nil {
			return err
		}
		hash, err := hashPassword(input.Password)
		if err != nil {
			return err
		}
		hashes[i] = hash
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ids := s.orderedIDs()
	now := dbTime(time.Now())
	for i, input := range inputs {
		res, err := tx.ExecContext(ctx, `INSERT INTO servers (username, password, name, type, location, region, disabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, input.Username, hashes[i], input.Name, input.Type, input.Location, input.Region, input.Disabled, now, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		ids = append(ids, int(id))
	}
	if err := setOrder(ctx, tx, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.setOrderCache(ids)
	return nil
}

func (s *Store) UpdateServer(ctx context.Context, username string, data map[string]any) (newUsername string, disconnect bool, err error) {
	fields := []string{}
	args := []any{}
	addString := func(name string) {
		raw, ok := data[name]
		if !ok {
			return
		}
		value, ok := raw.(string)
		if !ok || value == "" {
			return
		}
		if name == "password" {
			var hash string
			hash, err = hashPassword(value)
			if err != nil {
				return
			}
			value = hash
			disconnect = true
		}
		if name == "username" {
			newUsername = value
			disconnect = true
		}
		fields = append(fields, name+" = ?")
		args = append(args, value)
	}
	for _, name := range []string{"username", "password", "name", "type", "location", "region"} {
		addString(name)
		if err != nil {
			return "", false, err
		}
	}
	if raw, ok := data["disabled"]; ok {
		if value, ok := raw.(bool); ok {
			fields = append(fields, "disabled = ?")
			args = append(args, value)
			if value {
				disconnect = true
			}
		}
	}
	if len(fields) == 0 {
		return "", false, nil
	}
	fields = append(fields, "updated_at = ?")
	args = append(args, dbTime(time.Now()), username)
	res, err := s.db.ExecContext(ctx, `UPDATE servers SET `+strings.Join(fields, ", ")+` WHERE username = ?`, args...)
	if err != nil {
		return "", false, err
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return "", false, sql.ErrNoRows
	}
	return newUsername, disconnect, err
}

func (s *Store) DeleteServer(ctx context.Context, username string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM servers WHERE username = ?`, username).Scan(&id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM servers WHERE username = ?`, username); err != nil {
		return err
	}
	ids := []int{}
	for _, current := range s.orderedIDs() {
		if current != id {
			ids = append(ids, current)
		}
	}
	if err := setOrder(ctx, tx, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.setOrderCache(ids)
	return nil
}

func (s *Store) UpdateOrder(ctx context.Context, ids []int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setOrder(ctx, tx, ids); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.setOrderCache(ids)
	return nil
}

func (s *Store) CreateEvent(ctx context.Context, username string) error {
	now := dbTime(time.Now())
	_, err := s.db.ExecContext(ctx, `INSERT INTO events (username, resolved, created_at, updated_at) VALUES (?, false, ?, ?)`, username, now, now)
	return err
}

func (s *Store) ResolveEvent(ctx context.Context, username string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE events SET resolved = true, updated_at = ? WHERE username = ? AND resolved = false`, dbTime(time.Now()), username)
	return err
}

func (s *Store) ListEvents(ctx context.Context, size, offset int) (int, []Event, error) {
	if size <= 0 {
		size = 10
	}
	if offset < 0 {
		offset = 0
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
		return 0, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, resolved, created_at, updated_at FROM events ORDER BY id DESC LIMIT ? OFFSET ?`, size, offset)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var item Event
		var created, updated any
		if err := rows.Scan(&item.ID, &item.Username, &item.Resolved, &created, &updated); err != nil {
			return 0, nil, err
		}
		item.CreatedAt = normalizeDBTime(created)
		item.UpdatedAt = normalizeDBTime(updated)
		events = append(events, item)
	}
	return count, events, rows.Err()
}

func (s *Store) DeleteEvent(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteAllEvents(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM events`)
	return err
}

func (s *Store) CreateHistories(ctx context.Context, items []HistoryInput) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO server_histories
		(server_id, created_at, cpu, memory_used, memory_total, network_in, network_out, network_rx, network_tx)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if _, err := stmt.ExecContext(ctx, item.ServerID, dbTime(item.CreatedAt), item.CPU, item.MemoryUsed, item.MemoryTotal, item.NetworkIn, item.NetworkOut, item.NetworkRx, item.NetworkTx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ReadBandwidthHistory(ctx context.Context, username string, since time.Time) ([]BandwidthPoint, error) {
	rows, err := s.historyRows(ctx, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cutoff := since.UnixMilli()
	points := []BandwidthPoint{}
	for rows.Next() {
		var raw any
		var cpu, memoryUsed, memoryTotal, networkIn, networkOut, networkRx, networkTx float64
		if err := rows.Scan(&raw, &cpu, &memoryUsed, &memoryTotal, &networkIn, &networkOut, &networkRx, &networkTx); err != nil {
			return nil, err
		}
		t := parseDBTimeMS(raw)
		if t < cutoff {
			continue
		}
		points = append(points, BandwidthPoint{Time: t, In: fptr(networkIn), Out: fptr(networkOut), Rx: fptr(networkRx), Tx: fptr(networkTx)})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Time < points[j].Time })
	return points, rows.Err()
}

func (s *Store) ReadResourceHistory(ctx context.Context, username string, since time.Time) ([]ResourcePoint, error) {
	rows, err := s.historyRows(ctx, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cutoff := since.UnixMilli()
	points := []ResourcePoint{}
	for rows.Next() {
		var raw any
		var cpu, memoryUsed, memoryTotal, networkIn, networkOut, networkRx, networkTx float64
		if err := rows.Scan(&raw, &cpu, &memoryUsed, &memoryTotal, &networkIn, &networkOut, &networkRx, &networkTx); err != nil {
			return nil, err
		}
		t := parseDBTimeMS(raw)
		if t < cutoff {
			continue
		}
		points = append(points, ResourcePoint{
			Time: t, CPU: fptr(cpu), MemoryUsed: fptr(memoryUsed), MemoryTotal: fptr(memoryTotal),
			NetworkIn: fptr(networkIn), NetworkOut: fptr(networkOut), NetworkRx: fptr(networkRx), NetworkTx: fptr(networkTx),
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Time < points[j].Time })
	return points, rows.Err()
}

func (s *Store) historyRows(ctx context.Context, username string) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, `SELECT h.created_at, h.cpu, h.memory_used, h.memory_total, h.network_in, h.network_out, h.network_rx, h.network_tx
		FROM server_histories h JOIN servers s ON s.id = h.server_id WHERE s.username = ? ORDER BY h.created_at ASC`, username)
}

func (s *Store) reloadOrder(ctx context.Context) error {
	var order string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM options WHERE name = 'order'`).Scan(&order)
	if errors.Is(err, sql.ErrNoRows) {
		s.setOrderCache(nil)
		return nil
	}
	if err != nil {
		return err
	}
	return s.setOrderString(order)
}

func (s *Store) setOrderString(order string) error {
	ids := []int{}
	for _, part := range strings.Split(order, ",") {
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	s.setOrderCache(ids)
	return nil
}

func (s *Store) setOrderCache(ids []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.order = map[int]int{}
	for i, id := range ids {
		s.order[id] = i + 1
	}
}

func (s *Store) orderFor(id int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if order := s.order[id]; order > 0 {
		return order
	}
	return id
}

func (s *Store) orderedIDs() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]int, 0, len(s.order))
	for id := range s.order {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return s.order[ids[i]] < s.order[ids[j]] })
	return ids
}

func setOrder(ctx context.Context, tx *sql.Tx, ids []int) error {
	value := joinIDs(ids)
	_, err := tx.ExecContext(ctx, `INSERT INTO options (name, value) VALUES ('order', ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value`, value)
	return err
}

func joinIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

func validateNewServer(input ServerInput) error {
	switch {
	case strings.TrimSpace(input.Username) == "":
		return errors.New("username cannot be empty")
	case strings.TrimSpace(input.Password) == "":
		return errors.New("password cannot be empty")
	case input.Name == "", input.Type == "", input.Location == "", input.Region == "":
		return errors.New("server fields cannot be empty")
	}
	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 8)
	return string(hash), err
}

func ComparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func sqlitePath(raw string) string {
	if raw == "" {
		return raw
	}
	if !strings.HasPrefix(raw, "file:") {
		return raw
	}
	u, err := url.Parse(raw)
	if err == nil && u.Path != "" {
		return u.Path
	}
	return strings.TrimPrefix(strings.Split(raw, "?")[0], "file:")
}

func dbTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000+00:00")
}

func normalizeDBTime(v any) any {
	switch x := v.(type) {
	case time.Time:
		return dbTime(x)
	case []byte:
		return string(x)
	default:
		return x
	}
}

func parseDBTimeMS(v any) int64 {
	switch x := normalizeDBTime(v).(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case string:
		if n, err := strconv.ParseFloat(x, 64); err == nil {
			return int64(n)
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t.UnixMilli()
			}
		}
	case time.Time:
		return x.UnixMilli()
	}
	return 0
}

func fptr(v float64) *float64 { return &v }

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
	"github.com/vmihailenco/msgpack/v5"
)

var version = "dev"

type identify struct {
	Username string `msgpack:"username"`
	Password string `msgpack:"password"`
}

type nodeStatus struct {
	Online4         bool     `msgpack:"online4"`
	Online6         bool     `msgpack:"online6"`
	Uptime          uint64   `msgpack:"uptime"`
	Load            float64  `msgpack:"load"`
	CPU             float64  `msgpack:"cpu"`
	NetworkRx       uint64   `msgpack:"network_rx"`
	NetworkTx       uint64   `msgpack:"network_tx"`
	NetworkIn       uint64   `msgpack:"network_in"`
	NetworkOut      uint64   `msgpack:"network_out"`
	MemoryTotal     uint64   `msgpack:"memory_total"`
	MemoryUsed      uint64   `msgpack:"memory_used"`
	SwapTotal       uint64   `msgpack:"swap_total"`
	SwapUsed        uint64   `msgpack:"swap_used"`
	HddTotal        uint64   `msgpack:"hdd_total"`
	HddUsed         uint64   `msgpack:"hdd_used"`
	Platform        string   `msgpack:"platform"`
	PlatformVersion string   `msgpack:"platform_version"`
	Arch            string   `msgpack:"arch"`
	Virtualization  string   `msgpack:"virtualization"`
	CPUInfo         []string `msgpack:"cpu_info"`
	GPUInfo         []string `msgpack:"gpu_info"`
	Version         string   `msgpack:"version"`
	Load1           float64  `msgpack:"load1"`
	Load5           float64  `msgpack:"load5"`
	Load15          float64  `msgpack:"load15"`
	TCPConnCount    uint64   `msgpack:"tcp_conn_count"`
	UDPConnCount    uint64   `msgpack:"udp_conn_count"`
	ProcessCount    uint64   `msgpack:"process_count"`
	Temperatures    float64  `msgpack:"temperatures"`
	GPU             float64  `msgpack:"gpu"`
	Custom          string   `msgpack:"custom"`
}

type config struct {
	server   string
	username string
	password string
	interval time.Duration
	custom   string
}

type staticInfo struct {
	platform        string
	platformVersion string
	arch            string
	virtualization  string
	cpuInfo         []string
}

type trafficCounter struct {
	prevIn  uint64
	prevOut uint64
	prevAt  time.Time
}

type diskCounter struct {
	mounts      map[string]struct{}
	nextRefresh time.Time
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		log.Fatal(err)
	}

	auth, err := msgpack.Marshal(&identify{Username: cfg.username, Password: cfg.password})
	if err != nil {
		log.Fatal(err)
	}

	info := collectStaticInfo()
	for {
		connect(cfg, info, auth)
		time.Sleep(5 * time.Second)
	}
}

func parseFlags() (config, error) {
	server := flag.String("server", "", "server address, http(s):// or ws(s)://")
	username := flag.String("username", "", "client username")
	password := flag.String("password", "", "client password")
	dsn := flag.String("dsn", "", "DSN, format: ws(s)://username:password@yourdomain.com")
	interval := flag.Float64("interval", 1.5, "data collection interval in seconds")
	custom := flag.String("custom", "complete-go-client", "custom status text")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}
	if *interval <= 0 {
		return config{}, errors.New("interval must be greater than 0")
	}

	cfg := config{
		username: *username,
		password: *password,
		interval: time.Duration(*interval * float64(time.Second)),
		custom:   *custom,
	}
	if *server != "" {
		if err := applyEndpoint(&cfg, *server); err != nil {
			return config{}, err
		}
	}
	if *dsn != "" {
		if err := applyEndpoint(&cfg, *dsn); err != nil {
			return config{}, err
		}
	}
	if *username != "" {
		cfg.username = *username
	}
	if *password != "" {
		cfg.password = *password
	}
	if cfg.server == "" || cfg.username == "" || cfg.password == "" {
		return config{}, errors.New("server, username and password can not be blank")
	}
	return cfg, nil
}

func applyEndpoint(cfg *config, raw string) error {
	server, username, password, err := parseEndpoint(raw)
	if err != nil {
		return err
	}
	cfg.server = server
	if username != "" {
		cfg.username = username
	}
	if password != "" {
		cfg.password = password
	}
	return nil
}

func parseEndpoint(raw string) (string, string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("invalid server or dsn: %s", raw)
	}

	scheme := u.Scheme
	switch scheme {
	case "http":
		scheme = "ws"
	case "https":
		scheme = "wss"
	case "ws", "wss":
	default:
		return "", "", "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	password, _ := u.User.Password()
	return scheme + "://" + u.Host, u.User.Username(), password, nil
}

func connect(cfg config, info staticInfo, auth []byte) {
	socket, _, err := websocket.DefaultDialer.Dial(cfg.server+"/connect", nil)
	if err != nil {
		log.Println("connect:", err)
		return
	}
	defer socket.Close()

	if !readContains(socket, "Authentication required") {
		return
	}
	if err := socket.WriteMessage(websocket.BinaryMessage, auth); err != nil {
		log.Println("auth:", err)
		return
	}
	if !readContains(socket, "Authentication successful") {
		return
	}

	_, buf, err := socket.ReadMessage()
	if err != nil {
		log.Println("read ip type:", err)
		return
	}
	ipMessage := string(buf)
	log.Println(ipMessage)
	checkIP := 0
	if strings.Contains(ipMessage, "IPv4") {
		checkIP = 6
	} else if strings.Contains(ipMessage, "IPv6") {
		checkIP = 4
	} else {
		return
	}

	go drain(socket)

	traffic := &trafficCounter{}
	traffic.sample()
	disks := &diskCounter{}
	online4, online6 := checkOnline(checkIP)
	nextIPCheck := time.Now().Add(150 * time.Second)

	for {
		if time.Now().After(nextIPCheck) {
			online4, online6 = checkOnline(checkIP)
			nextIPCheck = time.Now().Add(150 * time.Second)
		}

		item := collectStatus(cfg, info, traffic, disks, online4, online6)
		data, err := msgpack.Marshal(item)
		if err != nil {
			log.Println("marshal:", err)
			return
		}
		if err := socket.WriteMessage(websocket.BinaryMessage, data); err != nil {
			log.Println("write:", err)
			return
		}
	}
}

func readContains(socket *websocket.Conn, want string) bool {
	_, buf, err := socket.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		return false
	}
	message := string(buf)
	log.Println(message)
	return strings.Contains(message, want)
}

func drain(socket *websocket.Conn) {
	for {
		if _, _, err := socket.NextReader(); err != nil {
			_ = socket.Close()
			return
		}
	}
}

func collectStatus(cfg config, info staticInfo, traffic *trafficCounter, disks *diskCounter, online4, online6 bool) nodeStatus {
	cpuValue := cpuPercent(cfg.interval)
	networkTotalIn, networkTotalOut, networkRateIn, networkRateOut := traffic.sample()
	memoryTotal, memoryUsed, swapTotal, swapUsed := memory()
	hddTotal, hddUsed := disks.sample()
	load1, load5, load15 := loadAvg()
	gpuInfo, gpuUsed := gpu()

	item := nodeStatus{
		Online4:         online4,
		Online6:         online6,
		Uptime:          uptime(),
		Load:            load1,
		CPU:             cpuValue,
		MemoryTotal:     memoryTotal,
		MemoryUsed:      memoryUsed,
		SwapTotal:       swapTotal,
		SwapUsed:        swapUsed,
		HddTotal:        hddTotal,
		HddUsed:         hddUsed,
		Platform:        info.platform,
		PlatformVersion: info.platformVersion,
		Arch:            info.arch,
		Virtualization:  info.virtualization,
		CPUInfo:         info.cpuInfo,
		GPUInfo:         gpuInfo,
		Version:         version,
		Load1:           load1,
		Load5:           load5,
		Load15:          load15,
		TCPConnCount:    connCount("tcp"),
		UDPConnCount:    connCount("udp"),
		ProcessCount:    processCount(),
		Temperatures:    temperature(),
		GPU:             gpuUsed,
		Custom:          cfg.custom,
	}
	fillNetworkFields(&item, networkTotalIn, networkTotalOut, networkRateIn, networkRateOut)
	return item
}

func fillNetworkFields(item *nodeStatus, totalIn, totalOut, rateIn, rateOut uint64) {
	// Current server semantics: network_rx/tx are cumulative bytes, network_in/out are bytes per second.
	item.NetworkRx = totalIn
	item.NetworkTx = totalOut
	item.NetworkIn = rateIn
	item.NetworkOut = rateOut
}

func collectStaticInfo() staticInfo {
	info := staticInfo{arch: runtime.GOARCH}
	if hostInfo, err := host.Info(); err == nil {
		info.platform = hostInfo.Platform
		info.platformVersion = hostInfo.PlatformVersion
		info.arch = firstNonEmpty(hostInfo.KernelArch, runtime.GOARCH)
		info.virtualization = hostInfo.VirtualizationSystem
	}
	if items, err := cpu.Info(); err == nil {
		seen := map[string]struct{}{}
		for _, item := range items {
			name := strings.TrimSpace(item.ModelName)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			info.cpuInfo = append(info.cpuInfo, name)
		}
	}
	if info.cpuInfo == nil {
		info.cpuInfo = []string{}
	}
	return info
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func uptime() uint64 {
	value, _ := host.Uptime()
	return value
}

func loadAvg() (float64, float64, float64) {
	avg, err := load.Avg()
	if err != nil {
		return 0, 0, 0
	}
	return avg.Load1, avg.Load5, avg.Load15
}

func cpuPercent(interval time.Duration) float64 {
	values, err := cpu.Percent(interval, false)
	if err != nil || len(values) == 0 {
		return 0
	}
	return math.Round(values[0]*10) / 10
}

func memory() (uint64, uint64, uint64, uint64) {
	vm, _ := mem.VirtualMemory()
	swap, _ := mem.SwapMemory()
	return vm.Total / 1024, vm.Used / 1024, swap.Total / 1024, swap.Used / 1024
}

func (d *diskCounter) sample() (uint64, uint64) {
	if d.mounts == nil || time.Now().After(d.nextRefresh) {
		d.refresh()
	}

	var total, used uint64
	for mount := range d.mounts {
		usage, err := disk.Usage(mount)
		if err != nil {
			delete(d.mounts, mount)
			continue
		}
		total += usage.Total / 1024 / 1024
		used += usage.Used / 1024 / 1024
	}
	return total, used
}

func (d *diskCounter) refresh() {
	d.mounts = map[string]struct{}{}
	seenDevices := map[string]struct{}{}
	partitions, err := disk.Partitions(false)
	if err != nil {
		return
	}
	for _, part := range partitions {
		if !validFS(part.Fstype) {
			continue
		}
		if _, ok := seenDevices[part.Device]; ok {
			continue
		}
		seenDevices[part.Device] = struct{}{}
		d.mounts[part.Mountpoint] = struct{}{}
	}
	d.nextRefresh = time.Now().Add(5 * time.Minute)
}

func validFS(name string) bool {
	switch strings.ToLower(name) {
	case "ext2", "ext3", "ext4", "reiserfs", "jfs", "btrfs", "fuseblk", "zfs", "simfs", "ntfs", "fat32", "exfat", "xfs", "apfs":
		return true
	default:
		return false
	}
}

func (c *trafficCounter) sample() (uint64, uint64, uint64, uint64) {
	var in, out uint64
	items, _ := gopsnet.IOCounters(true)
	for _, item := range items {
		if validInterface(item.Name) {
			in += item.BytesRecv
			out += item.BytesSent
		}
	}
	rx, tx := c.rates(time.Now(), in, out)
	return in, out, rx, tx
}

func (c *trafficCounter) rates(now time.Time, in, out uint64) (uint64, uint64) {
	if c.prevAt.IsZero() {
		c.prevIn, c.prevOut, c.prevAt = in, out, now
		return 0, 0
	}
	seconds := now.Sub(c.prevAt).Seconds()
	if seconds <= 0 {
		seconds = 1
	}
	rx := rate(in, c.prevIn, seconds)
	tx := rate(out, c.prevOut, seconds)
	c.prevIn, c.prevOut, c.prevAt = in, out, now
	return rx, tx
}

func rate(current, previous uint64, seconds float64) uint64 {
	if current < previous {
		return 0
	}
	return uint64(float64(current-previous) / seconds)
}

func validInterface(name string) bool {
	iface, err := net.InterfaceByName(name)
	if err == nil {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			return false
		}
	}
	for _, prefix := range []string{"tun", "kube", "docker", "vmbr", "br-", "vnet", "veth"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func checkOnline(checkIP int) (bool, bool) {
	if checkIP == 4 {
		return network(4), true
	}
	return true, network(6)
}

func network(ip int) bool {
	networkName := "tcp4"
	address := "8.8.8.8:53"
	if ip == 6 {
		networkName = "tcp6"
		address = "[2001:4860:4860::8888]:53"
	}

	conn, err := net.DialTimeout(networkName, address, 2*time.Second)
	if err != nil {
		return false
	}
	return conn.Close() == nil
}

func connCount(kind string) uint64 {
	items, err := gopsnet.ConnectionsWithoutUids(kind)
	if err != nil {
		return 0
	}
	return uint64(len(items))
}

func processCount() uint64 {
	info, err := host.Info()
	if err != nil {
		return 0
	}
	return info.Procs
}

func temperature() float64 {
	items, err := sensors.SensorsTemperatures()
	if err != nil {
		return 0
	}
	var maxTemp float64
	for _, item := range items {
		if item.Temperature > maxTemp {
			maxTemp = item.Temperature
		}
	}
	return math.Round(maxTemp*10) / 10
}

func gpu() ([]string, float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(
		ctx,
		"nvidia-smi",
		"--query-gpu=name,utilization.gpu",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return []string{}, 0
	}

	var names []string
	var maxUsed float64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, rawUsed, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
		used, err := strconv.ParseFloat(strings.TrimSpace(rawUsed), 64)
		if err == nil && used > maxUsed {
			maxUsed = used
		}
	}
	return names, maxUsed
}

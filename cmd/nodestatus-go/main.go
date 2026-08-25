package main

import (
	"bufio"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nodestatus-go/internal/server"
	"nodestatus-go/internal/status"
	"nodestatus-go/internal/store"
)

func main() {
	loadEnv(filepath.Join(os.Getenv("HOME"), ".nodestatus", ".env.local"))

	port := flag.Int("port", envInt("PORT", 35601), "web server port")
	interval := flag.Int("interval", envInt("INTERVAL", 1500), "public websocket update interval in milliseconds")
	pingInterval := flag.Int("ping-interval", envInt("PING_INTERVAL", 30), "websocket ping interval in seconds")
	reconnectTimeout := flag.Int("reconnect-timeout", envInt("RECONNECT_TIMEOUT", envInt("PUSH_TIMEOUT", 120)), "disconnect event delay in seconds")
	database := flag.String("database", envString("DATABASE", defaultDatabase()), "SQLite database path or file: URL")
	adminDir := flag.String("admin-dir", envString("ADMIN_DIR", filepath.Join("web", "hotaru-admin", "dist")), "built hotaru-admin dist directory")
	webUsername := flag.String("web-username", envString("WEB_USERNAME", "admin"), "admin username")
	webPassword := flag.String("web-password", envString("WEB_PASSWORD", ""), "admin password")
	webSecret := flag.String("web-secret", envString("WEB_SECRET", "node-secret"), "JWT secret")
	webTitle := flag.String("web-title", envString("WEB_TITLE", "Server Status"), "public title")
	webSubtitle := flag.String("web-subtitle", envString("WEB_SUBTITLE", "Servers' Probes Set up with NodeStatus"), "public subtitle")
	webHeadtitle := flag.String("web-headtitle", envString("WEB_HEADTITLE", "NodeStatus"), "public head title")
	flag.Parse()

	if *webPassword == "" {
		log.Fatal("WEB_PASSWORD or --web-password is required")
	}

	st, err := store.Open(*database)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	hub, err := status.NewHub(st, status.Options{
		Interval:         time.Duration(*interval) * time.Millisecond,
		PingInterval:     time.Duration(*pingInterval) * time.Second,
		ReconnectTimeout: time.Duration(*reconnectTimeout) * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer hub.Close()

	handler := server.New(st, hub, server.Config{
		WebUsername: *webUsername, WebPassword: *webPassword, WebSecret: *webSecret,
		WebTitle: *webTitle, WebSubtitle: *webSubtitle, WebHeadtitle: *webHeadtitle,
		AdminDir: *adminDir,
	})
	addr := ":" + strconv.Itoa(*port)
	log.Printf("NodeStatus Go listening on http://127.0.0.1%s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func loadEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if _, ok := os.LookupEnv(key); !ok {
			os.Setenv(key, value)
		}
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return value
	}
	return fallback
}

func defaultDatabase() string {
	if _, err := os.Stat("/usr/local/NodeStatus/server/db.sqlite"); err == nil {
		return "/usr/local/NodeStatus/server/db.sqlite"
	}
	return filepath.Join(os.Getenv("HOME"), ".nodestatus", "db.sqlite")
}

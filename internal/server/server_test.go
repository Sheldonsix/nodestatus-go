package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"

	"nodestatus-go/internal/status"
	"nodestatus-go/internal/store"
)

func TestSession(t *testing.T) {
	token, err := CreateToken("admin", "secret", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	username, err := VerifyToken(token, "secret")
	if err != nil || username != "admin" {
		t.Fatalf("username=%s err=%v", username, err)
	}
}

func TestAdminAndWebSockets(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CreateServer(context.Background(), store.ServerInput{
		Username: "node", Password: "secret", Name: "node", Type: "kvm", Location: "US", Region: "US",
	}); err != nil {
		t.Fatal(err)
	}
	hub, err := status.NewHub(st, status.Options{Interval: 20 * time.Millisecond, ReconnectTimeout: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	ts := httptest.NewServer(NewTestHandler(st, hub, Config{}))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/admin/session", "application/json", bytes.NewBufferString(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	var login response
	if err := json.NewDecoder(resp.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if login.Code != 0 || login.Data == nil {
		t.Fatalf("login failed: %#v", login)
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	public, _, err := websocket.DefaultDialer.Dial(wsURL+"/public", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer public.Close()

	agent, _, err := websocket.DefaultDialer.Dial(wsURL+"/connect", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	if _, msg, err := agent.ReadMessage(); err != nil || !strings.Contains(string(msg), "Authentication required") {
		t.Fatalf("auth prompt %q err=%v", msg, err)
	}
	auth, _ := msgpack.Marshal(map[string]string{"username": "node", "password": "secret"})
	if err := agent.WriteMessage(websocket.BinaryMessage, auth); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := agent.ReadMessage(); err != nil {
			t.Fatal(err)
		}
	}
	payload, _ := msgpack.Marshal(map[string]any{
		"online4": true, "online6": false, "uptime": 1, "load": 0.1, "cpu": 20,
		"network_rx": 1000, "network_tx": 2000, "network_in": 30, "network_out": 40,
		"memory_total": 100, "memory_used": 50, "swap_total": 0, "swap_used": 0,
		"hdd_total": 1000, "hdd_used": 500, "custom": "",
	})
	if err := agent.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var msg struct {
			Servers []status.ServerItem `json:"servers"`
		}
		if err := public.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if len(msg.Servers) == 1 && msg.Servers[0].Status["online4"] == true {
			return
		}
	}
	t.Fatal("public websocket did not receive updated status")
}

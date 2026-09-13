package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"nodestatus-go/internal/status"
	"nodestatus-go/internal/store"
)

type Config struct {
	WebUsername  string
	WebPassword  string
	WebSecret    string
	WebTitle     string
	WebSubtitle  string
	WebHeadtitle string
}

type Server struct {
	store *store.Store
	hub   *status.Hub
	cfg   Config
}

type response struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

func New(st *store.Store, hub *status.Hub, cfg Config) http.Handler {
	return &Server{store: st, hub: hub, cfg: cfg}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/connect":
		s.hub.ServeConnect(w, r)
	case r.URL.Path == "/public":
		s.hub.ServePublic(w, r)
	case r.URL.Path == "/api/admin/session":
		s.handleSession(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/admin/"):
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, fail("Verification failure"))
			return
		}
		s.handleAdmin(w, r)
	case r.URL.Path == "/api/config":
		writeJSON(w, http.StatusOK, map[string]string{"title": s.cfg.WebTitle, "subTitle": s.cfg.WebSubtitle, "headTitle": s.cfg.WebHeadtitle})
	case r.URL.Path == "/api/status":
		writeJSON(w, http.StatusOK, map[string]any{"servers": s.hub.ServersPub(), "updated": time.Now().Unix()})
	case strings.HasPrefix(r.URL.Path, "/api/server/") && strings.HasSuffix(r.URL.Path, "/history"):
		s.handleHistory(w, r)
	case r.URL.Path == "/":
		w.Write([]byte("NodeStatus Go API\n"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, fail("Username and password cannot be empty"))
			return
		}
		if strings.TrimSpace(body.Username) == "" || strings.TrimSpace(body.Password) == "" {
			writeJSON(w, http.StatusBadRequest, fail("Username and password cannot be empty"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Username)), []byte(s.cfg.WebUsername)) == 1 &&
			subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Password)), []byte(s.cfg.WebPassword)) == 1 {
			token, err := CreateToken(body.Username, s.cfg.WebSecret, time.Now())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, ok(token, "ok"))
			return
		}
		writeJSON(w, http.StatusUnauthorized, fail("username or password is incorrect"))
	case http.MethodGet:
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, fail("Verification failure"))
			return
		}
		writeJSON(w, http.StatusOK, ok(nil, "Verification success"))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch {
	case r.URL.Path == "/api/admin/servers" && r.Method == http.MethodGet:
		servers, err := s.store.ListServers(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		sort.Slice(servers, func(i, j int) bool { return servers[i].Order > servers[j].Order })
		writeJSON(w, http.StatusOK, ok(servers, "ok"))
	case r.URL.Path == "/api/admin/servers" && r.Method == http.MethodPost:
		s.createServer(w, r)
	case r.URL.Path == "/api/admin/servers" && r.Method == http.MethodPut:
		s.updateServer(w, r)
	case r.URL.Path == "/api/admin/servers/order" && r.Method == http.MethodPut:
		var body struct {
			Order []int `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Order) == 0 {
			writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
			return
		}
		if err := s.store.UpdateOrder(ctx, body.Order); err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		_ = s.hub.RefreshAll(ctx, false)
		writeJSON(w, http.StatusOK, ok(nil, "ok"))
	case strings.HasPrefix(r.URL.Path, "/api/admin/servers/") && r.Method == http.MethodDelete:
		username := strings.TrimPrefix(r.URL.Path, "/api/admin/servers/")
		if username == "" {
			writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
			return
		}
		if err := s.store.DeleteServer(ctx, username); err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		_ = s.hub.RefreshServer(ctx, username, true)
		writeJSON(w, http.StatusOK, ok(nil, "ok"))
	case r.URL.Path == "/api/admin/events" && r.Method == http.MethodGet:
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		count, list, err := s.store.ListEvents(ctx, size, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, ok(map[string]any{"count": count, "list": list}, "ok"))
	case r.URL.Path == "/api/admin/events" && r.Method == http.MethodDelete:
		if err := s.store.DeleteAllEvents(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, ok(nil, "ok"))
	case strings.HasPrefix(r.URL.Path, "/api/admin/events/") && r.Method == http.MethodDelete:
		id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/admin/events/"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
			return
		}
		if err := s.store.DeleteEvent(ctx, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, ok(nil, "ok"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
		return
	}
	if blob, hasData := raw["data"]; hasData {
		var encoded string
		if err := json.Unmarshal(blob, &encoded); err != nil {
			writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
			return
		}
		var inputs []store.ServerInput
		if err := json.Unmarshal([]byte(encoded), &inputs); err != nil {
			writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
			return
		}
		if err := s.store.BulkCreateServers(r.Context(), inputs); err != nil {
			writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
			return
		}
		_ = s.hub.RefreshAll(r.Context(), false)
		writeJSON(w, http.StatusOK, ok(nil, "ok"))
		return
	}
	var input store.ServerInput
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
		return
	}
	if err := s.store.CreateServer(r.Context(), input); err != nil {
		writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
		return
	}
	_ = s.hub.RefreshServer(r.Context(), input.Username, false)
	writeJSON(w, http.StatusOK, ok(nil, "ok"))
}

func (s *Server) updateServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string         `json:"username"`
		Data     map[string]any `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Data == nil {
		writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
		return
	}
	if value, ok := body.Data["username"].(string); ok && value == body.Username {
		delete(body.Data, "username")
	}
	newUsername, disconnect, err := s.store.UpdateServer(r.Context(), body.Username, body.Data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
		return
	}
	_ = s.hub.RefreshServer(r.Context(), body.Username, disconnect)
	if newUsername != "" {
		_ = s.hub.RefreshServer(r.Context(), newUsername, true)
	}
	writeJSON(w, http.StatusOK, ok(nil, "ok"))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/server/"), "/history")
	if username == "" {
		writeJSON(w, http.StatusBadRequest, fail("Wrong request"))
		return
	}
	rng := status.ParseHistoryRange(r.URL.Query().Get("range"))
	metric := status.ParseHistoryMetric(r.URL.Query().Get("metric"))
	data, err := s.hub.History(r.Context(), username, rng, metric)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, fail(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, ok(data, "ok"))
}

func (s *Server) authorized(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(strings.TrimSpace(parts[0])) != "bearer" {
		return false
	}
	username, err := VerifyToken(strings.TrimSpace(parts[1]), s.cfg.WebSecret)
	return err == nil && username == s.cfg.WebUsername
}

func CreateToken(username, secret string, now time.Time) (string, error) {
	claims := jwt.MapClaims{"username": username, "exp": now.Add(7 * 24 * time.Hour).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func VerifyToken(token, secret string) (string, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	username, _ := claims["username"].(string)
	if username == "" {
		return "", errors.New("invalid token")
	}
	return username, nil
}

func ok(data any, msg string) response {
	return response{Code: 0, Data: data, Msg: msg}
}

func fail(msg string) response {
	return response{Code: 1, Data: nil, Msg: msg}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func NewTestHandler(st *store.Store, hub *status.Hub, cfg Config) http.Handler {
	if cfg.WebUsername == "" {
		cfg.WebUsername = "admin"
	}
	if cfg.WebPassword == "" {
		cfg.WebPassword = "password"
	}
	if cfg.WebSecret == "" {
		cfg.WebSecret = "secret"
	}
	return New(st, hub, cfg)
}

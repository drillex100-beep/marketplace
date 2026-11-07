// Command status-page is Scilla's official status page addon: it polls the panel's
// nodes and protocols, keeps an hour-by-hour history in the panel's KV store and
// serves a public status page the panel proxies below the subscription path
// (/<sub>/addons-public/status-page/). Protocol: ../../PROTOCOL-v2.md plus the
// ui:public permission of the panel; it speaks no scripts, the page is plain
// HTML with a meta refresh.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	addonID      = "status-page"
	version      = "0.5.2.2"
	protocol     = 2
	pollEvery    = 30 * time.Second
	persistEvery = 5 * time.Minute
	kvSnapshot   = "snapshot"
	kvBucket     = "b/"
)

type config struct {
	listen       string
	adapterToken string
	panelURL     string
	panelToken   string
}

func envConfig() config {
	return config{
		listen:       envOr("SCILLA_ADAPTER_LISTEN", "127.0.0.1:8080"),
		adapterToken: os.Getenv("SCILLA_ADAPTER_TOKEN"),
		panelURL:     strings.TrimRight(os.Getenv("SCILLA_PANEL_URL"), "/"),
		panelToken:   os.Getenv("SCILLA_PANEL_TOKEN"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// panelNode and panelInbound mirror the panel's /panel/v2 answers (nodes:read and
// inbounds:read: no addresses, keys or secrets).
type panelNode struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PublicName string `json:"public_name"`
	Local      bool   `json:"local"`
	Enabled    bool   `json:"enabled"`
	Status     string `json:"status"` // ok, error, unknown
	Version    string `json:"version,omitempty"`
	Conns      int    `json:"conns"`
}

type panelInbound struct {
	ID          int64    `json:"id"`
	NodeID      int64    `json:"node_id"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Preset      string   `json:"preset"`
	Type        string   `json:"type"`
	Network     string   `json:"network"`
	Port        string   `json:"port"`
	Dest        string   `json:"dest,omitempty"`
	ServerNames []string `json:"server_names,omitempty"`
	Enabled     bool     `json:"enabled"`
	Status      string   `json:"status"` // ok, error, off, unknown
}

// counts is one hour bucket of one protocol.
type counts struct {
	OK      int `json:"ok"`
	Error   int `json:"error"`
	Off     int `json:"off"`
	Unknown int `json:"unknown"`
}

// state is what the page renders; buckets and incidents survive restarts through KV.
type state struct {
	Nodes      []panelNode                   `json:"nodes"`
	Inbounds   []panelInbound                `json:"inbounds"`
	Buckets    map[string]map[string]*counts `json:"-"`         // inbound id -> hour (UTC "2006-01-02T15") -> counts
	Incidents  map[int64]time.Time           `json:"incidents"` // inbound id -> last ok→error transition
	Streaks    map[int64]int                 `json:"-"`         // inbound id -> consecutive error polls
	LastPoll   time.Time                     `json:"last_poll"`
	PollFailed bool                          `json:"poll_failed"`
}

type server struct {
	cfg    config
	log    *slog.Logger
	mu     sync.Mutex
	st     *state
	dirty  map[string]bool // bucket keys changed since the last persist
	client *http.Client
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := envConfig()
	if cfg.adapterToken == "" || cfg.panelURL == "" || cfg.panelToken == "" {
		log.Error("SCILLA_ADAPTER_TOKEN, SCILLA_PANEL_URL and SCILLA_PANEL_TOKEN are required")
		os.Exit(1)
	}
	s := &server{
		cfg:    cfg,
		log:    log,
		st:     &state{Buckets: map[string]map[string]*counts{}, Incidents: map[int64]time.Time{}, Streaks: map[int64]int{}},
		dirty:  map[string]bool{},
		client: &http.Client{Timeout: 10 * time.Second},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := s.restore(ctx); err != nil {
		log.Warn("restore", "err", err)
	}
	go s.pollLoop(ctx)
	go s.persistLoop(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/info", s.auth(s.info))
	mux.HandleFunc("/v2/health", s.auth(s.health))
	mux.HandleFunc("/v2/", s.auth(s.notFound))
	mux.HandleFunc("/ui/", s.auth(s.ui))
	mux.HandleFunc("/", s.auth(s.notFound))
	srv := &http.Server{Addr: cfg.listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		pctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.persist(pctx)
		_ = srv.Shutdown(pctx)
	}()
	log.Info("status-page listening", "addr", cfg.listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}

func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.cfg.adapterToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "unauthorized", "message": "bad token"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) notFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"code": "not_found", "message": "not found"})
}

// info answers GET /v2/info: the addon describes itself; the permissions the panel
// trusts are the catalog's, these are for its texts.
func (s *server) info(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       addonID,
		"protocol": protocol,
		"version":  version,
		"kind":     "ui",
		"name":     map[string]string{"ru": "Статус-страница", "en": "Status page"},
		"description": map[string]string{
			"ru": "Публичная страница доступности серверов и протоколов: что работает, что нет и почему",
			"en": "A public page of the servers' and protocols' availability: what works, what does not and why",
		},
		"permissions": []string{"nodes:read", "inbounds:read", "kv", "ui:public"},
		"ui":          map[string]any{"entry": "/ui/index.html", "title": map[string]string{"ru": "Статус", "en": "Status"}},
	})
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	last, failed := s.st.LastPoll, s.st.PollFailed
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "last_poll": last, "poll_failed": failed})
}

// panelGet calls the Panel API.
func (s *server) panelGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.panelURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.panelToken)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("panel %s: %d", path, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

func (s *server) pollLoop(ctx context.Context) {
	s.poll(ctx)
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.poll(ctx)
		}
	}
}

// poll fetches the nodes and protocols and folds the answer into the state: the
// current hour's bucket of every protocol grows by one, and an error has to hold for
// two polls in a row before the page shows it, so one lost check does not flap.
func (s *server) poll(ctx context.Context) {
	var nodes []panelNode
	var inbounds []panelInbound
	err1 := s.panelGet(ctx, "/nodes", &nodes)
	err2 := s.panelGet(ctx, "/inbounds", &inbounds)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err1 != nil || err2 != nil {
		s.st.PollFailed = true
		if err1 != nil {
			s.log.Warn("poll nodes", "err", err1)
		}
		if err2 != nil {
			s.log.Warn("poll inbounds", "err", err2)
		}
		return
	}
	s.st.PollFailed = false
	s.st.LastPoll = time.Now()
	s.st.Nodes, s.st.Inbounds = nodes, inbounds
	hour := time.Now().UTC().Format("2006-01-02T15")
	seen := map[int64]bool{}
	for _, in := range inbounds {
		seen[in.ID] = true
		b := s.st.Buckets[strconv64(in.ID)]
		if b == nil {
			b = map[string]*counts{}
			s.st.Buckets[strconv64(in.ID)] = b
		}
		c := b[hour]
		if c == nil {
			c = &counts{}
			b[hour] = c
		}
		raw := in.Status
		prevStreak := s.st.Streaks[in.ID]
		disp := raw
		switch raw {
		case "error":
			if prevStreak == 0 {
				disp = "unknown" // the first bad poll is not shown yet
			}
			s.st.Streaks[in.ID] = prevStreak + 1
			if prevStreak == 1 { // the second bad poll confirms the outage
				s.st.Incidents[in.ID] = time.Now()
			}
		default:
			s.st.Streaks[in.ID] = 0
		}
		switch disp {
		case "ok":
			c.OK++
		case "error":
			c.Error++
		case "off":
			c.Off++
		default:
			c.Unknown++
		}
		s.dirty[hourKey(in.ID, hour)] = true
	}
	// Forget streaks of protocols that are gone.
	for id := range s.st.Streaks {
		if !seen[id] {
			delete(s.st.Streaks, id)
		}
	}
}

func strconv64(v int64) string             { return fmt.Sprintf("%d", v) }
func hourKey(id int64, hour string) string { return kvBucket + strconv64(id) + "/" + hour }

func (s *server) persistLoop(ctx context.Context) {
	t := time.NewTicker(persistEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_ = s.persist(pctx)
			cancel()
		}
	}
}

// persist writes the dirty hour buckets and the snapshot to the panel's KV.
func (s *server) persist(ctx context.Context) error {
	s.mu.Lock()
	dirty := make([]string, 0, len(s.dirty))
	for k := range s.dirty {
		dirty = append(dirty, k)
	}
	snap := *s.st
	snap.Buckets = nil
	s.mu.Unlock()
	for _, k := range dirty {
		parts := strings.Split(k, "/") // b/<id>/<hour>
		if len(parts) != 3 {
			continue
		}
		s.mu.Lock()
		b := s.st.Buckets[parts[1]][parts[2]]
		s.mu.Unlock()
		if b == nil {
			continue
		}
		body, _ := json.Marshal(b)
		if err := s.kvPut(ctx, k, body, 0); err != nil {
			s.log.Warn("kv put", "key", k, "err", err)
			continue
		}
		s.mu.Lock()
		delete(s.dirty, k)
		s.mu.Unlock()
	}
	body, _ := json.Marshal(snap)
	return s.kvPut(ctx, kvSnapshot, body, 0)
}

// restore loads the snapshot and every hour bucket back from the KV.
func (s *server) restore(ctx context.Context) error {
	var snap state
	body, err := s.kvGet(ctx, kvSnapshot)
	if err == nil && len(body) > 0 {
		if err := json.Unmarshal(body, &snap); err == nil {
			s.mu.Lock()
			s.st.Nodes, s.st.Inbounds, s.st.Incidents = snap.Nodes, snap.Inbounds, snap.Incidents
			s.st.LastPoll, s.st.PollFailed = snap.LastPoll, snap.PollFailed
			s.mu.Unlock()
		}
	}
	// Page through the bucket keys and read each value.
	after := ""
	for {
		var page struct {
			Keys []string `json:"keys"`
			Next string   `json:"next"`
		}
		if err := s.panelGet(ctx, "/kv?prefix="+kvBucket+"&after="+after+"&limit=1000", &page); err != nil {
			return err
		}
		for _, k := range page.Keys {
			v, err := s.kvGet(ctx, k)
			if err != nil || len(v) == 0 {
				continue
			}
			var c counts
			if json.Unmarshal(v, &c) != nil {
				continue
			}
			parts := strings.Split(k, "/")
			if len(parts) != 3 {
				continue
			}
			s.mu.Lock()
			b := s.st.Buckets[parts[1]]
			if b == nil {
				b = map[string]*counts{}
				s.st.Buckets[parts[1]] = b
			}
			b[parts[2]] = &c
			s.mu.Unlock()
		}
		if page.Next == "" {
			return nil
		}
		after = page.Next
	}
}

func (s *server) kvGet(ctx context.Context, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.panelURL+"/kv/"+key, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.panelToken)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kv get %s: %d", key, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<10))
}

func (s *server) kvPut(ctx context.Context, key string, body []byte, ttl int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.cfg.panelURL+"/kv/"+key, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.panelToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("kv put %s: %d %s", key, resp.StatusCode, b)
	}
	return nil
}

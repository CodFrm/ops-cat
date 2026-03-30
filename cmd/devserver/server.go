// cmd/devserver/server.go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/opskat/opskat/pkg/extension"
	"go.uber.org/zap"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Server serves the DevServer HTTP API, WebSocket event streaming, and extension frontend.
type Server struct {
	mux         *http.ServeMux
	manifest    *extension.Manifest
	plugin      *extension.Plugin
	host        *DevServerHost
	extDir      string
	extFrontend string
	wsClients   map[*websocket.Conn]struct{}
	wsMu        sync.Mutex
}

// NewServer creates a new DevServer HTTP server.
func NewServer(
	m *extension.Manifest,
	plugin *extension.Plugin,
	host *DevServerHost,
	extDir string,
	extFrontend string,
) *Server {
	s := &Server{
		mux:         http.NewServeMux(),
		manifest:    m,
		plugin:      plugin,
		host:        host,
		extDir:      extDir,
		extFrontend: extFrontend,
		wsClients:   make(map[*websocket.Conn]struct{}),
	}

	// Wire host callbacks for WebSocket broadcast
	host.SetLogCallback(func(level, msg string) {
		s.broadcast(map[string]any{"type": "log", "level": level, "message": msg})
	})
	host.SetEventCallback(func(eventType string, data json.RawMessage) {
		s.broadcast(map[string]any{"type": "event", "eventType": eventType, "data": data})
	})

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/manifest", s.handleGetManifest)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("PUT /api/credential", s.handlePutCredential)
	s.mux.HandleFunc("POST /api/tool/{name}", s.handleCallTool)
	s.mux.HandleFunc("POST /api/action/{name}", s.handleCallAction)
	s.mux.HandleFunc("POST /api/policy/{tool}", s.handleCheckPolicy)
	s.mux.HandleFunc("GET /api/kv", s.handleListKV)
	s.mux.HandleFunc("/ws/events", s.handleWebSocket)

	// Extension frontend proxy or static
	if s.extFrontend != "" {
		target, _ := url.Parse(s.extFrontend)
		proxy := httputil.NewSingleHostReverseProxy(target)
		s.mux.Handle("/extensions/", http.StripPrefix("/extensions", proxy))
	} else {
		s.mux.Handle("/extensions/", http.StripPrefix("/extensions/",
			http.FileServer(http.Dir(filepath.Join(s.extDir, "frontend")))))
	}

	// DevServer frontend SPA (embedded static files)
	s.mux.HandleFunc("/", s.handleSPA)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

// --- API Handlers ---

func (s *Server) handleGetManifest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.manifest)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	data, err := os.ReadFile(filepath.Join(s.host.dataDir, "config.json"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	err := os.WriteFile(filepath.Join(s.host.dataDir, "config.json"), data, 0644)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePutCredential(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	err := os.WriteFile(filepath.Join(s.host.dataDir, "credential.json"), data, 0644)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.plugin == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "plugin not loaded"})
		return
	}
	args, _ := io.ReadAll(r.Body)
	result, err := s.plugin.CallTool(r.Context(), name, args)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

func (s *Server) handleCallAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.plugin == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "plugin not loaded"})
		return
	}
	args, _ := io.ReadAll(r.Body)
	result, err := s.plugin.CallAction(r.Context(), name, args)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

func (s *Server) handleCheckPolicy(w http.ResponseWriter, r *http.Request) {
	tool := r.PathValue("tool")
	if s.plugin == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "plugin not loaded"})
		return
	}
	args, _ := io.ReadAll(r.Body)
	action, resource, err := s.plugin.CheckPolicy(r.Context(), tool, args)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"action": action, "resource": resource})
}

func (s *Server) handleListKV(w http.ResponseWriter, _ *http.Request) {
	s.host.kvMu.Lock()
	kv := make(map[string]string, len(s.host.kv))
	for k, v := range s.host.kv {
		kv[k] = string(v)
	}
	s.host.kvMu.Unlock()
	writeJSON(w, http.StatusOK, kv)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		zap.L().Warn("websocket accept", zap.Error(err))
		return
	}
	s.wsMu.Lock()
	s.wsClients[conn] = struct{}{}
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, conn)
		s.wsMu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			return
		}
	}
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!doctype html><html><body><h1>DevServer</h1><p>Frontend not built yet.</p></body></html>`))
}

func (s *Server) broadcast(msg any) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for conn := range s.wsClients {
		_ = wsjson.Write(context.Background(), conn, msg)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

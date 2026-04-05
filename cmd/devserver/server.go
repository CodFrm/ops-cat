// cmd/devserver/server.go
package main

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/opskat/opskat/pkg/extension"
	"go.uber.org/zap"
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
	s.mux.HandleFunc("POST /api/tool/{name}", s.handleCallTool)
	s.mux.HandleFunc("POST /api/action/{name}", s.handleCallAction)
	s.mux.HandleFunc("POST /api/policy/{tool}", s.handleCheckPolicy)
	s.mux.HandleFunc("GET /api/kv", s.handleListKV)
	s.mux.HandleFunc("/ws/events", s.handleWebSocket)

	// Extension frontend proxy or static — route under /extensions/{name}/ so that
	// the frontend loader's import(`/extensions/${name}/${entry}`) resolves correctly.
	extPrefix := "/extensions/" + s.manifest.Name
	if s.extFrontend != "" {
		target, _ := url.Parse(s.extFrontend)
		proxy := httputil.NewSingleHostReverseProxy(target)
		s.mux.Handle(extPrefix+"/", http.StripPrefix(extPrefix, proxy))
	} else {
		s.mux.Handle(extPrefix+"/", http.StripPrefix(extPrefix+"/",
			http.FileServer(http.Dir(s.extDir))))
	}

	// DevServer frontend SPA (embedded static files)
	staticFS, _ := fs.Sub(staticAssets, "static/assets")
	fileServer := http.FileServer(http.FS(staticFS))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA: serve index.html for non-asset paths
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			http.NotFound(w, r)
			return
		}
		// Try serving the file; fall back to index.html for SPA routes
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(staticFS, path); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Serve index.html for SPA
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s) //nolint:gosec // local development server
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
	if _, err := w.Write(data); err != nil {
		zap.L().Warn("write response", zap.Error(err))
	}
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	// Limit body to 1 MB to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body too large or unreadable"})
		return
	}
	// Validate JSON before writing to prevent persistent corruption
	var tmp any
	if err := json.Unmarshal(data, &tmp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	// 0600 — prevent world-readable credentials on shared hosts
	if err := os.WriteFile(filepath.Join(s.host.dataDir, "config.json"), data, 0600); err != nil {
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
	if _, err := w.Write(result); err != nil { //nolint:gosec // dev server, result is from trusted plugin
		zap.L().Warn("write response", zap.Error(err))
	}
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
	if _, err := w.Write(result); err != nil { //nolint:gosec // dev server, result is from trusted plugin
		zap.L().Warn("write response", zap.Error(err))
	}
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
		if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
			zap.L().Warn("websocket close", zap.Error(err))
		}
	}()

	for {
		_, _, err := conn.Read(r.Context())
		if err != nil {
			return
		}
	}
}

func (s *Server) broadcast(msg any) {
	// Snapshot client list under lock, then release before any network I/O
	s.wsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(s.wsClients))
	for c := range s.wsClients {
		conns = append(conns, c)
	}
	s.wsMu.Unlock()

	for _, c := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = wsjson.Write(ctx, c, msg)
		cancel()
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		zap.L().Warn("encode json response", zap.Error(err))
	}
}

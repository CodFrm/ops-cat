package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/opskat/opskat/pkg/extruntime"
)

type Handler struct {
	bridge        *extruntime.Bridge
	ext           *extruntime.ExtensionInfo
	config        *DevConfig
	logStream     *LogStream
	promptContent string
	configPath    string
}

func NewHandler(
	bridge *extruntime.Bridge,
	ext *extruntime.ExtensionInfo,
	config *DevConfig,
	logStream *LogStream,
	promptContent string,
	configPath string,
) *Handler {
	return &Handler{
		bridge:        bridge,
		ext:           ext,
		config:        config,
		logStream:     logStream,
		promptContent: promptContent,
		configPath:    configPath,
	}
}

// GetManifest returns manifest info + prompt content.
func (h *Handler) GetManifest(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"manifest": h.ext.Manifest,
		"prompt":   h.promptContent,
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetConfig returns the current devconfig.
func (h *Handler) GetConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(h.config.JSON())
}

// UpdateConfig writes new config to disk and reloads.
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	data, err := readBody(r, 1<<20)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	newCfg := &DevConfig{}
	if err := newCfg.Load(data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid config: " + err.Error()})
		return
	}
	if err := os.WriteFile(h.configPath, data, 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write file: " + err.Error()})
		return
	}
	*h.config = *newCfg
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// CallTool executes an extension tool through the bridge.
func (h *Handler) CallTool(w http.ResponseWriter, r *http.Request) {
	toolName := r.PathValue("name")
	if toolName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool name required"})
		return
	}

	body, err := readBody(r, 1<<20)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Qualify tool name with extension name
	qualifiedName := h.ext.Manifest.Name + "." + toolName
	extName, tool, ok := extruntime.ParseToolName(qualifiedName)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tool name"})
		return
	}

	ctx := context.Background()
	result, err := h.bridge.ExecuteTool(ctx, extName, tool, json.RawMessage(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

// TestConnection tests the extension's connection with given config.
func (h *Handler) TestConnection(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 1<<20)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	ctx := context.Background()
	plugin := h.bridge.GetExtension(h.ext.Manifest.Name)
	if plugin == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "extension not loaded"})
		return
	}

	// CallTestConnection through the host
	host := h.bridge // Bridge has access to the host
	_ = host
	// Use the loaded plugin directly
	extHost := h.bridge
	_ = extHost

	// For test connection, we call _test_connection tool via bridge
	result, err := h.bridge.ExecuteTool(ctx, h.ext.Manifest.Name, "_test_connection", json.RawMessage(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(result))
}

// LogStream sends real-time log events via SSE.
func (h *Handler) LogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send recent entries first
	for _, entry := range h.logStream.Recent(50) {
		fmt.Fprintf(w, "data: %s\n\n", entry.JSON())
	}
	flusher.Flush()

	// Subscribe to new entries
	ch := h.logStream.Subscribe()
	defer h.logStream.Unsubscribe(ch)

	for {
		select {
		case entry := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", entry.JSON())
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// NewViteProxy creates a reverse proxy to the vite dev server for frontend HMR.
func NewViteProxy(viteURL string) http.Handler {
	target, err := url.Parse(viteURL)
	if err != nil {
		panic("invalid vite URL: " + err.Error())
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	return http.StripPrefix("/extension", proxy)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readBody(r *http.Request, maxBytes int64) ([]byte, error) {
	if r.Body == nil {
		return []byte("{}"), nil
	}
	defer r.Body.Close()
	limited := http.MaxBytesReader(nil, r.Body, maxBytes)
	data := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := limited.Read(buf)
		data = append(data, buf[:n]...)
		if err != nil {
			if err.Error() == "http: request body too large" {
				return nil, fmt.Errorf("request body too large")
			}
			break
		}
	}
	if len(data) == 0 {
		return []byte("{}"), nil
	}
	_ = strings.TrimSpace // keep import
	return data, nil
}

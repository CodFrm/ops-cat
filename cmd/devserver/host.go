// cmd/devserver/host.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/opskat/opskat/pkg/extension"
	"go.uber.org/zap"
)

// Compile-time check: DevServerHost must satisfy extension.HostProvider.
var _ extension.HostProvider = (*DevServerHost)(nil)

// DevServerHost implements extension.HostProvider using file-based mocks.
// Credentials and configs are read from JSON files in the data directory.
// KV is stored in-memory (reset on restart).
type DevServerHost struct {
	dataDir string
	logger  *zap.Logger
	io      *extension.IOHandleManager
	kv      map[string][]byte
	kvMu    sync.Mutex
	logCb   func(level, msg string)
	eventCb func(eventType string, data json.RawMessage)
}

func NewDevServerHost(dataDir string) *DevServerHost {
	return &DevServerHost{
		dataDir: dataDir,
		logger:  zap.L(),
		io:      extension.NewIOHandleManager(),
		kv:      make(map[string][]byte),
	}
}

// SetLogCallback sets a callback for log messages (WebSocket broadcast).
func (h *DevServerHost) SetLogCallback(cb func(level, msg string)) {
	h.logCb = cb
}

// SetEventCallback sets a callback for action events (WebSocket broadcast).
func (h *DevServerHost) SetEventCallback(cb func(eventType string, data json.RawMessage)) {
	h.eventCb = cb
}

func (h *DevServerHost) IOOpen(params extension.IOOpenParams) (uint32, extension.IOMeta, error) {
	switch params.Type {
	case "file":
		return h.io.OpenFile(params.Path, params.Mode)
	case "http":
		return h.io.OpenHTTP(params, nil)
	case "tcp":
		return 0, extension.IOMeta{}, fmt.Errorf("tcp IO not supported in DevServer")
	default:
		return 0, extension.IOMeta{}, fmt.Errorf("unknown IO type: %q", params.Type)
	}
}

func (h *DevServerHost) IORead(handleID uint32, size int) ([]byte, error) {
	buf := make([]byte, size)
	n, err := h.io.Read(handleID, buf)
	if n > 0 {
		// Only io.EOF is safe to delay — the guest will get it on the next Read when n==0.
		if err == nil || err == io.EOF {
			return buf[:n], nil
		}
		// Real error occurred — surface it so the guest sees the failure rather than silent truncation.
		return nil, fmt.Errorf("read handle %d: %w (had %d bytes)", handleID, err, n)
	}
	if err != nil {
		return nil, err
	}
	return buf[:0], nil
}

func (h *DevServerHost) IOWrite(handleID uint32, data []byte) (int, error) {
	return h.io.Write(handleID, data)
}

func (h *DevServerHost) IOFlush(handleID uint32) (*extension.IOMeta, error) {
	return h.io.Flush(handleID)
}

func (h *DevServerHost) IOClose(handleID uint32) error {
	return h.io.Close(handleID)
}

func (h *DevServerHost) GetAssetConfig(assetID int64) (json.RawMessage, error) {
	data, err := os.ReadFile(filepath.Join(h.dataDir, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return json.RawMessage("{}"), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if len(data) == 0 {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(data), nil
}

func (h *DevServerHost) FileDialog(dialogType string, opts extension.DialogOptions) (string, error) {
	return "", fmt.Errorf("file dialog not supported in DevServer")
}

func (h *DevServerHost) Log(level, msg string) {
	switch level {
	case "debug":
		h.logger.Debug(msg)
	case "info":
		h.logger.Info(msg)
	case "warn":
		h.logger.Warn(msg)
	case "error":
		h.logger.Error(msg)
	default:
		h.logger.Info(msg)
	}
	if h.logCb != nil {
		h.logCb(level, msg)
	}
}

func (h *DevServerHost) KVGet(key string) ([]byte, error) {
	h.kvMu.Lock()
	defer h.kvMu.Unlock()
	v, ok := h.kv[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (h *DevServerHost) KVSet(key string, value []byte) error {
	h.kvMu.Lock()
	defer h.kvMu.Unlock()
	h.kv[key] = value
	return nil
}

func (h *DevServerHost) IOSetDeadline(handleID uint32, kind string, unixNanos int64) error {
	var t time.Time
	if unixNanos != 0 {
		t = time.Unix(0, unixNanos)
	}
	return h.io.SetDeadline(handleID, kind, t)
}

func (h *DevServerHost) ActionEvent(eventType string, data json.RawMessage) error {
	if h.eventCb != nil {
		h.eventCb(eventType, data)
	}
	return nil
}

func (h *DevServerHost) ActionShouldStop() bool { return false }

func (h *DevServerHost) SetActiveCancellation(_ *extension.ActionCancellation) {}

func (h *DevServerHost) CloseAll() {
	h.io.CloseAll()
}

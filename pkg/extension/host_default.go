// pkg/extension/host_default.go
package extension

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// TunnelDialer dials a TCP address through an SSH tunnel.
type TunnelDialer interface {
	Dial(tunnelAssetID int64, addr string) (net.Conn, error)
}

// Dependency interfaces for DefaultHostProvider
type AssetConfigGetter interface {
	GetAssetConfig(assetID int64) (json.RawMessage, error)
}

type FileDialogOpener interface {
	FileDialog(dialogType string, opts DialogOptions) (string, error)
}

type KVStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
}

type ActionEventHandler interface {
	OnActionEvent(eventType string, data json.RawMessage) error
}

type DefaultHostConfig struct {
	Logger           *zap.Logger
	AssetConfigs     AssetConfigGetter
	FileDialogs      FileDialogOpener
	KV               KVStore
	ActionEvents     ActionEventHandler
	TunnelDialer     TunnelDialer // SSH tunnel dialer (nil = no tunnel support)
	AssetSSHTunnelID int64        // Current asset's SSH tunnel ID (0 = direct)
}

type DefaultHostProvider struct {
	cfg          DefaultHostConfig
	io           *IOHandleManager
	activeCancel atomic.Pointer[ActionCancellation]
}

func NewDefaultHostProvider(cfg DefaultHostConfig) *DefaultHostProvider {
	return &DefaultHostProvider{
		cfg: cfg,
		io:  NewIOHandleManager(),
	}
}

func (h *DefaultHostProvider) IOOpen(params IOOpenParams) (uint32, IOMeta, error) {
	switch params.Type {
	case "file":
		return h.io.OpenFile(params.Path, params.Mode)
	case "http":
		var dial DialFunc
		if h.cfg.AssetSSHTunnelID > 0 && h.cfg.TunnelDialer != nil {
			tunnelID := h.cfg.AssetSSHTunnelID
			dial = func(network, addr string) (net.Conn, error) {
				return h.cfg.TunnelDialer.Dial(tunnelID, addr)
			}
		}
		return h.io.OpenHTTP(params, dial)
	case "tcp":
		return h.openTCP(params)
	default:
		return 0, IOMeta{}, fmt.Errorf("unknown IO type: %q", params.Type)
	}
}

func (h *DefaultHostProvider) openTCP(params IOOpenParams) (uint32, IOMeta, error) {
	if params.Addr == "" {
		return 0, IOMeta{}, fmt.Errorf("tcp: addr is required")
	}
	timeout := time.Duration(params.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	var conn net.Conn
	var err error
	if h.cfg.AssetSSHTunnelID > 0 && h.cfg.TunnelDialer != nil {
		conn, err = h.cfg.TunnelDialer.Dial(h.cfg.AssetSSHTunnelID, params.Addr)
	} else {
		dialer := &net.Dialer{Timeout: timeout}
		conn, err = dialer.Dial("tcp", params.Addr)
	}
	if err != nil {
		return 0, IOMeta{}, err
	}

	id, err := h.io.Register(conn, conn, conn, IOMeta{})
	if err != nil {
		conn.Close()
		return 0, IOMeta{}, err
	}
	return id, IOMeta{}, nil
}

func (h *DefaultHostProvider) IORead(handleID uint32, size int) ([]byte, error) {
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

func (h *DefaultHostProvider) IOWrite(handleID uint32, data []byte) (int, error) {
	return h.io.Write(handleID, data)
}

func (h *DefaultHostProvider) IOFlush(handleID uint32) (*IOMeta, error) {
	return h.io.Flush(handleID)
}

func (h *DefaultHostProvider) IOClose(handleID uint32) error {
	return h.io.Close(handleID)
}

func (h *DefaultHostProvider) IOSetDeadline(handleID uint32, kind string, unixNanos int64) error {
	var t time.Time
	if unixNanos != 0 {
		t = time.Unix(0, unixNanos)
	}
	return h.io.SetDeadline(handleID, kind, t)
}

func (h *DefaultHostProvider) GetAssetConfig(assetID int64) (json.RawMessage, error) {
	if h.cfg.AssetConfigs == nil {
		return nil, fmt.Errorf("asset config getter not configured")
	}
	return h.cfg.AssetConfigs.GetAssetConfig(assetID)
}

func (h *DefaultHostProvider) FileDialog(dialogType string, opts DialogOptions) (string, error) {
	if h.cfg.FileDialogs == nil {
		return "", fmt.Errorf("file dialog opener not configured")
	}
	return h.cfg.FileDialogs.FileDialog(dialogType, opts)
}

func (h *DefaultHostProvider) Log(level, msg string) {
	if h.cfg.Logger == nil {
		return
	}
	switch level {
	case "debug":
		h.cfg.Logger.Debug(msg)
	case "info":
		h.cfg.Logger.Info(msg)
	case "warn":
		h.cfg.Logger.Warn(msg)
	case "error":
		h.cfg.Logger.Error(msg)
	default:
		h.cfg.Logger.Info(msg)
	}
}

func (h *DefaultHostProvider) KVGet(key string) ([]byte, error) {
	if h.cfg.KV == nil {
		return nil, fmt.Errorf("KV store not configured")
	}
	return h.cfg.KV.Get(key)
}

func (h *DefaultHostProvider) KVSet(key string, value []byte) error {
	if h.cfg.KV == nil {
		return fmt.Errorf("KV store not configured")
	}
	return h.cfg.KV.Set(key, value)
}

func (h *DefaultHostProvider) ActionEvent(eventType string, data json.RawMessage) error {
	if h.cfg.ActionEvents == nil {
		return nil
	}
	return h.cfg.ActionEvents.OnActionEvent(eventType, data)
}

func (h *DefaultHostProvider) ActionShouldStop() bool {
	c := h.activeCancel.Load()
	return c != nil && c.ShouldStop()
}

func (h *DefaultHostProvider) SetActiveCancellation(c *ActionCancellation) {
	h.activeCancel.Store(c)
}

func (h *DefaultHostProvider) CloseAll() {
	h.io.CloseAll()
}

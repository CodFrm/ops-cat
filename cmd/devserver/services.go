package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DevHostServices implements extruntime.HostServices with real HTTP,
// in-memory KV, and config from devconfig.json. All calls are logged.
type DevHostServices struct {
	config    *DevConfig
	kv        map[string]map[string]string
	mu        sync.RWMutex
	logStream *LogStream
}

func NewDevHostServices(config *DevConfig, logStream *LogStream) *DevHostServices {
	return &DevHostServices{
		config:    config,
		kv:        make(map[string]map[string]string),
		logStream: logStream,
	}
}

func (s *DevHostServices) KVGet(_ context.Context, ext, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.logStream.Push(LogEntry{Type: "kv_get", Extension: ext, Detail: map[string]any{"key": key}})
	if extMap, ok := s.kv[ext]; ok {
		if val, ok := extMap[key]; ok {
			return []byte(val), nil
		}
	}
	return nil, fmt.Errorf("key not found: %s", key)
}

func (s *DevHostServices) KVSet(_ context.Context, ext, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv[ext] == nil {
		s.kv[ext] = make(map[string]string)
	}
	s.kv[ext][key] = string(value)
	s.logStream.Push(LogEntry{Type: "kv_set", Extension: ext, Detail: map[string]any{"key": key, "size": len(value)}})
	return nil
}

func (s *DevHostServices) EmitEvent(event string, data any) {
	s.logStream.Push(LogEntry{Type: "event", Detail: map[string]any{"event": event, "data": data}})
}

func (s *DevHostServices) HTTPRequest(_ context.Context, method, url string, headers map[string][]string, body []byte) (int, map[string][]string, []byte, error) {
	start := time.Now()
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header = http.Header(headers)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logStream.Push(LogEntry{Type: "http", Detail: map[string]any{
			"method": method, "url": url, "error": err.Error(),
		}})
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return 0, nil, nil, err
	}
	s.logStream.Push(LogEntry{Type: "http", Detail: map[string]any{
		"method": method, "url": url, "status": resp.StatusCode,
		"duration_ms": time.Since(start).Milliseconds(), "resp_size": len(respBody),
	}})
	return resp.StatusCode, map[string][]string(resp.Header), respBody, nil
}

func (s *DevHostServices) GetCredential(_ context.Context, assetID int64) (map[string]string, error) {
	s.logStream.Push(LogEntry{Type: "credential_get", Detail: map[string]any{"asset_id": assetID}})
	asset, ok := s.config.Assets[assetID]
	if !ok {
		return nil, fmt.Errorf("asset %d not found in devconfig", assetID)
	}
	if len(asset.Credential) == 0 {
		return nil, fmt.Errorf("no credential for asset %d", assetID)
	}
	return asset.Credential, nil
}

func (s *DevHostServices) GetAssetConfig(_ context.Context, assetID int64, _ string) (json.RawMessage, error) {
	s.logStream.Push(LogEntry{Type: "asset_config", Detail: map[string]any{"asset_id": assetID}})
	asset, ok := s.config.Assets[assetID]
	if !ok {
		return nil, fmt.Errorf("asset %d not found in devconfig", assetID)
	}
	return asset.Config, nil
}

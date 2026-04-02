// cmd/devserver/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opskat/opskat/pkg/extension"
	"go.uber.org/zap"
)

func main() {
	var (
		extDir      = flag.String("ext-dir", "", "Extension dist directory (contains main.wasm + frontend/)")
		manifest    = flag.String("manifest", "", "Path to manifest.json")
		port        = flag.Int("port", 3456, "Server port")
		extFrontend = flag.String("ext-frontend", "", "Extension frontend Vite dev server URL for HMR proxy")
	)
	flag.Parse()

	logger, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(logger)

	if *extDir == "" || *manifest == "" {
		fmt.Fprintln(os.Stderr, "Usage: devserver --ext-dir <dir> --manifest <path> [--port <port>] [--ext-frontend <url>]")
		os.Exit(1)
	}

	// Parse manifest
	manifestData, err := os.ReadFile(*manifest)
	if err != nil {
		logger.Fatal("read manifest", zap.Error(err))
	}
	m, err := extension.ParseManifest(manifestData)
	if err != nil {
		logger.Fatal("parse manifest", zap.Error(err))
	}

	// Load WASM
	wasmPath := filepath.Join(*extDir, m.Backend.Binary)
	wasmBytes, err := os.ReadFile(wasmPath) //nolint:gosec // user-specified CLI argument
	if err != nil {
		logger.Fatal("read wasm", zap.String("path", wasmPath), zap.Error(err))
	}

	// Create data directory next to manifest
	dataDir := filepath.Join(filepath.Dir(*manifest), ".devserver")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Fatal("create data dir", zap.Error(err))
	}

	// Create host + plugin
	host := NewDevServerHost(dataDir)
	ctx := context.Background()
	plugin, err := extension.LoadPlugin(ctx, m, wasmBytes, host, nil)
	if err != nil {
		logger.Fatal("load plugin", zap.Error(err))
	}
	defer func() {
		if err := plugin.Close(ctx); err != nil {
			logger.Warn("close plugin", zap.Error(err))
		}
	}()

	// Start server
	srv := NewServer(m, plugin, host, *extDir, *extFrontend)
	addr := fmt.Sprintf(":%d", *port)
	logger.Info("DevServer starting",
		zap.String("extension", m.Name),
		zap.String("addr", addr),
	)
	if err := srv.ListenAndServe(addr); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

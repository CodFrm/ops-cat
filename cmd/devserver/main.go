// devserver is a lightweight development server for testing OpsKat extensions.
// It loads a WASM extension plugin and provides an HTTP API + web UI for
// full-stack testing without the Wails desktop client.
//
// Usage:
//
//	devserver --dir /path/to/extension [--config devconfig.json] [--port 3456] [--vite http://localhost:5173]
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opskat/opskat/pkg/extruntime"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	dir := flag.String("dir", ".", "Extension directory (containing manifest.json)")
	configPath := flag.String("config", "", "devconfig.json path (default: <dir>/devconfig.json)")
	port := flag.Int("port", 3456, "HTTP server port")
	viteURL := flag.String("vite", "", "Vite dev server URL for frontend HMR (e.g. http://localhost:5173)")
	flag.Parse()

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("resolve dir: %v", err)
	}

	// Load manifest
	manifestData, err := os.ReadFile(filepath.Join(absDir, "manifest.json"))
	if err != nil {
		log.Fatalf("read manifest.json: %v", err)
	}
	manifest, err := extruntime.ParseManifest(manifestData)
	if err != nil {
		log.Fatalf("parse manifest: %v", err)
	}
	ext := &extruntime.ExtensionInfo{Manifest: manifest, Dir: absDir}

	// Load devconfig
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = filepath.Join(absDir, "devconfig.json")
	}
	devCfg := &DevConfig{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := devCfg.Load(data); err != nil {
			log.Fatalf("parse devconfig: %v", err)
		}
		log.Printf("Loaded config from %s (%d assets)", cfgPath, len(devCfg.Assets))
	} else {
		log.Printf("No devconfig.json found, using empty config")
	}

	// Create log stream
	logStream := NewLogStream(256)

	// Create host services
	services := NewDevHostServices(devCfg, logStream)

	// Load WASM plugin
	ctx := context.Background()
	host := extruntime.NewExtensionHost(services)
	wasmPath := filepath.Join(absDir, manifest.Backend.Binary)
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		log.Fatalf("read WASM %s: %v", wasmPath, err)
	}
	_, err = host.LoadPluginFromBytes(ctx, manifest.Name, wasmBytes)
	if err != nil {
		log.Fatalf("load plugin: %v", err)
	}
	defer host.Close(ctx)

	// Create bridge
	bridge := extruntime.NewBridge(host,
		extruntime.WithExecutionPolicy(NewDevPolicy(devCfg)),
	)
	bridge.RegisterExtension(ext)

	// Load prompt content
	promptContent := ""
	if manifest.PromptFile != "" {
		if data, err := os.ReadFile(filepath.Join(absDir, manifest.PromptFile)); err == nil {
			promptContent = string(data)
		}
	}

	// Setup HTTP handlers
	handler := NewHandler(bridge, ext, devCfg, logStream, promptContent, cfgPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/manifest", handler.GetManifest)
	mux.HandleFunc("GET /api/config", handler.GetConfig)
	mux.HandleFunc("PUT /api/config", handler.UpdateConfig)
	mux.HandleFunc("POST /api/tool/{name}", handler.CallTool)
	mux.HandleFunc("POST /api/test-connection", handler.TestConnection)
	mux.HandleFunc("GET /api/logs", handler.LogStream)

	// Extension frontend assets
	if *viteURL != "" {
		mux.Handle("/extension/", NewViteProxy(*viteURL))
	} else {
		distDir := filepath.Join(absDir, "dist", "frontend")
		mux.Handle("/extension/", http.StripPrefix("/extension/", http.FileServer(http.Dir(distDir))))
	}

	// Dev server UI
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(staticSub)))

	log.Printf("Extension: %s v%s", manifest.Name, manifest.Version)
	log.Printf("Tools: %d, Pages: %d", len(manifest.Tools), len(manifest.Frontend.Pages))
	log.Printf("Dev server: http://localhost:%d", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

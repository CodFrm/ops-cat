package app

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ExtensionAssetHandler serves extension static files from the extensions
// directory at /extensions/{name}/..., falling back to the default handler
// for all other paths.
type ExtensionAssetHandler struct {
	extensionsDir  string
	defaultHandler http.Handler
}

// NewExtensionAssetHandler creates a handler that serves extension files.
func NewExtensionAssetHandler(extensionsDir string, defaultHandler http.Handler) *ExtensionAssetHandler {
	return &ExtensionAssetHandler{
		extensionsDir:  extensionsDir,
		defaultHandler: defaultHandler,
	}
}

func (h *ExtensionAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/extensions/") {
		if h.defaultHandler != nil {
			h.defaultHandler.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
		return
	}

	rel := strings.TrimPrefix(r.URL.Path, "/extensions/")
	if rel == "" || strings.HasPrefix(rel, "/") {
		http.NotFound(w, r)
		return
	}

	file, err := os.OpenInRoot(h.extensionsDir, filepath.FromSlash(rel))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	http.ServeContent(w, r, path.Base(rel), info.ModTime(), file)
}

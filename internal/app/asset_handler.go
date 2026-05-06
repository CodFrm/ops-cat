package app

import (
	"net/http"
	"path/filepath"
	"strings"
)

const extensionPathPrefix = "/extensions/"

// ExtensionAssetHandler serves extension static files from the extensions
// directory at /extensions/{name}/..., falling back to the default handler
// for all other paths.
type ExtensionAssetHandler struct {
	extensionsDir  string
	defaultHandler http.Handler
	fileSystem     http.FileSystem
	fileHandler    http.Handler
}

// NewExtensionAssetHandler creates a handler that serves extension files.
func NewExtensionAssetHandler(extensionsDir string, defaultHandler http.Handler) *ExtensionAssetHandler {
	cleanDir, err := filepath.Abs(extensionsDir)
	if err != nil {
		cleanDir = filepath.Clean(extensionsDir)
	}

	fileSystem := http.Dir(cleanDir)
	return &ExtensionAssetHandler{
		extensionsDir:  cleanDir,
		defaultHandler: defaultHandler,
		fileSystem:     fileSystem,
		fileHandler:    http.StripPrefix(extensionPathPrefix, http.FileServer(fileSystem)),
	}
}

func (h *ExtensionAssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, extensionPathPrefix) {
		if h.defaultHandler != nil {
			h.defaultHandler.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
		return
	}

	rel := strings.TrimPrefix(r.URL.Path, extensionPathPrefix)
	if !h.isLocalExtensionPath(rel) {
		http.NotFound(w, r)
		return
	}

	file, err := h.fileSystem.Open(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	h.fileHandler.ServeHTTP(w, r)
}

func (h *ExtensionAssetHandler) isLocalExtensionPath(rel string) bool {
	localRel := filepath.FromSlash(rel)
	if !filepath.IsLocal(localRel) {
		return false
	}

	filePath := filepath.Join(h.extensionsDir, localRel)
	relToRoot, err := filepath.Rel(h.extensionsDir, filePath)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

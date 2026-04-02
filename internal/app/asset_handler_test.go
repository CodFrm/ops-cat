package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionAssetHandler(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "test-ext", "frontend")
	_ = os.MkdirAll(extDir, 0755)
	_ = os.WriteFile(filepath.Join(extDir, "index.js"), []byte("export function Page(){}"), 0644)

	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("fallback"))
	})

	handler := NewExtensionAssetHandler(dir, fallback)

	t.Run("serves extension files", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/extensions/test-ext/frontend/index.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "export function Page(){}" {
			t.Fatalf("unexpected body: %s", body)
		}
	})

	t.Run("falls back for non-extension paths", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/other/path", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "fallback" {
			t.Fatalf("expected fallback, got: %s", body)
		}
	})

	t.Run("returns 404 for missing extension files", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/extensions/test-ext/missing.js", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

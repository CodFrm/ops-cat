package extension

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			logger.Default().Warn("close zip reader", zap.Error(err))
		}
	}()

	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.Contains(name, "..") {
			return fmt.Errorf("zip contains path traversal: %s", f.Name)
		}
		target := filepath.Join(destDir, name)

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		out, err := os.Create(target) //nolint:gosec // target validated by traversal check above
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			if closeErr := out.Close(); closeErr != nil {
				logger.Default().Warn("close output file", zap.Error(closeErr))
			}
			return err
		}

		_, err = io.Copy(out, rc) //nolint:gosec // extensions are from trusted registry
		if closeErr := rc.Close(); closeErr != nil {
			logger.Default().Warn("close zip entry reader", zap.Error(closeErr))
		}
		if closeErr := out.Close(); closeErr != nil {
			logger.Default().Warn("close output file", zap.Error(closeErr))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		data, err := os.ReadFile(path) //nolint:gosec // path from filepath.Walk within validated src directory
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644) //nolint:gosec // target derived from validated src/dst directories
	})
}

package app

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/opskat/opskat/internal/pkg/charset"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/transform"
)

// TableExportWriteOptions controls how exported table data is written to disk.
type TableExportWriteOptions struct {
	Encoding string `json:"encoding"`
	Append   bool   `json:"append"`
}

// SelectTableExportFile opens a native save dialog for table exports.
func (a *App) SelectTableExportFile(defaultFilename, filterName, pattern string) (string, error) {
	if filterName == "" {
		filterName = "Export Files"
	}
	if pattern == "" {
		pattern = "*.*"
	}
	filePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Export Data",
		DefaultFilename: defaultFilename,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: filterName, Pattern: pattern},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save file dialog failed: %w", err)
	}
	return filePath, nil
}

// WriteTableExportFile writes exported table content to the path chosen by the user.
func (a *App) WriteTableExportFile(filePath, content string, options *TableExportWriteOptions) error {
	if filePath == "" {
		return fmt.Errorf("export file path is empty")
	}
	opts := TableExportWriteOptions{}
	if options != nil {
		opts = *options
	}
	return writeTableExportFile(filePath, content, opts)
}

func writeTableExportFile(filePath, content string, options TableExportWriteOptions) (err error) {
	suppressBOM := false
	if options.Append {
		if info, statErr := os.Stat(filePath); statErr == nil && info.Size() > 0 {
			suppressBOM = true
		}
	}
	data, err := encodeTableExportContent(content, options.Encoding, suppressBOM)
	if err != nil {
		return err
	}

	flag := os.O_CREATE | os.O_WRONLY
	if options.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(filePath, flag, 0644) //nolint:gosec // user-selected export path
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	_, err = f.Write(data)
	return err
}

func encodeTableExportContent(content, name string, suppressBOM bool) ([]byte, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "utf-8-bom" || normalized == "utf8-bom" {
		if suppressBOM {
			return []byte(content), nil
		}
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(content)...), nil
	}

	enc, ok := charset.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unsupported export encoding: %s", name)
	}
	if enc == nil {
		return []byte(content), nil
	}
	data, _, err := transform.Bytes(enc.NewEncoder(), []byte(content))
	if err != nil {
		return nil, fmt.Errorf("encode export content as %s: %w", name, err)
	}
	if bom := tableExportBOM(normalized); len(bom) > 0 && !suppressBOM {
		data = append(bytes.Clone(bom), data...)
	}
	return data, nil
}

func tableExportBOM(normalized string) []byte {
	switch strings.ReplaceAll(normalized, "_", "-") {
	case "utf-16le", "utf16le":
		return []byte{0xff, 0xfe}
	case "utf-16be", "utf16be":
		return []byte{0xfe, 0xff}
	}
	return nil
}

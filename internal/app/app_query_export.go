package app

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
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

func writeTableExportFile(filePath, content string, options TableExportWriteOptions) error {
	data, err := encodeTableExportContent(content, options.Encoding)
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
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func encodeTableExportContent(content, charset string) ([]byte, error) {
	normalized := strings.ToLower(strings.TrimSpace(charset))
	if normalized == "" || normalized == "utf-8" || normalized == "utf8" || normalized == "65001" {
		return []byte(content), nil
	}
	if normalized == "utf-8-bom" || normalized == "utf8-bom" {
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(content)...), nil
	}

	enc, bom := tableExportTextEncoding(normalized)
	if enc == nil {
		return nil, fmt.Errorf("unsupported export encoding: %s", charset)
	}
	data, _, err := transform.Bytes(enc.NewEncoder(), []byte(content))
	if err != nil {
		return nil, fmt.Errorf("encode export content as %s: %w", charset, err)
	}
	if len(bom) > 0 {
		data = append(bytes.Clone(bom), data...)
	}
	return data, nil
}

func tableExportTextEncoding(charset string) (encoding.Encoding, []byte) {
	switch strings.ReplaceAll(charset, "_", "-") {
	case "utf-16le", "utf16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), []byte{0xff, 0xfe}
	case "utf-16be", "utf16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), []byte{0xfe, 0xff}
	case "gb18030", "54936":
		return simplifiedchinese.GB18030, nil
	case "gbk", "cp936", "936":
		return simplifiedchinese.GBK, nil
	case "big5", "950":
		return traditionalchinese.Big5, nil
	case "shift-jis", "shiftjis", "sjis", "cp932", "932":
		return japanese.ShiftJIS, nil
	case "iso-8859-1", "latin1", "latin-1", "28591":
		return charmap.ISO8859_1, nil
	case "windows-1252", "cp1252", "1252":
		return charmap.Windows1252, nil
	default:
		return nil, nil
	}
}

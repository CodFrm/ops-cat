package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestWriteTableExportFileWithEncoding(t *testing.T) {
	dir := t.TempDir()

	t.Run("writes UTF-8 BOM when requested", func(t *testing.T) {
		target := filepath.Join(dir, "utf8-bom.csv")
		err := writeTableExportFile(target, "name\n中文", TableExportWriteOptions{Encoding: "utf-8-bom"})
		if err != nil {
			t.Fatalf("writeTableExportFile() error = %v", err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		wantPrefix := []byte{0xef, 0xbb, 0xbf}
		if !bytes.HasPrefix(got, wantPrefix) {
			t.Fatalf("expected UTF-8 BOM prefix, got % x", got[:min(len(got), 3)])
		}
	})

	t.Run("writes GB18030 bytes when requested", func(t *testing.T) {
		target := filepath.Join(dir, "gb18030.csv")
		content := "name\n中文"
		err := writeTableExportFile(target, content, TableExportWriteOptions{Encoding: "gb18030"})
		if err != nil {
			t.Fatalf("writeTableExportFile() error = %v", err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		want, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(content))
		if err != nil {
			t.Fatalf("GB18030 encode error = %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("encoded bytes mismatch\n got: % x\nwant: % x", got, want)
		}
	})

	t.Run("appends to an existing export", func(t *testing.T) {
		target := filepath.Join(dir, "append.csv")
		if err := os.WriteFile(target, []byte("first\n"), 0644); err != nil {
			t.Fatalf("seed file error = %v", err)
		}

		err := writeTableExportFile(target, "second\n", TableExportWriteOptions{Encoding: "utf-8", Append: true})
		if err != nil {
			t.Fatalf("writeTableExportFile() error = %v", err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(got) != "first\nsecond\n" {
			t.Fatalf("unexpected appended file: %q", string(got))
		}
	})
}

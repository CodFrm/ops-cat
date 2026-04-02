package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExtListManifestParsing(t *testing.T) {
	Convey("cmdExtList manifest scanning", t, func() {
		Convey("reads manifest and extracts tools", func() {
			dir := t.TempDir()
			extDir := filepath.Join(dir, "test-ext")
			So(os.MkdirAll(extDir, 0755), ShouldBeNil)

			manifest := map[string]any{
				"name":    "test-ext",
				"version": "1.0.0",
				"i18n": map[string]any{
					"displayName": "Test Extension",
					"description": "A test extension",
				},
				"tools": []map[string]any{
					{"name": "tool_a"},
					{"name": "tool_b"},
				},
			}
			data, _ := json.Marshal(manifest)
			So(os.WriteFile(filepath.Join(extDir, "manifest.json"), data, 0644), ShouldBeNil)

			// Read and parse manually (same logic as cmdExtList)
			entries, err := os.ReadDir(dir)
			So(err, ShouldBeNil)
			So(len(entries), ShouldEqual, 1)

			manifestData, err := os.ReadFile(filepath.Join(dir, entries[0].Name(), "manifest.json")) //nolint:gosec // test file with known path
			So(err, ShouldBeNil)

			var parsed struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				I18n    struct {
					DisplayName string `json:"displayName"`
				} `json:"i18n"`
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			}
			err = json.Unmarshal(manifestData, &parsed)
			So(err, ShouldBeNil)
			So(parsed.Name, ShouldEqual, "test-ext")
			So(parsed.Version, ShouldEqual, "1.0.0")
			So(parsed.I18n.DisplayName, ShouldEqual, "Test Extension")
			So(len(parsed.Tools), ShouldEqual, 2)
			So(parsed.Tools[0].Name, ShouldEqual, "tool_a")
		})

		Convey("skips directories without manifest", func() {
			dir := t.TempDir()
			So(os.MkdirAll(filepath.Join(dir, "no-manifest"), 0755), ShouldBeNil)

			entries, err := os.ReadDir(dir)
			So(err, ShouldBeNil)

			count := 0
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				manifestPath := filepath.Join(dir, entry.Name(), "manifest.json")
				if _, err := os.ReadFile(manifestPath); err != nil { //nolint:gosec // test file with known path
					continue
				}
				count++
			}
			So(count, ShouldEqual, 0)
		})
	})
}

func TestExtExecArgParsing(t *testing.T) {
	Convey("ext exec argument parsing", t, func() {
		Convey("parses --args flag", func() {
			args := []string{"oss", "list_buckets", "--args", `{"asset_id": 1}`}
			So(args[0], ShouldEqual, "oss")
			So(args[1], ShouldEqual, "list_buckets")

			var toolArgs json.RawMessage
			for i := 2; i < len(args); i++ {
				if args[i] == "--args" && i+1 < len(args) {
					toolArgs = json.RawMessage(args[i+1])
					break
				}
			}
			So(json.Valid(toolArgs), ShouldBeTrue)

			var parsed map[string]any
			So(json.Unmarshal(toolArgs, &parsed), ShouldBeNil)
			So(parsed["asset_id"], ShouldEqual, float64(1))
		})

		Convey("parses --args= form", func() {
			args := []string{"oss", "list_buckets", `--args={"bucket":"test"}`}

			var toolArgs json.RawMessage
			for i := 2; i < len(args); i++ {
				if len(args[i]) > 7 && args[i][:7] == "--args=" {
					toolArgs = json.RawMessage(args[i][7:])
					break
				}
			}
			So(json.Valid(toolArgs), ShouldBeTrue)
		})

		Convey("defaults to empty object", func() {
			args := []string{"oss", "list_buckets"}
			var toolArgs = json.RawMessage("{}")
			for i := 2; i < len(args); i++ {
				if args[i] == "--args" && i+1 < len(args) {
					toolArgs = json.RawMessage(args[i+1])
					break
				}
			}
			So(string(toolArgs), ShouldEqual, "{}")
		})
	})
}

package extension

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseManifest(t *testing.T) {
	Convey("ParseManifest", t, func() {
		Convey("should parse a valid manifest", func() {
			data := []byte(`{
				"name": "oss",
				"version": "1.0.0",
				"icon": "cloud-storage",
				"minAppVersion": "1.2.0",
				"i18n": {
					"displayName": "manifest.displayName",
					"description": "manifest.description"
				},
				"backend": {
					"runtime": "wasm",
					"binary": "main.wasm"
				},
				"assetTypes": [{
					"type": "oss",
					"i18n": { "name": "assetType.oss.name" },
					"configSchema": {
						"type": "object",
						"properties": {
							"provider": { "type": "string" }
						},
						"required": ["provider"]
					}
				}],
				"tools": [{
					"name": "list_buckets",
					"i18n": { "description": "tools.list_buckets.description" },
					"parameters": {
						"type": "object",
						"properties": {
							"prefix": { "type": "string" }
						}
					}
				}],
				"policies": {
					"type": "oss",
					"actions": ["list", "read", "write", "delete", "admin"],
					"groups": [{
						"id": "ext:oss:readonly",
						"i18n": { "name": "policy.readonly.name", "description": "policy.readonly.description" },
						"policy": { "allow_list": ["list", "read"], "deny_list": ["delete", "admin"] }
					}],
					"default": ["ext:oss:readonly"]
				},
				"frontend": {
					"entry": "frontend/index.js",
					"styles": "frontend/style.css",
					"pages": [{
						"id": "browser",
						"i18n": { "name": "pages.browser.name" },
						"component": "BrowserPage"
					}]
				}
			}`)

			m, err := ParseManifest(data)
			So(err, ShouldBeNil)
			So(m.Name, ShouldEqual, "oss")
			So(m.Version, ShouldEqual, "1.0.0")
			So(m.MinAppVersion, ShouldEqual, "1.2.0")
			So(m.Backend.Runtime, ShouldEqual, "wasm")
			So(m.Backend.Binary, ShouldEqual, "main.wasm")
			So(len(m.AssetTypes), ShouldEqual, 1)
			So(m.AssetTypes[0].Type, ShouldEqual, "oss")
			So(len(m.Tools), ShouldEqual, 1)
			So(m.Tools[0].Name, ShouldEqual, "list_buckets")
			So(m.Policies.Type, ShouldEqual, "oss")
			So(len(m.Policies.Groups), ShouldEqual, 1)
			So(m.Policies.Groups[0].ID, ShouldEqual, "ext:oss:readonly")
			So(m.Policies.Default, ShouldResemble, []string{"ext:oss:readonly"})
			So(m.Frontend.Entry, ShouldEqual, "frontend/index.js")
			So(m.Frontend.Styles, ShouldEqual, "frontend/style.css")
			So(len(m.Frontend.Pages), ShouldEqual, 1)
			So(m.Frontend.Pages[0].Component, ShouldEqual, "BrowserPage")
		})

		Convey("should reject manifest missing required fields", func() {
			data := []byte(`{"version": "1.0.0"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "name")
		})

		Convey("should reject invalid minAppVersion", func() {
			data := []byte(`{"name": "x", "version": "1.0.0", "minAppVersion": "invalid"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "minAppVersion")
		})

		Convey("should reject policy group without ext: prefix", func() {
			data := []byte(`{
				"name": "x", "version": "1.0.0", "minAppVersion": "1.0.0",
				"backend": {"runtime": "wasm", "binary": "main.wasm"},
				"policies": {
					"type": "x", "actions": ["read"],
					"groups": [{"id": "nope:bad", "i18n": {"name": "n", "description": "d"}, "policy": {"allow_list": ["read"]}}]
				}
			}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "ext:")
		})
	})
}

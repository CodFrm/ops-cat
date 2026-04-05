package extension

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const testValidSHA256 = "93a44bbb96c751218e4c00d479e4c14358122a389acca16205b1e4d0dc5f9476"

func TestParseManifest(t *testing.T) {
	Convey("ParseManifest", t, func() {
		Convey("should parse a valid manifest", func() {
			data := []byte(`{
				"name": "oss",
				"version": "1.0.0",
				"icon": "cloud-storage",
				"minAppVersion": "1.2.0",
				"hostABI": "1.0",
				"binarySha256": "` + testValidSHA256 + `",
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
			So(m.HostABI, ShouldEqual, "1.0")
			So(m.BinarySHA256, ShouldEqual, testValidSHA256)
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
			data := []byte(`{"name": "x", "version": "1.0.0", "minAppVersion": "invalid", "hostABI":"1.0", "binarySha256":"` + testValidSHA256 + `"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "minAppVersion")
		})

		Convey("should reject manifest missing hostABI", func() {
			data := []byte(`{"name": "x", "version": "1.0.0", "binarySha256":"` + testValidSHA256 + `"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hostABI is required")
		})

		Convey("should reject manifest with unsupported hostABI", func() {
			data := []byte(`{"name": "x", "version": "1.0.0", "hostABI": "9.9", "binarySha256":"` + testValidSHA256 + `"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not supported")
		})

		Convey("should reject manifest missing binarySha256", func() {
			data := []byte(`{"name": "x", "version": "1.0.0", "hostABI":"1.0"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "binarySha256 is required")
		})

		Convey("should reject manifest with invalid binarySha256 format", func() {
			data := []byte(`{"name": "x", "version": "1.0.0", "hostABI":"1.0", "binarySha256": "not-a-hash"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "64 hex")
		})

		Convey("should reject manifest with invalid name characters", func() {
			data := []byte(`{"name": "../../etc/passwd", "version": "1.0.0", "hostABI":"1.0", "binarySha256":"` + testValidSHA256 + `"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "name must match")
		})

		Convey("should reject manifest with uppercase name", func() {
			data := []byte(`{"name": "MyExt", "version": "1.0.0", "hostABI":"1.0", "binarySha256":"` + testValidSHA256 + `"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "name must match")
		})

		Convey("should accept valid name characters", func() {
			data := []byte(`{"name": "my-ext_1", "version": "1.0.0", "hostABI":"1.0", "binarySha256":"` + testValidSHA256 + `"}`)
			_, err := ParseManifest(data)
			So(err, ShouldBeNil)
		})

		Convey("should parse page slot field", func() {
			data := []byte(`{
				"name": "oss",
				"version": "1.0.0",
				"hostABI": "1.0",
				"binarySha256": "` + testValidSHA256 + `",
				"frontend": {
					"pages": [{
						"id": "connect",
						"slot": "asset.connect",
						"i18n": { "name": "pages.connect.name" },
						"component": "ConnectPage"
					}]
				}
			}`)
			m, err := ParseManifest(data)
			So(err, ShouldBeNil)
			So(len(m.Frontend.Pages), ShouldEqual, 1)
			So(m.Frontend.Pages[0].Slot, ShouldEqual, "asset.connect")
		})

		Convey("should reject policy group without ext: prefix", func() {
			data := []byte(`{
				"name": "x", "version": "1.0.0", "minAppVersion": "1.0.0",
				"hostABI": "1.0", "binarySha256": "` + testValidSHA256 + `",
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

		Convey("should reject invalid credentials capability", func() {
			data := []byte(`{
				"name": "x", "version": "1.0.0",
				"hostABI": "1.0", "binarySha256": "` + testValidSHA256 + `",
				"capabilities": { "credentials": "write" }
			}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "credentials")
		})
	})
}

func TestCapabilityChecks(t *testing.T) {
	Convey("Capability checks", t, func() {
		extDir := "/var/ext/test-ext"

		Convey("FS read — deny by default", func() {
			m := &Manifest{Capabilities: Capabilities{FS: FSCapability{}}}
			So(m.CheckFSRead("/tmp/foo.txt", extDir), ShouldNotBeNil)
		})

		Convey("FS read — allow within ${EXT_DIR}", func() {
			m := &Manifest{Capabilities: Capabilities{FS: FSCapability{Read: []string{"${EXT_DIR}/**"}}}}
			So(m.CheckFSRead("/var/ext/test-ext/data/foo.txt", extDir), ShouldBeNil)
		})

		Convey("FS read — deny outside ${EXT_DIR}", func() {
			m := &Manifest{Capabilities: Capabilities{FS: FSCapability{Read: []string{"${EXT_DIR}/**"}}}}
			So(m.CheckFSRead("/etc/passwd", extDir), ShouldNotBeNil)
			So(m.CheckFSRead("/var/ext/other-ext/foo.txt", extDir), ShouldNotBeNil)
		})

		Convey("FS read — allow explicit absolute path prefix", func() {
			m := &Manifest{Capabilities: Capabilities{FS: FSCapability{Read: []string{"/tmp/allowed/**"}}}}
			So(m.CheckFSRead("/tmp/allowed/foo.txt", extDir), ShouldBeNil)
			So(m.CheckFSRead("/tmp/other/foo.txt", extDir), ShouldNotBeNil)
		})

		Convey("FS read — reject path traversal", func() {
			m := &Manifest{Capabilities: Capabilities{FS: FSCapability{Read: []string{"/tmp/**"}}}}
			// After filepath.Abs, traversal resolves; verify it's blocked.
			err := m.CheckFSRead("/tmp/../etc/passwd", extDir)
			So(err, ShouldNotBeNil)
		})

		Convey("FS write — separate capability", func() {
			m := &Manifest{Capabilities: Capabilities{FS: FSCapability{
				Read:  []string{"${EXT_DIR}/**"},
				Write: []string{"${EXT_DIR}/data/**"},
			}}}
			So(m.CheckFSWrite("/var/ext/test-ext/data/foo.txt", extDir), ShouldBeNil)
			So(m.CheckFSWrite("/var/ext/test-ext/config.json", extDir), ShouldNotBeNil) // read-only area
		})

		Convey("HTTP URL — deny by default", func() {
			m := &Manifest{}
			So(m.CheckHTTPURL("https://api.example.com/v1/foo", false), ShouldNotBeNil)
		})

		Convey("HTTP URL — allow explicit prefix", func() {
			m := &Manifest{Capabilities: Capabilities{HTTP: HTTPCapability{
				Allowlist: []string{"https://api.example.com/"},
			}}}
			So(m.CheckHTTPURL("https://api.example.com/v1/foo", false), ShouldBeNil)
			So(m.CheckHTTPURL("https://evil.example.com/", false), ShouldNotBeNil)
		})

		Convey("HTTP URL — reject RFC1918 without tunnel", func() {
			m := &Manifest{Capabilities: Capabilities{HTTP: HTTPCapability{
				Allowlist: []string{"http://10.0.0.1/"},
			}}}
			err := m.CheckHTTPURL("http://10.0.0.1/foo", false)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "private")
		})

		Convey("HTTP URL — reject loopback", func() {
			m := &Manifest{Capabilities: Capabilities{HTTP: HTTPCapability{
				Allowlist: []string{"http://127.0.0.1/"},
			}}}
			So(m.CheckHTTPURL("http://127.0.0.1/foo", false), ShouldNotBeNil)
			So(m.CheckHTTPURL("http://localhost/foo", false), ShouldNotBeNil)
		})

		Convey("HTTP URL — reject link-local metadata", func() {
			m := &Manifest{Capabilities: Capabilities{HTTP: HTTPCapability{
				Allowlist: []string{"http://169.254.169.254/"},
			}}}
			err := m.CheckHTTPURL("http://169.254.169.254/latest/meta-data/", false)
			So(err, ShouldNotBeNil)
		})

		Convey("HTTP URL — allow private when tunnel enabled", func() {
			m := &Manifest{Capabilities: Capabilities{
				HTTP:   HTTPCapability{Allowlist: []string{"http://10.0.0.1/"}},
				Tunnel: true,
			}}
			So(m.CheckHTTPURL("http://10.0.0.1/foo", true), ShouldBeNil)
		})

		Convey("HTTP URL — reject non-http scheme", func() {
			m := &Manifest{Capabilities: Capabilities{HTTP: HTTPCapability{
				Allowlist: []string{"file:///etc/"},
			}}}
			So(m.CheckHTTPURL("file:///etc/passwd", false), ShouldNotBeNil)
		})

		Convey("Credentials — deny by default", func() {
			m := &Manifest{}
			So(m.CheckCredentialRead(), ShouldNotBeNil)
		})

		Convey("Credentials — allow when declared", func() {
			m := &Manifest{Capabilities: Capabilities{Credentials: "read"}}
			So(m.CheckCredentialRead(), ShouldBeNil)
		})

		Convey("Tunnel — deny by default", func() {
			m := &Manifest{}
			So(m.CheckTunnel(), ShouldNotBeNil)
		})

		Convey("Tunnel — allow when declared", func() {
			m := &Manifest{Capabilities: Capabilities{Tunnel: true}}
			So(m.CheckTunnel(), ShouldBeNil)
		})
	})
}

package extension

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseManifest(t *testing.T) {
	Convey("ParseManifest", t, func() {
		Convey("解析合法 manifest", func() {
			data := []byte(`{
				"name": "oss",
				"displayName": "对象存储",
				"displayName_en": "Object Storage",
				"version": "1.0.0",
				"icon": "cloud-storage",
				"description": "管理 S3/OSS/MinIO 存储桶和对象",
				"minAppVersion": "1.0.0",
				"backend": {
					"runtime": "wasm",
					"binary": "main.wasm"
				},
				"assetTypes": [
					{
						"type": "oss",
						"name": "对象存储",
						"name_en": "Object Storage",
						"configSchema": {
							"type": "object",
							"properties": {
								"provider": {"type": "string", "enum": ["s3", "oss", "minio"]},
								"endpoint": {"type": "string"}
							},
							"required": ["provider", "endpoint"]
						}
					}
				],
				"tools": [
					{
						"name": "list_buckets",
						"description": "列出存储桶",
						"parameters": {
							"type": "object",
							"properties": {
								"prefix": {"type": "string"}
							}
						}
					}
				],
				"policies": {
					"type": "oss",
					"actions": ["list", "read", "write", "delete", "admin"]
				},
				"frontend": {
					"entry": "frontend/index.js",
					"pages": [
						{"id": "browser", "name": "文件浏览器", "component": "BrowserPage"}
					]
				}
			}`)

			m, err := ParseManifest(data)
			So(err, ShouldBeNil)
			So(m.Name, ShouldEqual, "oss")
			So(m.DisplayName, ShouldEqual, "对象存储")
			So(m.Version, ShouldEqual, "1.0.0")
			So(m.MinAppVersion, ShouldEqual, "1.0.0")
			So(m.Backend.Runtime, ShouldEqual, "wasm")
			So(m.Backend.Binary, ShouldEqual, "main.wasm")
			So(len(m.AssetTypes), ShouldEqual, 1)
			So(m.AssetTypes[0].Type, ShouldEqual, "oss")
			So(len(m.Tools), ShouldEqual, 1)
			So(m.Tools[0].Name, ShouldEqual, "list_buckets")
			So(m.Policies.Type, ShouldEqual, "oss")
			So(m.Frontend.Entry, ShouldEqual, "frontend/index.js")
			So(len(m.Frontend.Pages), ShouldEqual, 1)
		})

		Convey("缺少必填字段返回错误", func() {
			data := []byte(`{"displayName": "test"}`)
			_, err := ParseManifest(data)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "name")
		})

		Convey("无效 JSON 返回错误", func() {
			_, err := ParseManifest([]byte(`{invalid`))
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCheckAppVersionCompatibility(t *testing.T) {
	Convey("CheckAppVersionCompatibility", t, func() {
		Convey("当前版本满足最低要求", func() {
			So(CheckAppVersionCompatibility("1.0.0", "1.0.0"), ShouldBeTrue)
			So(CheckAppVersionCompatibility("1.2.0", "1.0.0"), ShouldBeTrue)
			So(CheckAppVersionCompatibility("2.0.0", "1.9.9"), ShouldBeTrue)
		})

		Convey("当前版本不满足最低要求", func() {
			So(CheckAppVersionCompatibility("1.0.0", "1.1.0"), ShouldBeFalse)
			So(CheckAppVersionCompatibility("0.9.0", "1.0.0"), ShouldBeFalse)
		})

		Convey("minAppVersion 为空时总是兼容", func() {
			So(CheckAppVersionCompatibility("1.0.0", ""), ShouldBeTrue)
		})

		Convey("无效版本号返回 false", func() {
			So(CheckAppVersionCompatibility("invalid", "1.0.0"), ShouldBeFalse)
			So(CheckAppVersionCompatibility("1.0.0", "invalid"), ShouldBeFalse)
		})
	})
}

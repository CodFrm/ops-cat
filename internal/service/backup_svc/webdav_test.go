package backup_svc

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWebDAVBackups(t *testing.T) {
	Convey("WebDAV backups", t, func() {
		var putPath string
		var putBody []byte
		var putUser, putPass string
		var mkcolPath string
		var propfindPath string
		var propfindUser, propfindPass, propfindDepth string
		var propfindAuthOK bool
		var getPath, getUser, getPass string
		var getAuthOK bool

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "PROPFIND":
				propfindPath = r.URL.Path
				propfindUser, propfindPass, propfindAuthOK = r.BasicAuth()
				propfindDepth = r.Header.Get("Depth")
				w.WriteHeader(207)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/opskat/</d:href>
    <d:propstat>
      <d:prop><d:resourcetype><d:collection /></d:resourcetype></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/opskat/opskat-backup.encrypted.json</d:href>
    <d:propstat>
      <d:prop>
        <d:getcontentlength>12</d:getcontentlength>
        <d:getlastmodified>Sat, 25 Apr 2026 10:00:00 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/opskat/opskat-backup-20260425.json</d:href>
    <d:propstat>
      <d:prop><d:getcontentlength>7</d:getcontentlength></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/opskat/notes.txt</d:href>
    <d:propstat>
      <d:prop><d:getcontentlength>5</d:getcontentlength></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
			case "PUT":
				putPath = r.URL.Path
				putUser, putPass, _ = r.BasicAuth()
				putBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusCreated)
			case "MKCOL":
				mkcolPath = r.URL.Path
				w.WriteHeader(http.StatusCreated)
			case "GET":
				getPath = r.URL.Path
				getUser, getPass, getAuthOK = r.BasicAuth()
				_, _ = w.Write([]byte("backup-bytes"))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		defer srv.Close()

		cfg := WebDAVConfig{
			URL:      srv.URL + "/dav/opskat/",
			AuthType: WebDAVAuthBasic,
			Username: "dav-user",
			Password: "dav-pass",
		}

		Convey("uploads the canonical encrypted backup file", func() {
			info, err := CreateOrUpdateWebDAVBackup(cfg, []byte("backup-bytes"))
			So(err, ShouldBeNil)
			So(info.Name, ShouldEqual, "opskat-backup.encrypted.json")
			So(info.Size, ShouldEqual, len("backup-bytes"))
			So(mkcolPath, ShouldEqual, "/dav/opskat/")
			So(putPath, ShouldEqual, "/dav/opskat/opskat-backup.encrypted.json")
			So(string(putBody), ShouldEqual, "backup-bytes")
			So(putUser, ShouldEqual, "dav-user")
			So(putPass, ShouldEqual, "dav-pass")
		})

		Convey("lists only encrypted OpsKat backup files", func() {
			backups, err := ListWebDAVBackups(cfg)
			So(err, ShouldBeNil)
			// 仅匹配 .encrypted.json，明文的 opskat-backup-20260425.json 与 notes.txt 都被过滤掉。
			So(backups, ShouldHaveLength, 1)
			So(propfindAuthOK, ShouldBeTrue)
			So(propfindUser, ShouldEqual, "dav-user")
			So(propfindPass, ShouldEqual, "dav-pass")
			So(propfindDepth, ShouldEqual, "1")
			So(backups[0].Name, ShouldEqual, "opskat-backup.encrypted.json")
			So(backups[0].Size, ShouldEqual, int64(12))
			So(backups[0].UpdatedAt, ShouldEqual, "Sat, 25 Apr 2026 10:00:00 GMT")
		})

		Convey("uses opskat as the default storage directory", func() {
			rootCfg := cfg
			rootCfg.URL = srv.URL + "/dav/"

			info, err := CreateOrUpdateWebDAVBackup(rootCfg, []byte("backup-bytes"))
			So(err, ShouldBeNil)
			So(info.Path, ShouldEndWith, "/dav/opskat/opskat-backup.encrypted.json")
			So(mkcolPath, ShouldEqual, "/dav/opskat/")
			So(putPath, ShouldEqual, "/dav/opskat/opskat-backup.encrypted.json")

			_, err = ListWebDAVBackups(rootCfg)
			So(err, ShouldBeNil)
			So(propfindPath, ShouldEqual, "/dav/opskat/")
		})

		Convey("downloads a selected backup file", func() {
			content, err := GetWebDAVBackupContent(cfg, "opskat-backup.encrypted.json")
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "backup-bytes")
			So(getPath, ShouldEqual, "/dav/opskat/opskat-backup.encrypted.json")
			So(getAuthOK, ShouldBeTrue)
			So(getUser, ShouldEqual, "dav-user")
			So(getPass, ShouldEqual, "dav-pass")
		})
	})
}

func TestValidateWebDAVURL(t *testing.T) {
	Convey("ValidateWebDAVURL", t, func() {
		Convey("accepts plain http and https URLs", func() {
			So(ValidateWebDAVURL("http://example.com/dav/"), ShouldBeNil)
			So(ValidateWebDAVURL("https://example.com/dav/opskat/"), ShouldBeNil)
		})
		Convey("rejects empty URL", func() {
			err := ValidateWebDAVURL("")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "WebDAV URL is required")
		})
		Convey("rejects URLs with non-http(s) scheme", func() {
			err := ValidateWebDAVURL("ftp://example.com/dav/")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "http://")
		})
		Convey("rejects URLs missing host", func() {
			err := ValidateWebDAVURL("https:///dav/")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "host")
		})
		Convey("rejects URLs containing userinfo to avoid storing plaintext credentials", func() {
			err := ValidateWebDAVURL("https://user:pass@example.com/dav/")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "credentials")

			err = ValidateWebDAVURL("https://user@example.com/dav/")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "credentials")
		})
	})
}

func TestWebDAVFileURLRejectsPathTraversal(t *testing.T) {
	Convey("webDAVFileURL rejects path-traversal-like names", t, func() {
		base := "https://example.com/dav/"
		Convey("accepts simple filenames", func() {
			out, err := webDAVFileURL(base, "opskat-backup.encrypted.json")
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "https://example.com/dav/opskat/opskat-backup.encrypted.json")
		})
		Convey("rejects names containing '/'", func() {
			_, err := webDAVFileURL(base, "foo/bar.json")
			So(err, ShouldNotBeNil)
		})
		Convey("rejects names containing '\\'", func() {
			_, err := webDAVFileURL(base, `foo\bar.json`)
			So(err, ShouldNotBeNil)
		})
		Convey("rejects '.' and '..'", func() {
			_, err := webDAVFileURL(base, ".")
			So(err, ShouldNotBeNil)
			_, err = webDAVFileURL(base, "..")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestWebDAVNameFromHref(t *testing.T) {
	Convey("webDAVNameFromHref", t, func() {
		Convey("returns the unescaped filename for a normal href", func() {
			So(webDAVNameFromHref("/dav/opskat/opskat-backup.encrypted.json"),
				ShouldEqual, "opskat-backup.encrypted.json")
		})
		Convey("handles trailing slash on directory hrefs", func() {
			So(webDAVNameFromHref("/dav/opskat/"), ShouldEqual, "opskat")
		})
		Convey("returns empty when href encodes a path separator", func() {
			// %2F 解码后是 '/'，会被 webDAVFileURL 拒绝，所以这里直接返回空名让上层过滤掉。
			So(webDAVNameFromHref("/dav/opskat/foo%2Fbar.encrypted.json"), ShouldEqual, "")
		})
		Convey("returns the unescaped name for normal escaped chars", func() {
			// %20 等普通转义不引入路径分隔符，正常解码返回。
			So(webDAVNameFromHref("/dav/opskat/opskat-backup%20copy.encrypted.json"),
				ShouldEqual, "opskat-backup copy.encrypted.json")
		})
		Convey("returns the dated encrypted backup name unchanged", func() {
			So(webDAVNameFromHref("/dav/opskat/opskat-backup-20260425.encrypted.json"),
				ShouldEqual, "opskat-backup-20260425.encrypted.json")
		})
	})
}

func TestIsOpsKatBackupName(t *testing.T) {
	Convey("isOpsKatBackupName", t, func() {
		Convey("matches the canonical encrypted backup filename", func() {
			So(isOpsKatBackupName("opskat-backup.encrypted.json"), ShouldBeTrue)
		})
		Convey("matches dated encrypted backups", func() {
			So(isOpsKatBackupName("opskat-backup-20260425.encrypted.json"), ShouldBeTrue)
		})
		Convey("rejects plaintext local exports", func() {
			// ExportToFile 在不带密码时产生 opskat-backup-YYYYMMDD.json（明文），
			// 这种文件不应被 ImportFromWebDAV 选中并尝试 DecryptBackup。
			So(isOpsKatBackupName("opskat-backup-20260425.json"), ShouldBeFalse)
		})
		Convey("rejects empty and unrelated names", func() {
			So(isOpsKatBackupName(""), ShouldBeFalse)
			So(isOpsKatBackupName("notes.txt"), ShouldBeFalse)
			So(isOpsKatBackupName("backup.encrypted.json"), ShouldBeFalse)
		})
	})
}

func TestEnsureWebDAVDirectoryAcceptsConflict(t *testing.T) {
	Convey("ensureWebDAVDirectory treats 409/405 as 'directory already exists'", t, func() {
		mkcolStatus := http.StatusCreated
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "MKCOL" {
				w.WriteHeader(mkcolStatus)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		}))
		defer srv.Close()

		cfg := WebDAVConfig{URL: srv.URL + "/dav/opskat/"}

		Convey("201 Created succeeds", func() {
			mkcolStatus = http.StatusCreated
			So(ensureWebDAVDirectory(cfg, srv.URL+"/dav/opskat/"), ShouldBeNil)
		})
		Convey("405 Method Not Allowed (RFC 4918 §9.3.1) succeeds", func() {
			mkcolStatus = http.StatusMethodNotAllowed
			So(ensureWebDAVDirectory(cfg, srv.URL+"/dav/opskat/"), ShouldBeNil)
		})
		Convey("409 Conflict succeeds (Nextcloud/ownCloud 等服务器在目录已存在时返回 409)", func() {
			mkcolStatus = http.StatusConflict
			So(ensureWebDAVDirectory(cfg, srv.URL+"/dav/opskat/"), ShouldBeNil)
		})
		Convey("403 Forbidden fails", func() {
			mkcolStatus = http.StatusForbidden
			So(ensureWebDAVDirectory(cfg, srv.URL+"/dav/opskat/"), ShouldNotBeNil)
		})
	})
}

func TestWebDAVRequestRejectsRedirects(t *testing.T) {
	Convey("webDAVRequest does not silently follow redirects", t, func() {
		var putHits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PUT" {
				putHits++
				w.Header().Set("Location", "https://elsewhere.example/dav/opskat/opskat-backup.encrypted.json")
				w.WriteHeader(http.StatusFound)
				return
			}
			if r.Method == "MKCOL" {
				w.WriteHeader(http.StatusCreated)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		}))
		defer srv.Close()

		cfg := WebDAVConfig{URL: srv.URL + "/dav/opskat/"}
		_, err := CreateOrUpdateWebDAVBackup(cfg, []byte("payload"))
		So(err, ShouldNotBeNil)
		// 默认 http.Client 会自动跟随 302 并把 PUT 改成 GET，导致备份内容丢失；
		// CheckRedirect 拦截后这里应直接返回错误且只发出 1 次 PUT。
		So(err.Error(), ShouldContainSubstring, "redirect")
		So(putHits, ShouldEqual, 1)
	})
}

func TestTestWebDAVConnectionVerifiesWriteCapability(t *testing.T) {
	Convey("TestWebDAVConnection", t, func() {
		Convey("performs MKCOL + PUT(probe) + DELETE(probe) and reports success on writable servers", func() {
			var mkcolHits, putHits, deleteHits int
			var lastPutPath, lastDeletePath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "MKCOL":
					mkcolHits++
					w.WriteHeader(http.StatusCreated)
				case "PUT":
					putHits++
					lastPutPath = r.URL.Path
					w.WriteHeader(http.StatusCreated)
				case "DELETE":
					deleteHits++
					lastDeletePath = r.URL.Path
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer srv.Close()

			cfg := WebDAVConfig{URL: srv.URL + "/dav/opskat/"}
			err := TestWebDAVConnection(cfg)
			So(err, ShouldBeNil)
			So(mkcolHits, ShouldEqual, 1)
			So(putHits, ShouldEqual, 1)
			So(deleteHits, ShouldEqual, 1)
			So(strings.HasPrefix(lastPutPath, "/dav/opskat/.opskat-webdav-connection-test-"), ShouldBeTrue)
			So(lastPutPath, ShouldEqual, lastDeletePath)
		})

		Convey("reports failure when PUT is rejected (e.g. read-only server)", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "MKCOL":
					w.WriteHeader(http.StatusMethodNotAllowed)
				case "PUT":
					w.WriteHeader(http.StatusForbidden)
				case "DELETE":
					w.WriteHeader(http.StatusForbidden)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer srv.Close()

			cfg := WebDAVConfig{URL: srv.URL + "/dav/opskat/"}
			err := TestWebDAVConnection(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, fmt.Sprintf("HTTP %d", http.StatusForbidden))
		})

		Convey("reports failure when MKCOL is rejected with a non-success non-conflict status", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			}))
			defer srv.Close()

			cfg := WebDAVConfig{URL: srv.URL + "/dav/opskat/"}
			err := TestWebDAVConnection(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "create directory failed")
		})

		Convey("reports failure when DELETE cleanup fails after a successful PUT", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "MKCOL":
					w.WriteHeader(http.StatusCreated)
				case "PUT":
					w.WriteHeader(http.StatusCreated)
				case "DELETE":
					// 服务器允许写但拒绝删除（罕见但可能：例如无 unlink 权限）。
					w.WriteHeader(http.StatusForbidden)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer srv.Close()

			cfg := WebDAVConfig{URL: srv.URL + "/dav/opskat/"}
			err := TestWebDAVConnection(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "cleanup failed")
		})
	})
}

func TestListWebDAVBackupsHandlesNotFound(t *testing.T) {
	Convey("ListWebDAVBackups returns an empty list when the directory does not exist (HTTP 404)", t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		backups, err := ListWebDAVBackups(WebDAVConfig{URL: srv.URL + "/dav/opskat/"})
		So(err, ShouldBeNil)
		So(backups, ShouldBeEmpty)
	})

	Convey("ListWebDAVBackups surfaces unexpected errors (HTTP 500)", t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := ListWebDAVBackups(WebDAVConfig{URL: srv.URL + "/dav/opskat/"})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "list failed")
	})
}

func TestCreateOrUpdateWebDAVBackupReportsUploadFailure(t *testing.T) {
	Convey("CreateOrUpdateWebDAVBackup surfaces a non-success PUT status", t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "MKCOL":
				w.WriteHeader(http.StatusCreated)
			case "PUT":
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("forbidden"))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		defer srv.Close()

		_, err := CreateOrUpdateWebDAVBackup(WebDAVConfig{URL: srv.URL + "/dav/opskat/"}, []byte("payload"))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "upload failed")
	})
}

func TestGetWebDAVBackupContentReportsDownloadFailure(t *testing.T) {
	Convey("GetWebDAVBackupContent surfaces a non-success GET status", t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := GetWebDAVBackupContent(WebDAVConfig{URL: srv.URL + "/dav/opskat/"}, "opskat-backup.encrypted.json")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "download failed")
	})
}

func TestWebDAVRequestHandlesRedirectWithoutLocation(t *testing.T) {
	Convey("webDAVRequest reports a generic redirect error when Location header is missing", t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 故意不写 Location，验证另一个错误路径。
			w.WriteHeader(http.StatusMovedPermanently)
		}))
		defer srv.Close()

		_, _, err := webDAVRequest(WebDAVConfig{URL: srv.URL}, http.MethodGet, srv.URL+"/probe", nil, nil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "redirect")
	})
}

func TestWebDAVFileURLDefaultsToCanonicalName(t *testing.T) {
	Convey("webDAVFileURL substitutes the canonical encrypted backup filename when name is empty", t, func() {
		out, err := webDAVFileURL("https://example.com/dav/", "")
		So(err, ShouldBeNil)
		So(out, ShouldEndWith, "/opskat/"+webDAVBackupFilename)
	})
}

func TestWebDAVStoragePathDefaultsToOpskatAtRoot(t *testing.T) {
	Convey("webDAVStoragePath returns /opskat/ when the base URL has no path", t, func() {
		So(webDAVStoragePath(""), ShouldEqual, "/opskat/")
	})
	Convey("webDAVStoragePath keeps the configured directory if it already ends in /opskat", t, func() {
		So(webDAVStoragePath("/dav/opskat"), ShouldEqual, "/dav/opskat/")
	})
	Convey("webDAVStoragePath appends /opskat/ to a custom base path", t, func() {
		So(webDAVStoragePath("/dav"), ShouldEqual, "/dav/opskat/")
	})
}

func TestGetWebDAVBackupContentRejectsInvalidName(t *testing.T) {
	Convey("GetWebDAVBackupContent refuses names that contain path separators", t, func() {
		_, err := GetWebDAVBackupContent(WebDAVConfig{URL: "https://example.com/dav/"}, "../etc/passwd")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "invalid")
	})
}

func TestBestPropFallback(t *testing.T) {
	Convey("bestProp returns the first propstat when none reports 200", t, func() {
		body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/dav/opskat/opskat-backup.encrypted.json</d:href>
    <d:propstat>
      <d:prop><d:getcontentlength>9</d:getcontentlength></d:prop>
      <d:status>HTTP/1.1 404 Not Found</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`)
		// 解析后 backups 会被 isOpsKatBackupName 接受，但 bestProp 走 fallback 返回第一个 propstat。
		backups, err := parseWebDAVBackupList(body)
		So(err, ShouldBeNil)
		So(backups, ShouldHaveLength, 1)
		So(backups[0].Size, ShouldEqual, int64(9))
	})
}

func TestParseWebDAVBackupListSkipsBadEntries(t *testing.T) {
	Convey("parseWebDAVBackupList tolerates entries with empty href and non-200 propstat", t, func() {
		body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href></d:href>
    <d:propstat>
      <d:prop><d:getcontentlength>9</d:getcontentlength></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
  <d:response>
    <d:href>/dav/opskat/opskat-backup.encrypted.json</d:href>
    <d:propstat>
      <d:prop><d:getcontentlength>0</d:getcontentlength></d:prop>
      <d:status>HTTP/1.1 404 Not Found</d:status>
    </d:propstat>
    <d:propstat>
      <d:prop>
        <d:getcontentlength>21</d:getcontentlength>
        <d:getlastmodified>Sun, 26 Apr 2026 10:00:00 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`)
		backups, err := parseWebDAVBackupList(body)
		So(err, ShouldBeNil)
		So(backups, ShouldHaveLength, 1)
		So(backups[0].Size, ShouldEqual, int64(21)) // bestProp 选中 200 OK 的 propstat
	})

	Convey("parseWebDAVBackupList errors on invalid XML", t, func() {
		_, err := parseWebDAVBackupList([]byte("not xml"))
		So(err, ShouldNotBeNil)
	})
}

func TestValidateWebDAVConfig(t *testing.T) {
	Convey("ValidateWebDAVConfig", t, func() {
		base := WebDAVConfig{URL: "https://example.com/dav/"}

		Convey("none 仅校验 URL", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthNone
			So(ValidateWebDAVConfig(cfg), ShouldBeNil)
		})

		Convey("basic 缺 username 报错", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthBasic
			cfg.Password = "s3cret"
			err := ValidateWebDAVConfig(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "username")
		})

		Convey("basic 缺 password 报错", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthBasic
			cfg.Username = "alice"
			err := ValidateWebDAVConfig(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "password")
		})

		Convey("basic 用户名+密码齐全通过", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthBasic
			cfg.Username = "alice"
			cfg.Password = "s3cret"
			So(ValidateWebDAVConfig(cfg), ShouldBeNil)
		})

		Convey("bearer 缺 token 报错", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthBearer
			err := ValidateWebDAVConfig(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "token")
		})

		Convey("bearer 有 token 通过", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthBearer
			cfg.Token = "abc"
			So(ValidateWebDAVConfig(cfg), ShouldBeNil)
		})

		Convey("未知 AuthType 报错", func() {
			cfg := base
			cfg.AuthType = WebDAVAuthType("digest")
			err := ValidateWebDAVConfig(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "auth type")
		})

		Convey("URL 含 user:pass@ 沿用 ValidateWebDAVURL 行为报错", func() {
			cfg := WebDAVConfig{
				URL:      "https://user:pass@example.com/dav/",
				AuthType: WebDAVAuthNone,
			}
			err := ValidateWebDAVConfig(cfg)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "credentials")
		})
	})
}

func TestApplyWebDAVAuth(t *testing.T) {
	Convey("applyWebDAVAuth", t, func() {
		makeReq := func() *http.Request {
			req, err := http.NewRequest("GET", "https://example.com/dav/", nil)
			So(err, ShouldBeNil)
			return req
		}

		Convey("none 不写 Authorization 头", func() {
			req := makeReq()
			applyWebDAVAuth(req, WebDAVConfig{AuthType: WebDAVAuthNone})
			So(req.Header.Get("Authorization"), ShouldEqual, "")
		})

		Convey("basic 走 SetBasicAuth", func() {
			req := makeReq()
			applyWebDAVAuth(req, WebDAVConfig{
				AuthType: WebDAVAuthBasic,
				Username: "alice",
				Password: "s3cret",
			})
			user, pass, ok := req.BasicAuth()
			So(ok, ShouldBeTrue)
			So(user, ShouldEqual, "alice")
			So(pass, ShouldEqual, "s3cret")
		})

		Convey("bearer 写 Authorization: Bearer <token>", func() {
			req := makeReq()
			applyWebDAVAuth(req, WebDAVConfig{
				AuthType: WebDAVAuthBearer,
				Token:    "abc.def.ghi",
			})
			So(req.Header.Get("Authorization"), ShouldEqual, "Bearer abc.def.ghi")
		})

		Convey("bearer 但 token 为空时不写头", func() {
			req := makeReq()
			applyWebDAVAuth(req, WebDAVConfig{AuthType: WebDAVAuthBearer})
			So(req.Header.Get("Authorization"), ShouldEqual, "")
		})

		Convey("空 AuthType 视作 none", func() {
			req := makeReq()
			applyWebDAVAuth(req, WebDAVConfig{})
			So(req.Header.Get("Authorization"), ShouldEqual, "")
		})
	})
}

func TestWebDAVRequestWritesBearerHeader(t *testing.T) {
	Convey("webDAVRequest 写 bearer token 到 Authorization 头", t, func() {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		cfg := WebDAVConfig{
			URL:      srv.URL + "/dav/opskat/",
			AuthType: WebDAVAuthBearer,
			Token:    "tok-xyz",
		}
		_, _, err := webDAVRequest(cfg, http.MethodGet, srv.URL+"/probe", nil, nil)
		So(err, ShouldBeNil)
		So(gotAuth, ShouldEqual, "Bearer tok-xyz")
	})
}

func TestWebDAVRequestWritesNoAuthForNone(t *testing.T) {
	Convey("webDAVRequest 在 AuthType=none 时不写 Authorization", t, func() {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		cfg := WebDAVConfig{
			URL:      srv.URL + "/dav/opskat/",
			AuthType: WebDAVAuthNone,
		}
		_, _, err := webDAVRequest(cfg, http.MethodGet, srv.URL+"/probe", nil, nil)
		So(err, ShouldBeNil)
		So(gotAuth, ShouldEqual, "")
	})
}

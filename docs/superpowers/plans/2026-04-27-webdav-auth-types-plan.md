# WebDAV 备份多鉴权类型 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 WebDAV 备份加上 `none` / `basic` / `bearer` 三种鉴权方式，UI 上按选择动态渲染对应字段，密码 / token 解密后明文回填。

**Architecture:** 在 `bootstrap.AppConfig` 增加 `WebDAVAuthType` 与 `WebDAVToken`；`backup_svc.WebDAVConfig` 扩字段并把鉴权头注入抽成 `applyWebDAVAuth(req, cfg)`；`app` 层 Wails binding 改用 `WebDAVSaveInput` struct 入参，`GetWebDAVConfig` 解密回填明文；前端 `BackupSection` 用 `Select` 切换鉴权类型动态渲染字段。

**Tech Stack:** Go 1.25 + Wails v2 + GORM/SQLite + React 19 + TypeScript + shadcn/ui + i18next

**关联 Spec:** `docs/superpowers/specs/2026-04-27-webdav-auth-types-design.md`

---

### Task 1: 扩展 `bootstrap.AppConfig` 字段

**Files:**
- Modify: `internal/bootstrap/config.go:11-26`

- [ ] **Step 1: 在 `AppConfig` 结构体里 WebDAV 字段附近加两条**

把 `internal/bootstrap/config.go` 第 21-23 行那段改成：

```go
WebDAVURL       string `json:"webdav_url,omitempty"`        // WebDAV 备份目录
WebDAVAuthType  string `json:"webdav_auth_type,omitempty"`  // "none" | "basic" | "bearer"
WebDAVUsername  string `json:"webdav_username,omitempty"`   // WebDAV 用户名（非敏感，仅 basic）
WebDAVPassword  string `json:"webdav_password,omitempty"`   // 加密后的 WebDAV 密码（仅 basic）
WebDAVToken     string `json:"webdav_token,omitempty"`      // 加密后的 Bearer token（仅 bearer）
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/bootstrap/...`
Expected: 无报错。

- [ ] **Step 3: 提交**

```bash
git add internal/bootstrap/config.go
git commit -m "✨ AppConfig 增加 WebDAV 鉴权类型与 token 字段"
```

---

### Task 2: TDD `applyWebDAVAuth` 辅助函数

**Files:**
- Modify: `internal/service/backup_svc/webdav.go`
- Test: `internal/service/backup_svc/webdav_test.go`

- [ ] **Step 1: 在 `webdav.go` 顶部 const 块下方加 AuthType 常量**

把 `internal/service/backup_svc/webdav.go:16-19` 那段 const 块下方追加：

```go
// WebDAVAuthType 描述 WebDAV 服务器接受的鉴权方式。
type WebDAVAuthType string

const (
	WebDAVAuthNone   WebDAVAuthType = "none"
	WebDAVAuthBasic  WebDAVAuthType = "basic"
	WebDAVAuthBearer WebDAVAuthType = "bearer"
)
```

并把 `WebDAVConfig` 结构体（webdav.go:38-42）改为：

```go
// WebDAVConfig contains the connection details used for WebDAV backup transport.
type WebDAVConfig struct {
	URL      string         `json:"url"`
	AuthType WebDAVAuthType `json:"authType"`
	Username string         `json:"username,omitempty"` // 仅 basic
	Password string         `json:"password,omitempty"` // 仅 basic
	Token    string         `json:"token,omitempty"`    // 仅 bearer
}
```

- [ ] **Step 2: 编写失败测试** —— 在 `webdav_test.go` 末尾追加

```go
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
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/service/backup_svc/ -run TestApplyWebDAVAuth -v`
Expected: 编译失败，提示 `undefined: applyWebDAVAuth`。

- [ ] **Step 4: 写最小实现** —— 在 `webdav.go` 中现有 `webDAVRequest` 函数下方加：

```go
// applyWebDAVAuth 按 cfg.AuthType 给 req 注入鉴权头。
// 抽成函数：单测可直接验证 header；新增 Digest 等鉴权方式时仅改这一处。
func applyWebDAVAuth(req *http.Request, cfg WebDAVConfig) {
	switch cfg.AuthType {
	case WebDAVAuthBasic:
		req.SetBasicAuth(cfg.Username, cfg.Password)
	case WebDAVAuthBearer:
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
	case WebDAVAuthNone, "":
		// no-op
	}
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/service/backup_svc/ -run TestApplyWebDAVAuth -v`
Expected: 全部 PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/service/backup_svc/webdav.go internal/service/backup_svc/webdav_test.go
git commit -m "✨ backup_svc 新增 applyWebDAVAuth 与 WebDAVAuthType"
```

---

### Task 3: TDD `ValidateWebDAVConfig` 校验函数

**Files:**
- Modify: `internal/service/backup_svc/webdav.go`
- Test: `internal/service/backup_svc/webdav_test.go`

- [ ] **Step 1: 编写失败测试** —— 在 `webdav_test.go` 末尾追加

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/backup_svc/ -run TestValidateWebDAVConfig -v`
Expected: 编译失败，`undefined: ValidateWebDAVConfig`。

- [ ] **Step 3: 写实现** —— 把它加到 `webdav.go` 现有 `ValidateWebDAVURL` 下方：

```go
// ValidateWebDAVConfig 校验 URL 与 AuthType 必填字段。
// app 层在 Save / Test 入口处调用，避免发出无意义请求。
func ValidateWebDAVConfig(cfg WebDAVConfig) error {
	if err := ValidateWebDAVURL(cfg.URL); err != nil {
		return err
	}
	switch cfg.AuthType {
	case WebDAVAuthNone, "":
		return nil
	case WebDAVAuthBasic:
		if strings.TrimSpace(cfg.Username) == "" {
			return fmt.Errorf("WebDAV username is required for basic auth")
		}
		if cfg.Password == "" {
			return fmt.Errorf("WebDAV password is required for basic auth")
		}
		return nil
	case WebDAVAuthBearer:
		if strings.TrimSpace(cfg.Token) == "" {
			return fmt.Errorf("WebDAV token is required for bearer auth")
		}
		return nil
	default:
		return fmt.Errorf("unsupported WebDAV auth type %q", cfg.AuthType)
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/backup_svc/ -run TestValidateWebDAVConfig -v`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/service/backup_svc/webdav.go internal/service/backup_svc/webdav_test.go
git commit -m "✨ backup_svc 新增 ValidateWebDAVConfig 入口校验"
```

---

### Task 4: 把 `applyWebDAVAuth` 接到 `webDAVRequest` + 更新存量测试

**Files:**
- Modify: `internal/service/backup_svc/webdav.go:178-210` (`webDAVRequest`)
- Modify: `internal/service/backup_svc/webdav_test.go` (现有测试需要为 `WebDAVConfig` 显式赋 `AuthType`)

- [ ] **Step 1: 替换 `webDAVRequest` 中的鉴权调用**

把 `webdav.go` 现有：

```go
if cfg.Username != "" || cfg.Password != "" {
	req.SetBasicAuth(cfg.Username, cfg.Password)
}
```

改成：

```go
applyWebDAVAuth(req, cfg)
```

- [ ] **Step 2: 给现有测试中所有用到 Username/Password 的 `WebDAVConfig` 字面量加上 `AuthType: WebDAVAuthBasic`**

即 `webdav_test.go` 中下面这处：

```go
cfg := WebDAVConfig{
	URL:      srv.URL + "/dav/opskat/",
	Username: "dav-user",
	Password: "dav-pass",
}
```

改成：

```go
cfg := WebDAVConfig{
	URL:      srv.URL + "/dav/opskat/",
	AuthType: WebDAVAuthBasic,
	Username: "dav-user",
	Password: "dav-pass",
}
```

其余 `WebDAVConfig{URL: srv.URL + ...}` 里没有 username/password 的（如 `TestEnsureWebDAVDirectoryAcceptsConflict`、`TestWebDAVRequestRejectsRedirects`、`TestTestWebDAVConnectionVerifiesWriteCapability` 中所有 cfg、`TestListWebDAVBackupsHandlesNotFound`、`TestCreateOrUpdateWebDAVBackupReportsUploadFailure`、`TestGetWebDAVBackupContentReportsDownloadFailure`、`TestGetWebDAVBackupContentRejectsInvalidName`），保持不变 —— 它们走 `none` 分支不需要鉴权头。

- [ ] **Step 3: 在 stub 测试中针对三种 AuthType 增加一个跨类型回归用例** —— 在 `TestWebDAVBackups` 末尾追加新的 `Convey("supports bearer auth", ...)`：

把 `TestWebDAVBackups` 里的 stub `srv` 闭包改写一下也可以；最少代码改动方式是新增独立 `TestWebDAVRequestWritesBearerHeader`：

```go
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
```

放在 `webdav_test.go` 末尾。

- [ ] **Step 4: 运行整个 backup_svc 测试包**

Run: `go test ./internal/service/backup_svc/ -v`
Expected: 全部 PASS（包括 Task 2/3 新增的 + 现有所有用例）。

- [ ] **Step 5: 提交**

```bash
git add internal/service/backup_svc/webdav.go internal/service/backup_svc/webdav_test.go
git commit -m "♻️ webDAVRequest 改用 applyWebDAVAuth 并补 bearer/none 测试"
```

---

### Task 5: 重构 app 层 Wails binding（save / test / get / storage）

**Files:**
- Modify: `internal/app/app_settings.go:402-643`

- [ ] **Step 1: 替换 `WebDAVStoredConfig` 结构定义**

把 `app_settings.go:402-408` 的：

```go
// WebDAVStoredConfig 是前端可读取的 WebDAV 配置，不包含明文密码。
type WebDAVStoredConfig struct {
	URL         string `json:"url"`
	Username    string `json:"username,omitempty"`
	PasswordSet bool   `json:"passwordSet"`
	Configured  bool   `json:"configured"`
}
```

改为：

```go
// WebDAVStoredConfig 是前端可读取的 WebDAV 配置；password / token 解密后明文回填，
// 便于设置页编辑时直接显示已有值（数据未离开本地进程，加密存储已在落盘层做）。
type WebDAVStoredConfig struct {
	URL        string `json:"url"`
	AuthType   string `json:"authType"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Token      string `json:"token,omitempty"`
	Configured bool   `json:"configured"`
}

// WebDAVSaveInput 是 SaveWebDAVConfig / TestWebDAVConfig 的入参，把鉴权方式与凭据收成一个 struct。
type WebDAVSaveInput struct {
	URL      string `json:"url"`
	AuthType string `json:"authType"`
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}
```

- [ ] **Step 2: 重写 `SaveWebDAVConfig`**

把 `app_settings.go:484-509` 整个 `SaveWebDAVConfig` 函数体替换为：

```go
// SaveWebDAVConfig 保存 WebDAV 备份配置。按 AuthType 持久化对应字段，并清空其他类型字段以避免历史秘密残留。
func (a *App) SaveWebDAVConfig(in WebDAVSaveInput) error {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	svcCfg := backup_svc.WebDAVConfig{
		URL:      strings.TrimSpace(in.URL),
		AuthType: backup_svc.WebDAVAuthType(in.AuthType),
		Username: strings.TrimSpace(in.Username),
		Password: in.Password,
		Token:    strings.TrimSpace(in.Token),
	}
	if svcCfg.AuthType == "" {
		svcCfg.AuthType = backup_svc.WebDAVAuthNone
	}
	if err := backup_svc.ValidateWebDAVConfig(svcCfg); err != nil {
		return err
	}

	cfg.WebDAVURL = svcCfg.URL
	cfg.WebDAVAuthType = string(svcCfg.AuthType)

	// 清空所有 type 字段，再按当前 type 写回。避免切换鉴权方式后旧凭据仍留在 config.json。
	cfg.WebDAVUsername = ""
	cfg.WebDAVPassword = ""
	cfg.WebDAVToken = ""

	switch svcCfg.AuthType {
	case backup_svc.WebDAVAuthBasic:
		cfg.WebDAVUsername = svcCfg.Username
		encrypted, err := credential_svc.Default().Encrypt(svcCfg.Password)
		if err != nil {
			return fmt.Errorf("加密 WebDAV 密码失败: %w", err)
		}
		cfg.WebDAVPassword = encrypted
	case backup_svc.WebDAVAuthBearer:
		encrypted, err := credential_svc.Default().Encrypt(svcCfg.Token)
		if err != nil {
			return fmt.Errorf("加密 WebDAV token 失败: %w", err)
		}
		cfg.WebDAVToken = encrypted
	}
	return bootstrap.SaveConfig(cfg)
}
```

- [ ] **Step 3: 重写 `GetWebDAVConfig`**

把 `app_settings.go:512-523` 整个函数体替换为：

```go
// GetWebDAVConfig 读取已保存的 WebDAV 配置，password / token 解密后明文回填。
func (a *App) GetWebDAVConfig() (*WebDAVStoredConfig, error) {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return &WebDAVStoredConfig{}, nil
	}

	authType := cfg.WebDAVAuthType
	if authType == "" && strings.TrimSpace(cfg.WebDAVURL) != "" {
		authType = string(backup_svc.WebDAVAuthNone)
	}

	out := &WebDAVStoredConfig{
		URL:        cfg.WebDAVURL,
		AuthType:   authType,
		Username:   cfg.WebDAVUsername,
		Configured: strings.TrimSpace(cfg.WebDAVURL) != "",
	}
	if cfg.WebDAVPassword != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.WebDAVPassword)
		if err != nil {
			return nil, fmt.Errorf("解密 WebDAV 密码失败: %w", err)
		}
		out.Password = decrypted
	}
	if cfg.WebDAVToken != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.WebDAVToken)
		if err != nil {
			return nil, fmt.Errorf("解密 WebDAV token 失败: %w", err)
		}
		out.Token = decrypted
	}
	return out, nil
}
```

- [ ] **Step 4: 重写 `ClearWebDAVConfig`** —— 同时清掉新字段

把 `app_settings.go:526-535` 函数体替换为：

```go
// ClearWebDAVConfig 清除 WebDAV 备份配置。
func (a *App) ClearWebDAVConfig() error {
	cfg := bootstrap.GetConfig()
	if cfg == nil {
		return fmt.Errorf("config not loaded")
	}
	cfg.WebDAVURL = ""
	cfg.WebDAVAuthType = ""
	cfg.WebDAVUsername = ""
	cfg.WebDAVPassword = ""
	cfg.WebDAVToken = ""
	return bootstrap.SaveConfig(cfg)
}
```

- [ ] **Step 5: 重写 `TestWebDAVConfig`** —— 删除「password 空时回退」的旧 fallback

把 `app_settings.go:538-556` 函数体替换为：

```go
// TestWebDAVConfig 用入参里的字段测试 WebDAV 目录连通性与写权限。
// 完全使用入参字段，不再回退到已存配置——前端已回填明文凭据。
func (a *App) TestWebDAVConfig(in WebDAVSaveInput) error {
	svcCfg := backup_svc.WebDAVConfig{
		URL:      strings.TrimSpace(in.URL),
		AuthType: backup_svc.WebDAVAuthType(in.AuthType),
		Username: strings.TrimSpace(in.Username),
		Password: in.Password,
		Token:    strings.TrimSpace(in.Token),
	}
	if svcCfg.AuthType == "" {
		svcCfg.AuthType = backup_svc.WebDAVAuthNone
	}
	if err := backup_svc.ValidateWebDAVConfig(svcCfg); err != nil {
		return err
	}
	return backup_svc.TestWebDAVConnection(svcCfg)
}
```

- [ ] **Step 6: 更新 `webDAVConfigFromStorage`** —— 按 AuthType 取字段

把 `app_settings.go:623-643` 函数体替换为：

```go
func (a *App) webDAVConfigFromStorage() (backup_svc.WebDAVConfig, error) {
	cfg := bootstrap.GetConfig()
	if cfg == nil || strings.TrimSpace(cfg.WebDAVURL) == "" {
		return backup_svc.WebDAVConfig{}, fmt.Errorf("WebDAV 未配置")
	}

	authType := backup_svc.WebDAVAuthType(cfg.WebDAVAuthType)
	if authType == "" {
		authType = backup_svc.WebDAVAuthNone
	}

	out := backup_svc.WebDAVConfig{
		URL:      cfg.WebDAVURL,
		AuthType: authType,
		Username: cfg.WebDAVUsername,
	}
	if cfg.WebDAVPassword != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.WebDAVPassword)
		if err != nil {
			return backup_svc.WebDAVConfig{}, fmt.Errorf("解密 WebDAV 密码失败: %w", err)
		}
		out.Password = decrypted
	}
	if cfg.WebDAVToken != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.WebDAVToken)
		if err != nil {
			return backup_svc.WebDAVConfig{}, fmt.Errorf("解密 WebDAV token 失败: %w", err)
		}
		out.Token = decrypted
	}
	return out, nil
}
```

- [ ] **Step 7: 编译验证**

Run: `go build ./...`
Expected: 全部成功（前端 wailsjs 文件届时还是旧签名，但 Go 编译不依赖它们）。

- [ ] **Step 8: 跑后端测试**

Run: `make test`
Expected: 全部 PASS。

- [ ] **Step 9: 提交**

```bash
git add internal/app/app_settings.go
git commit -m "♻️ WebDAV binding 改用 SaveInput struct 并支持多鉴权类型"
```

---

### Task 6: 重新生成 Wails 前端 binding

**Files:**
- Auto-regen: `frontend/wailsjs/go/app/App.d.ts` / `App.js` / `models.ts`

- [ ] **Step 1: 启动 wails dev 触发 binding 生成**

Run: `make dev`
Expected: Wails 自动重新生成 `frontend/wailsjs/go/app/*` 与 `models.ts`；窗口打开后按 `Ctrl-C` 退出。

> 如果不想启动桌面窗口，也可以执行 `wails build -tags="" -skipbindings=false -devtools` 后立即 `Ctrl-C`，或运行 `wails generate module`（取决于 wails CLI 版本）。最稳妥还是 `make dev`。

- [ ] **Step 2: 确认 binding 已更新**

Run: `grep -n "SaveWebDAVConfig\|TestWebDAVConfig\|GetWebDAVConfig" frontend/wailsjs/go/app/App.d.ts`
Expected: 看到三处签名分别带 `WebDAVSaveInput` 入参 / `WebDAVStoredConfig` 返回。

```
grep -n "WebDAVSaveInput\|WebDAVStoredConfig" frontend/wailsjs/go/models.ts
```

Expected: 看到 `WebDAVSaveInput` 与 `WebDAVStoredConfig` 两个类型。

- [ ] **Step 3: 提交（生成产物）**

```bash
git add frontend/wailsjs/
git commit -m "🔧 重新生成 Wails 前端 binding（WebDAV 多鉴权）"
```

---

### Task 7: 新增 i18n 文案（zh-CN + en）

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN/common.json:506` 附近
- Modify: `frontend/src/i18n/locales/en/common.json:506` 附近

- [ ] **Step 1: 在 zh-CN 中新增 5 条 key**

把 `frontend/src/i18n/locales/zh-CN/common.json` 中现有：

```json
    "webdavPasswordPlaceholder": "留空则保留已保存密码",
```

改为下面这一段（删除已不再使用的 `webdavPasswordPlaceholder`，新增 5 条 key）：

```json
    "webdavAuthType": "鉴权方式",
    "webdavAuthNone": "无",
    "webdavAuthBasic": "Basic（账号密码）",
    "webdavAuthBearer": "Bearer Token",
    "webdavToken": "Token",
```

- [ ] **Step 2: 在 en 中新增对应 5 条**

同样在 `frontend/src/i18n/locales/en/common.json` 中把 `"webdavPasswordPlaceholder": "Leave blank to keep existing password",` 一行替换为：

```json
    "webdavAuthType": "Auth type",
    "webdavAuthNone": "None",
    "webdavAuthBasic": "Basic (username + password)",
    "webdavAuthBearer": "Bearer Token",
    "webdavToken": "Token",
```

- [ ] **Step 3: 验证 JSON 合法**

Run: `node -e "JSON.parse(require('fs').readFileSync('frontend/src/i18n/locales/zh-CN/common.json','utf8'))" && node -e "JSON.parse(require('fs').readFileSync('frontend/src/i18n/locales/en/common.json','utf8'))"`
Expected: 无输出（解析成功）。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/i18n/locales/zh-CN/common.json frontend/src/i18n/locales/en/common.json
git commit -m "🌐 WebDAV 设置增加鉴权类型相关文案"
```

---

### Task 8: 重构 `BackupSection.tsx` 的 WebDAV 卡片

**Files:**
- Modify: `frontend/src/components/settings/BackupSection.tsx:143-156, 198-211, 384-434, 586-636`

- [ ] **Step 1: 重写 WebDAV state**

把 `BackupSection.tsx:143-156` 的 WebDAV state 块整段替换为：

```tsx
  // WebDAV
  const [webdavConfigured, setWebDAVConfigured] = useState(false);
  const [webdavURL, setWebDAVURL] = useState("");
  const [webdavAuthType, setWebDAVAuthType] = useState<"none" | "basic" | "bearer">("basic");
  const [webdavUsername, setWebDAVUsername] = useState("");
  const [webdavPassword, setWebDAVPassword] = useState("");
  const [webdavToken, setWebDAVToken] = useState("");
  const [webdavBackups, setWebDAVBackups] = useState<backup_svc.WebDAVBackupInfo[]>([]);
  const [selectedWebDAVBackup, setSelectedWebDAVBackup] = useState("");
  const [webdavSaving, setWebDAVSaving] = useState(false);
  const [webdavTesting, setWebDAVTesting] = useState(false);
  const [webdavPushing, setWebDAVPushing] = useState(false);
  const [webdavPulling, setWebDAVPulling] = useState(false);
  const [webdavPullPasswordOpen, setWebDAVPullPasswordOpen] = useState(false);
  const [webdavPullPassword, setWebDAVPullPassword] = useState("");
```

（删除 `webdavPasswordSet`，新增 `webdavAuthType` / `webdavToken`）

- [ ] **Step 2: 重写初始化 `useEffect`**

把 `BackupSection.tsx:198-211` 的 `useEffect` 整段替换为：

```tsx
  useEffect(() => {
    (async () => {
      try {
        const cfg = await GetWebDAVConfig();
        if (!cfg) return;
        setWebDAVURL(cfg.url || "");
        setWebDAVAuthType(((cfg.authType as "none" | "basic" | "bearer") || "basic"));
        setWebDAVUsername(cfg.username || "");
        setWebDAVPassword(cfg.password || "");
        setWebDAVToken(cfg.token || "");
        setWebDAVConfigured(!!cfg.configured);
      } catch {
        /* not configured */
      }
    })();
  }, []);
```

- [ ] **Step 3: 重写 `handleWebDAVSave` / `handleWebDAVTest` / `handleWebDAVClear`**

把 `BackupSection.tsx:385-434` 三个函数整段替换为：

```tsx
  const buildWebDAVInput = () => ({
    url: webdavURL.trim(),
    authType: webdavAuthType,
    username: webdavUsername.trim(),
    password: webdavPassword,
    token: webdavToken.trim(),
  });

  const handleWebDAVSave = async () => {
    if (!webdavURL.trim()) {
      toast.error(t("backup.webdavURLRequired"));
      return;
    }
    setWebDAVSaving(true);
    try {
      await SaveWebDAVConfig(buildWebDAVInput());
      setWebDAVConfigured(true);
      toast.success(t("backup.webdavSaved"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setWebDAVSaving(false);
    }
  };

  const handleWebDAVTest = async () => {
    if (!webdavURL.trim()) {
      toast.error(t("backup.webdavURLRequired"));
      return;
    }
    setWebDAVTesting(true);
    try {
      await TestWebDAVConfig(buildWebDAVInput());
      toast.success(t("backup.webdavTestSuccess"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    } finally {
      setWebDAVTesting(false);
    }
  };

  const handleWebDAVClear = async () => {
    try {
      await ClearWebDAVConfig();
      setWebDAVConfigured(false);
      setWebDAVURL("");
      setWebDAVAuthType("basic");
      setWebDAVUsername("");
      setWebDAVPassword("");
      setWebDAVToken("");
      setWebDAVBackups([]);
      setSelectedWebDAVBackup("");
      toast.success(t("backup.webdavCleared"));
    } catch (e: unknown) {
      toast.error(errMsg(e));
    }
  };
```

- [ ] **Step 4: 重写卡片中字段渲染**

把 `BackupSection.tsx:586-636` 的 WebDAV `Card` 内容里 URL 输入下方那段（从 `<div className="grid gap-3 sm:grid-cols-2">` 包住 username/password 的部分）整段替换为：

```tsx
            <div className="grid gap-1.5">
              <Label>{t("backup.webdavAuthType")}</Label>
              <Select
                value={webdavAuthType}
                onValueChange={(v) => setWebDAVAuthType(v as "none" | "basic" | "bearer")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">{t("backup.webdavAuthNone")}</SelectItem>
                  <SelectItem value="basic">{t("backup.webdavAuthBasic")}</SelectItem>
                  <SelectItem value="bearer">{t("backup.webdavAuthBearer")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {webdavAuthType === "basic" && (
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label>{t("backup.webdavUsername")}</Label>
                  <Input
                    value={webdavUsername}
                    onChange={(e) => setWebDAVUsername(e.target.value)}
                    placeholder={t("backup.webdavUsernamePlaceholder")}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label>{t("backup.webdavPassword")}</Label>
                  <PasswordInput
                    value={webdavPassword}
                    onChange={(e) => setWebDAVPassword(e.target.value)}
                  />
                </div>
              </div>
            )}
            {webdavAuthType === "bearer" && (
              <div className="grid gap-1.5">
                <Label>{t("backup.webdavToken")}</Label>
                <PasswordInput
                  value={webdavToken}
                  onChange={(e) => setWebDAVToken(e.target.value)}
                />
              </div>
            )}
```

- [ ] **Step 5: 类型 / lint 检查**

Run: `cd frontend && pnpm lint`
Expected: 无错误（warning 不阻塞）。

Run: `cd frontend && pnpm test`
Expected: 已有测试全部 PASS（本次未新增前端测试）。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/settings/BackupSection.tsx
git commit -m "✨ WebDAV 设置卡片支持鉴权类型切换"
```

---

### Task 9: 全量验证 + 手测

**Files:**
- 无代码改动

- [ ] **Step 1: 后端单测 + lint**

Run: `make test && make lint`
Expected: 全部 PASS。

- [ ] **Step 2: 前端 lint + 测试 + 构建**

Run: `cd frontend && pnpm lint && pnpm test && pnpm build`
Expected: 全部成功。

- [ ] **Step 3: 启动 dev 手测三种鉴权类型**

Run: `make dev`

依次执行：

1. **None**：URL 填一个公网只读 WebDAV（或本地起一个 caddy file_server with anonymous）→ 选 `None` → 保存 → 测试连接 → 推送（应失败：只读）→ 切换为可写的 WebDAV 服务再试。
2. **Basic**：URL + 用户名 + 密码 → 选 `Basic` → 保存 → 测试连接 → 推送 → 拉取，验证内容一致。
3. **Bearer**：在支持的服务（如 Nextcloud 应用密码 / 自建带 Bearer 反代）→ 选 `Bearer Token` → 保存 → 测试连接 → 推送 → 拉取。

每次保存后关掉设置页再打开，验证 password / token 解密回填正确，且切换鉴权类型时其它 type 的字段已被清空（看 config.json）。

- [ ] **Step 4: 用 `git log --oneline` 复核提交历史**

Run: `git log --oneline -10`
Expected: 看到本计划 9 个 task 对应 8-9 条提交（task 1 / task 6 是单独 commit；task 2/3/4 各 1 条；task 5/7/8 各 1 条；task 9 通常无新 commit）。

---

## 自审备忘

**Spec 覆盖**：
- ✅ `WebDAVAuthType` 常量（none/basic/bearer）— Task 2
- ✅ `WebDAVConfig` 扩字段 — Task 2
- ✅ `applyWebDAVAuth` — Task 2
- ✅ `ValidateWebDAVConfig` — Task 3
- ✅ `webDAVRequest` 改造 — Task 4
- ✅ `bootstrap.AppConfig` 新字段 — Task 1
- ✅ `WebDAVSaveInput` / `WebDAVStoredConfig` 扩展 — Task 5
- ✅ `SaveWebDAVConfig` 清空他 type 字段 — Task 5 Step 2
- ✅ `TestWebDAVConfig` 删 fallback — Task 5 Step 5
- ✅ `GetWebDAVConfig` 解密回填 — Task 5 Step 3
- ✅ `webDAVConfigFromStorage` 按 type 取字段 — Task 5 Step 6
- ✅ Wails binding 重新生成 — Task 6
- ✅ 前端 state 重构 + 动态字段渲染 — Task 8
- ✅ i18n 5 条 key — Task 7
- ✅ 测试覆盖 applyWebDAVAuth / ValidateWebDAVConfig / bearer-header / no-auth — Task 2/3/4

**完成判定**：
- `make test` + `make lint` 全绿
- `cd frontend && pnpm lint && pnpm test && pnpm build` 全绿
- 三种鉴权类型手测：保存 / 测试连接 / 推送 / 拉取 全部走通

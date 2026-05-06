# WebDAV 备份：多鉴权类型支持

- 日期：2026-04-27
- 分支：`codex/webdav-backup`
- 范围：`internal/bootstrap/config.go`、`internal/service/backup_svc/webdav.go`、`internal/app/app_settings.go`、`frontend/src/components/settings/BackupSection.tsx`、i18n 文案

## 背景

当前 WebDAV 备份只支持 HTTP Basic 一种鉴权方式（`webdav.go` 中硬写 `req.SetBasicAuth(...)`，`WebDAVConfig` 仅含 `URL/Username/Password`）。坚果云之外的服务（自建 OAuth 网关、Nextcloud 应用密码、局域网只读分享）需要 Bearer Token 或无鉴权。

OpsKat 尚未发布版本，无需考虑现有用户的配置迁移。

## 目标

- 支持三种鉴权类型：`none` / `basic` / `bearer`
- UI 上提供「鉴权方式」下拉，按选择动态显示对应字段
- 保存后重新打开设置页时，密码 / token 解密后明文回填，便于编辑
- 切换鉴权方式并保存时，清空其他类型的历史秘密字段

## 非目标

- 不实现 Digest 鉴权（实现复杂、用户基数小，按需后续扩展）
- 不实现自定义 Authorization header / 自定义 scheme
- 不做配置迁移（无历史用户）

## 数据结构

### `internal/bootstrap/config.go`

在现有 WebDAV 字段附近增加：

```go
WebDAVAuthType string `json:"webdav_auth_type,omitempty"` // "none" | "basic" | "bearer"
WebDAVToken    string `json:"webdav_token,omitempty"`     // 加密后的 bearer token
```

保留 `WebDAVURL` / `WebDAVUsername` / `WebDAVPassword`，仅在 `AuthType == "basic"` 时有值。

### `internal/service/backup_svc/webdav.go`

```go
type WebDAVAuthType string

const (
    WebDAVAuthNone   WebDAVAuthType = "none"
    WebDAVAuthBasic  WebDAVAuthType = "basic"
    WebDAVAuthBearer WebDAVAuthType = "bearer"
)

type WebDAVConfig struct {
    URL      string         `json:"url"`
    AuthType WebDAVAuthType `json:"authType"`
    Username string         `json:"username,omitempty"` // basic
    Password string         `json:"password,omitempty"` // basic
    Token    string         `json:"token,omitempty"`    // bearer
}
```

### `internal/app/app_settings.go`

```go
type WebDAVSaveInput struct {
    URL      string `json:"url"`
    AuthType string `json:"authType"`
    Username string `json:"username"`
    Password string `json:"password"`
    Token    string `json:"token"`
}

type WebDAVStoredConfig struct {
    URL        string `json:"url"`
    AuthType   string `json:"authType"`
    Username   string `json:"username,omitempty"`
    Password   string `json:"password,omitempty"` // 解密后明文
    Token      string `json:"token,omitempty"`    // 解密后明文
    Configured bool   `json:"configured"`
}

func (a *App) SaveWebDAVConfig(in WebDAVSaveInput) error
func (a *App) TestWebDAVConfig(in WebDAVSaveInput) error
func (a *App) GetWebDAVConfig() (*WebDAVStoredConfig, error)
func (a *App) ClearWebDAVConfig() error // 不变
```

`SaveWebDAVConfig` 行为：
- 校验 URL（复用 `ValidateWebDAVURL`）和 AuthType（新增 `ValidateWebDAVConfig`）。
- 按 `AuthType` 持久化对应字段，**清空其他 type 的字段**。例如保存为 `bearer` 时，`WebDAVUsername` / `WebDAVPassword` 清空。
- Token / Password 走 `credential_svc.Default().Encrypt` 后落盘。

`TestWebDAVConfig` 行为：
- 完全使用入参字段，**不再回退到已存配置**（前端已回填明文，无需 fallback）。
- 删除 `app_settings.go` 中现有「密码为空时复用 stored 解密」分支。

`GetWebDAVConfig` 行为：
- 解密 `WebDAVPassword` / `WebDAVToken`，明文回填。
- 解密失败时返回错误（与现有「解密 WebDAV 密码失败」错误一致）。
- `AuthType` 为空但 URL 非空时按 `none` 处理（仅未发布开发环境会发生）。

## 后端鉴权分发

### `webDAVRequest` 改造

把当前硬写的：

```go
if cfg.Username != "" || cfg.Password != "" {
    req.SetBasicAuth(cfg.Username, cfg.Password)
}
```

替换为：

```go
applyWebDAVAuth(req, cfg)
```

新增小函数：

```go
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

抽出后单测可直接验证 header 设置；将来加 Digest 仅改这一处。

### `ValidateWebDAVConfig`

公开给 app 层，集中校验：

| AuthType | 必填                       |
|----------|----------------------------|
| `none`   | URL                        |
| `basic`  | URL + Username + Password  |
| `bearer` | URL + Token                |

未知 type 报错。URL 校验复用 `parseWebDAVBaseURL`（已禁 `user:pass@` 形式）。

### `TestWebDAVConnection`

逻辑不变：MKCOL → PUT 探测 → DELETE 探测，对所有 AuthType 一致。`none` 时服务端拒写返回 401/403 会透出真实错误，符合「测试连接」语义。

### 敏感字段日志

`webDAVRequest` 返回 `(status, body, err)` 不带 request header；token 不会出现在错误信息里。`fmt.Errorf("WebDAV ... HTTP %d: %s", status, body)` 里的 `body` 是服务端响应正文，不含我们发送的 token。无新增日志风险。

## 前端 UI

### `BackupSection.tsx` 状态

```ts
const [webdavURL, setWebDAVURL] = useState("");
const [webdavAuthType, setWebDAVAuthType] = useState<"none" | "basic" | "bearer">("basic");
const [webdavUsername, setWebDAVUsername] = useState("");
const [webdavPassword, setWebDAVPassword] = useState("");
const [webdavToken, setWebDAVToken] = useState("");
const [webdavConfigured, setWebDAVConfigured] = useState(false);
// 删除：webdavPasswordSet
```

### 初始化（替换现有 `GetWebDAVConfig` 副作用）

```ts
const cfg = await GetWebDAVConfig();
if (!cfg) return;
setWebDAVURL(cfg.url || "");
setWebDAVAuthType((cfg.authType as never) || "basic");
setWebDAVUsername(cfg.username || "");
setWebDAVPassword(cfg.password || "");
setWebDAVToken(cfg.token || "");
setWebDAVConfigured(!!cfg.configured);
```

### 字段渲染

URL 字段下方加鉴权类型 `Select`，下面字段按选择动态显示：

```tsx
<div className="grid gap-1.5">
  <Label>{t("backup.webdavAuthType")}</Label>
  <Select value={webdavAuthType} onValueChange={(v) => setWebDAVAuthType(v as never)}>
    <SelectTrigger><SelectValue /></SelectTrigger>
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
      <Input value={webdavUsername} onChange={(e) => setWebDAVUsername(e.target.value)} />
    </div>
    <div className="grid gap-1.5">
      <Label>{t("backup.webdavPassword")}</Label>
      <PasswordInput value={webdavPassword} onChange={(e) => setWebDAVPassword(e.target.value)} />
    </div>
  </div>
)}

{webdavAuthType === "bearer" && (
  <div className="grid gap-1.5">
    <Label>{t("backup.webdavToken")}</Label>
    <PasswordInput value={webdavToken} onChange={(e) => setWebDAVToken(e.target.value)} />
  </div>
)}
```

切换 type 时本地 state 保留各自的值（误切回来不丢数据）；保存成功后不主动清空（后端已清旧 type 字段，下次重新打开会拉到干净状态）。

### 保存 / 测试调用

```ts
const input = {
  url: webdavURL.trim(),
  authType: webdavAuthType,
  username: webdavUsername.trim(),
  password: webdavPassword,
  token: webdavToken.trim(),
};
await SaveWebDAVConfig(input);   // 或 TestWebDAVConfig(input)
```

### i18n（zh-CN / en）

| key | zh-CN | en |
|-----|-------|----|
| `backup.webdavAuthType`   | 鉴权方式             | Auth type |
| `backup.webdavAuthNone`   | 无                   | None |
| `backup.webdavAuthBasic`  | Basic（账号密码）    | Basic (username + password) |
| `backup.webdavAuthBearer` | Bearer Token         | Bearer Token |
| `backup.webdavToken`      | Token                | Token |

## 测试

### `internal/service/backup_svc/webdav_test.go`

- **`TestApplyWebDAVAuth`**：手造 `http.Request`，分别用三种 AuthType 调 `applyWebDAVAuth`，断言：
  - `none` → 无 `Authorization` header
  - `basic` → `Authorization: Basic <base64(user:pass)>`
  - `bearer` → `Authorization: Bearer <token>`
  - `bearer` 且 `Token == ""` → 无 `Authorization`
- **`TestValidateWebDAVConfig`**：表驱动覆盖 `none` / `basic` 缺字段 / `bearer` 缺字段 / 未知 type / URL 含 `user:pass@`。
- **扩展现有 stub 测试**：在 `httptest.Server` stub 端校验 header
  - `bearer` → `r.Header.Get("Authorization") == "Bearer xxx"`
  - `none`  → `r.Header.Get("Authorization") == ""`
  - `basic` → 使用 `r.BasicAuth()` 校验

### 前端 / app 层

- `BackupSection.tsx` 现无单测，本次不补，靠 `make dev` 手测。
- `app_settings.go` 现无 WebDAV 单测，本次不补；新逻辑都集中在 `backup_svc` 层。

## 实施清单（按依赖从底到顶）

1. `internal/bootstrap/config.go` — 新增 `WebDAVAuthType`、`WebDAVToken`
2. `internal/service/backup_svc/webdav.go` — 新增 `WebDAVAuthType` 常量、扩 `WebDAVConfig`、新增 `applyWebDAVAuth` / `ValidateWebDAVConfig`，`webDAVRequest` 改用 `applyWebDAVAuth`
3. `internal/service/backup_svc/webdav_test.go` — 上面三组单测
4. `internal/app/app_settings.go`：
   - 新增 `WebDAVSaveInput`
   - `SaveWebDAVConfig(in)` / `TestWebDAVConfig(in)` 改签名
   - `GetWebDAVConfig` 返回扩字段并解密回填
   - `webDAVConfigFromStorage()` 读 `WebDAVAuthType`，按 type 取字段
   - 删除 `TestWebDAVConfig` 旧的 fallback 分支
5. `frontend/wailsjs/go/*` — `make dev` 自动重生
6. `frontend/src/components/settings/BackupSection.tsx` — UI 重构
7. `frontend/src/i18n/locales/zh-CN.json` / `en.json` — 新增 5 条 key

## 风险与注意

- Wails binding 签名变更：`SaveWebDAVConfig` / `TestWebDAVConfig` 从 3 参变 1 个 struct 参；`GetWebDAVConfig` 返回结构变化。前端调用点都集中在 `BackupSection.tsx`，一处全改。
- `webDAVConfigFromStorage`：URL 空仍报「WebDAV 未配置」；`AuthType` 空但 URL 非空按 `none` 宽容处理。
- Token 加密键：复用 `credential_svc.Default()`，与 password 同套，无新增 keychain 项。
- `none` AuthType 测试连接：服务端拒写返回 401/403 会被错误信息透出，符合预期。
- 不变项：URL 校验、备份文件命名 / 探测 / 列表逻辑、加密备份内容均零改动。

## 完成判定

- `make test` 全绿
- `make lint` 全绿
- `make dev` 手测：none / basic / bearer 三种各跑一次「保存 → 测试连接 → 推送 → 拉取」

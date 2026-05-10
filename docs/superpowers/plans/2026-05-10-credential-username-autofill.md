# 密钥关联用户名 + 资产表单自动填充 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给生成/导入 SSH 密钥流程加上「关联用户名」字段，并在资产表单中根据所选密钥自动填充 username。

**Architecture:** 后端在 `credential_mgr_svc` 的请求结构 / 函数签名末尾加 `username` 入参，透传到 entity；前端在 `PasswordSourceField` 增加可选 `onUsernameChange` 回调，5 个 ConfigSection 把 `setUsername` 接进去（联动只在用户交互的 `Select.onValueChange` 中触发，初次挂载不触发，编辑模式天然免疫）。

**Tech Stack:** Go 1.25 + Wails v2、React 19 + TypeScript + Vitest + RTL、i18next、shadcn/ui Select。

**Spec:** `docs/superpowers/specs/2026-05-10-credential-username-autofill-design.md`

---

## File Structure

**后端**
- `internal/service/credential_mgr_svc/credential_mgr.go` — `GenerateKeyRequest` 加 `Username`；`GenerateSSHKey` / `ImportSSHKeyFromFile` / `ImportSSHKeyFromPEM` 落库时填入
- `internal/service/credential_mgr_svc/credential_mgr_test.go` — 新增三个 service-level 测试用例（自带 in-memory fake repo）
- `internal/app/app_credential.go` — 三个 binding 末尾加 `username string` 参数透传

**前端**
- `frontend/src/components/asset/PasswordSourceField.tsx` — 新增 prop `onUsernameChange`，managed 模式 onValueChange 内触发
- `frontend/src/components/asset/SSHConfigSection.tsx` — `<PasswordSourceField onUsernameChange={setUsername} />`
- `frontend/src/components/asset/DatabaseConfigSection.tsx` — 同上
- `frontend/src/components/asset/RedisConfigSection.tsx` — 同上
- `frontend/src/components/asset/MongoDBConfigSection.tsx` — 同上
- `frontend/src/components/asset/KafkaConfigSection.tsx` — 两处 `PasswordSourceField` 都接（L185 setter 风格、L394 controller 风格）
- `frontend/src/components/settings/CredentialManager.tsx` — `GenerateKeyDialog` / `ImportKeyDialog` 各加一个 username state + Input + 提交时透传

**测试**
- `frontend/src/__tests__/PasswordSourceField.test.tsx` — 新建：联动行为
- `frontend/src/__tests__/SSHConfigSection.test.tsx` — 新建：以 SSH 为代表，集成验证

**自动重生成（不手改）**
- `frontend/wailsjs/go/app/App.{d.ts,js}`、`models.ts`（执行 `make dev` 后 Wails 自动产物）

---

## Task 1: 后端 — `GenerateKeyRequest` 增 `Username` 字段并透传

**Files:**
- Modify: `internal/service/credential_mgr_svc/credential_mgr.go:24-31, 244-271`

- [ ] **Step 1: 编辑 `GenerateKeyRequest` 结构，加 `Username` 字段**

修改文件 `internal/service/credential_mgr_svc/credential_mgr.go`，把 struct 改成：

```go
// GenerateKeyRequest SSH 密钥生成请求
type GenerateKeyRequest struct {
	Name       string `json:"name"`
	Comment    string `json:"comment"`
	Username   string `json:"username"`   // 关联用户名（可选），用于资产表单自动填充
	KeyType    string `json:"keyType"`    // rsa, ed25519, ecdsa
	KeySize    int    `json:"keySize"`    // RSA: 2048/4096; ECDSA: 256/384/521; ED25519 忽略
	Passphrase string `json:"passphrase"` // 私钥密码（可选）
}
```

- [ ] **Step 2: 在 `GenerateSSHKey` 构造 entity 时填入 Username**

定位到 L244-256 处的 `cred := &credential_entity.Credential{...}`，加一行 `Username: req.Username,`：

```go
now := time.Now().Unix()
cred := &credential_entity.Credential{
	Name:        req.Name,
	Type:        credential_entity.TypeSSHKey,
	Comment:     comment,
	Username:    req.Username,
	KeyType:     req.KeyType,
	KeySize:     req.KeySize,
	PrivateKey:  encryptedPrivateKey,
	PublicKey:   publicKeyStr,
	Fingerprint: fingerprint,
	Createtime:  now,
	Updatetime:  now,
}
```

- [ ] **Step 3: 编译通过**

Run: `go build ./internal/service/credential_mgr_svc/...`
Expected: 退出码 0，无输出。

- [ ] **Step 4: 提交**

```bash
git add internal/service/credential_mgr_svc/credential_mgr.go
git commit -m "✨ credential_mgr: GenerateKeyRequest 支持 username 字段"
```

---

## Task 2: 后端 — `ImportSSHKeyFromFile` / `ImportSSHKeyFromPEM` 加 `username` 参数

**Files:**
- Modify: `internal/service/credential_mgr_svc/credential_mgr.go:273-358`

- [ ] **Step 1: 给 `ImportSSHKeyFromFile` 末尾加 `username string`**

```go
// ImportSSHKeyFromFile 从文件导入私钥
func ImportSSHKeyFromFile(ctx context.Context, name, comment, filePath, passphrase, username string) (*credential_entity.Credential, error) {
	if name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}

	data, err := os.ReadFile(filePath) //nolint:gosec // file path from user config
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	return ImportSSHKeyFromPEM(ctx, name, comment, string(data), passphrase, username)
}
```

- [ ] **Step 2: 给 `ImportSSHKeyFromPEM` 末尾加 `username string` 并写入 entity**

修改函数签名，并在 L340 处的 `cred := ...` 中加入 `Username: username,`：

```go
// ImportSSHKeyFromPEM 从 PEM 字符串导入私钥
func ImportSSHKeyFromPEM(ctx context.Context, name, comment, pemData, passphrase, username string) (*credential_entity.Credential, error) {
	if name == "" {
		return nil, fmt.Errorf("密钥名称不能为空")
	}
	// ... 中间 PEM 解析、加密逻辑保持不变 ...

	now := time.Now().Unix()
	cred := &credential_entity.Credential{
		Name:        name,
		Type:        credential_entity.TypeSSHKey,
		Comment:     comment,
		Username:    username,
		KeyType:     keyType,
		KeySize:     keySize,
		PrivateKey:  encryptedPrivateKey,
		Passphrase:  encryptedPassphrase,
		PublicKey:   publicKeyStr,
		Fingerprint: fingerprint,
		Createtime:  now,
		Updatetime:  now,
	}

	if err := credential_repo.Credential().Create(ctx, cred); err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}
	return cred, nil
}
```

- [ ] **Step 3: 编译通过**

Run: `go build ./internal/service/credential_mgr_svc/...`
Expected: 退出码 0。如果有 caller 编译错，下一步会修。

- [ ] **Step 4: 找出所有 caller，确认下一步要改哪些**

Run: `grep -rn "ImportSSHKeyFromFile\|ImportSSHKeyFromPEM" --include="*.go" .`
Expected: 列表中应仅有 `internal/app/app_credential.go` 一处使用方（以及 svc 内部 `ImportSSHKeyFromFile` 调 `ImportSSHKeyFromPEM` 那处，已在 Step 1 修复）。

- [ ] **Step 5: 提交**

```bash
git add internal/service/credential_mgr_svc/credential_mgr.go
git commit -m "✨ credential_mgr: ImportSSHKeyFrom{File,PEM} 加 username 参数"
```

---

## Task 3: 后端 — `app_credential.go` binding + `assettype/ssh.go` AI 调用透传 `username`

**Files:**
- Modify: `internal/app/app_credential.go:60-87`
- Modify: `internal/assettype/ssh.go:74, 118` (Task 2 grep 揭示的额外 caller，AI 工具 handler)

- [ ] **Step 1: 改 `GenerateSSHKey` binding 末尾加 `username string`，透传给 service**

```go
// GenerateSSHKey 生成新的 SSH 密钥对
func (a *App) GenerateSSHKey(name, comment, keyType string, keySize int, passphrase, username string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.GenerateSSHKey(a.langCtx(), credential_mgr_svc.GenerateKeyRequest{
		Name:       name,
		Comment:    comment,
		Username:   username,
		KeyType:    keyType,
		KeySize:    keySize,
		Passphrase: passphrase,
	})
}
```

- [ ] **Step 2: 改 `ImportSSHKeyFile` binding 末尾加 `username string`**

```go
// ImportSSHKeyFile 通过文件选择框导入 SSH 密钥
func (a *App) ImportSSHKeyFile(name, comment, passphrase, username string) (*credential_entity.Credential, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择 SSH 私钥文件",
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}
	return credential_mgr_svc.ImportSSHKeyFromFile(a.langCtx(), name, comment, filePath, passphrase, username)
}
```

- [ ] **Step 3: 改 `ImportSSHKeyPEM` binding 末尾加 `username string`**

```go
// ImportSSHKeyPEM 通过粘贴 PEM 内容导入 SSH 密钥
func (a *App) ImportSSHKeyPEM(name, comment, pemData, passphrase, username string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.ImportSSHKeyFromPEM(a.langCtx(), name, comment, pemData, passphrase, username)
}
```

- [ ] **Step 4: 修 `internal/assettype/ssh.go` 中 2 处 AI handler 调用**

L74 `sshHandler.ApplyCreateArgs` 内、L118 `sshHandler.ApplyUpdateArgs` 内的 `ImportSSHKeyFromPEM` 调用，末尾追加 `ArgString(args, "username")` —— 这样 AI 创建/更新 SSH 资产时若带 `username`，导入出来的 credential 也会附带同样的 username。

L74 修改：
```go
cred, err := credential_mgr_svc.ImportSSHKeyFromPEM(ctx, credName, "", privateKey, ArgString(args, "passphrase"), ArgString(args, "username"))
```

L118 修改：
```go
cred, err := credential_mgr_svc.ImportSSHKeyFromPEM(ctx, credName, "", privateKey, ArgString(args, "passphrase"), ArgString(args, "username"))
```

- [ ] **Step 5: 整个 backend 编译通过**

Run: `go build ./...`
Expected: 退出码 0。

- [ ] **Step 6: 提交**

```bash
git add internal/app/app_credential.go internal/assettype/ssh.go
git commit -m "✨ app/assettype: SSH 密钥导入/生成 caller 透传 username 参数"
```

---

## Task 4: 后端 — Service 层 in-memory 测试

**Files:**
- Modify: `internal/service/credential_mgr_svc/credential_mgr_test.go`

> 没有现成的 DB/svc 测试基础设施，但 `credential_repo` 暴露 `RegisterCredential(CredentialRepo)`、`credential_svc` 暴露 `SetDefault(*CredentialSvc)`，可在测试里注册一个内存 fake repo 跑端到端调用，无需 mockgen。

- [ ] **Step 1: 在测试文件追加 `fakeCredentialRepo` + 三个测试用例**

在 `internal/service/credential_mgr_svc/credential_mgr_test.go` 末尾追加（顶部 import 区按需补 `context`、`sync`、`credential_repo`、`credential_svc`）：

```go
// fakeCredentialRepo: 内存实现，仅用于测试。捕获最近一次 Create 的 cred 用于断言。
type fakeCredentialRepo struct {
	mu      sync.Mutex
	creds   []*credential_entity.Credential
	nextID  int64
}

func (r *fakeCredentialRepo) Find(_ context.Context, id int64) (*credential_entity.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.creds {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (r *fakeCredentialRepo) List(_ context.Context) ([]*credential_entity.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*credential_entity.Credential(nil), r.creds...), nil
}
func (r *fakeCredentialRepo) ListByType(_ context.Context, t string) ([]*credential_entity.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*credential_entity.Credential
	for _, c := range r.creds {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out, nil
}
func (r *fakeCredentialRepo) Create(_ context.Context, cred *credential_entity.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	cred.ID = r.nextID
	r.creds = append(r.creds, cred)
	return nil
}
func (r *fakeCredentialRepo) Update(_ context.Context, cred *credential_entity.Credential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.creds {
		if c.ID == cred.ID {
			r.creds[i] = cred
			return nil
		}
	}
	return fmt.Errorf("not found")
}
func (r *fakeCredentialRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.creds {
		if c.ID == id {
			r.creds = append(r.creds[:i], r.creds[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func setupCredentialTestEnv(t *testing.T) *fakeCredentialRepo {
	t.Helper()
	credential_svc.SetDefault(credential_svc.New("test-master-key-1234567890abcdef", []byte("test-salt-16byte")))
	repo := &fakeCredentialRepo{}
	credential_repo.RegisterCredential(repo)
	return repo
}

func TestGenerateSSHKey_PersistsUsername(t *testing.T) {
	Convey("GenerateSSHKey 写入 username 字段", t, func() {
		repo := setupCredentialTestEnv(t)
		ctx := context.Background()

		Convey("提供 username", func() {
			cred, err := GenerateSSHKey(ctx, GenerateKeyRequest{
				Name:     "test-key",
				KeyType:  credential_entity.KeyTypeED25519,
				Username: "alice",
			})
			assert.NoError(t, err)
			assert.Equal(t, "alice", cred.Username)
			assert.Len(t, repo.creds, 1)
			assert.Equal(t, "alice", repo.creds[0].Username)
		})

		Convey("username 留空保持空字符串", func() {
			cred, err := GenerateSSHKey(ctx, GenerateKeyRequest{
				Name:    "test-key-2",
				KeyType: credential_entity.KeyTypeED25519,
			})
			assert.NoError(t, err)
			assert.Equal(t, "", cred.Username)
		})
	})
}

func TestImportSSHKeyFromPEM_PersistsUsername(t *testing.T) {
	Convey("ImportSSHKeyFromPEM 写入 username 字段", t, func() {
		setupCredentialTestEnv(t)
		ctx := context.Background()

		// 用 gossh 现场生成一个无 passphrase 的 ed25519 PEM，避免 fixture
		_, privKey, err := ed25519.GenerateKey(rand.Reader)
		assert.NoError(t, err)
		block, err := gossh.MarshalPrivateKey(privKey, "test")
		assert.NoError(t, err)
		pemData := string(pem.EncodeToMemory(block))

		cred, err := ImportSSHKeyFromPEM(ctx, "imported", "comment", pemData, "", "bob")
		assert.NoError(t, err)
		assert.Equal(t, "bob", cred.Username)
	})
}
```

注意：测试顶部 import 区需要包含 `context`、`fmt`、`sync`、`github.com/opskat/opskat/internal/repository/credential_repo`、`github.com/opskat/opskat/internal/service/credential_svc`。文件原有的 `crypto/ed25519`、`crypto/rand`、`encoding/pem`、`gossh` import 已经存在，无需重复。

- [ ] **Step 2: 运行测试看通过**

Run: `go test ./internal/service/credential_mgr_svc/ -run 'TestGenerateSSHKey_PersistsUsername|TestImportSSHKeyFromPEM_PersistsUsername' -v`
Expected: PASS。如果失败，查看输出修补 import 或字段拼写。

- [ ] **Step 3: 跑整包测试确认未破坏现有用例**

Run: `go test ./internal/service/credential_mgr_svc/`
Expected: ok。

- [ ] **Step 4: 提交**

```bash
git add internal/service/credential_mgr_svc/credential_mgr_test.go
git commit -m "✅ credential_mgr: 增加 username 持久化的 service 级测试"
```

---

## Task 5: 前端 — `PasswordSourceField` 新增 `onUsernameChange` prop

**Files:**
- Modify: `frontend/src/components/asset/PasswordSourceField.tsx:8-26, 109-131`

- [ ] **Step 1: 在 props 接口加可选回调**

修改 `PasswordSourceFieldProps`：

```ts
interface PasswordSourceFieldProps {
  source: "inline" | "managed";
  onSourceChange: (source: "inline" | "managed") => void;
  password: string;
  onPasswordChange: (password: string) => void;
  credentialId: number;
  onCredentialIdChange: (id: number) => void;
  managedPasswords: credential_entity.Credential[];
  /** Placeholder for inline password input when no existing password */
  placeholder?: string;
  /** Placeholder shown when an existing encrypted password is set */
  hasExistingPassword?: boolean;
  /** Asset ID for decrypting existing password on reveal */
  editAssetId?: number;
  /** Override label for inline secret input. */
  secretLabel?: string;
  /** Override label for managed secret selector. */
  selectSecretLabel?: string;
  /** 选/换密钥时若所选 credential.username 非空则触发，把 username 回传给父组件 */
  onUsernameChange?: (username: string) => void;
}
```

并在解构里增加 `onUsernameChange`。

- [ ] **Step 2: 改 managed 模式 Select 的 onValueChange，命中非空 username 时触发 onUsernameChange**

把当前 L113 的 `<Select value={String(credentialId)} onValueChange={(v) => onCredentialIdChange(Number(v))}>` 改成：

```tsx
<Select
  value={String(credentialId)}
  onValueChange={(v) => {
    const id = Number(v);
    onCredentialIdChange(id);
    if (id !== 0 && onUsernameChange) {
      const cred = managedPasswords.find((p) => p.id === id);
      if (cred && cred.username) {
        onUsernameChange(cred.username);
      }
    }
  }}
>
```

- [ ] **Step 3: 类型检查通过**

Run: `cd frontend && pnpm tsc --noEmit`
Expected: 退出码 0（如果 monorepo 用别的命令，看 `frontend/package.json` 里 `lint:types` 之类）。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/components/asset/PasswordSourceField.tsx
git commit -m "✨ PasswordSourceField: 新增 onUsernameChange 联动回调"
```

---

## Task 6: 前端 — `PasswordSourceField` 单元测试

**Files:**
- Create: `frontend/src/__tests__/PasswordSourceField.test.tsx`

> 该组件依赖 Wails 运行时的 `GetAssetPassword`；测试按 `RedisConfigSection.test.tsx` 思路对外部依赖最小化。`__tests__/setup.ts` 已 mock Wails runtime，但若 `GetAssetPassword` 未 mock，下面用例不触发它（不点 reveal 按钮、不传 editAssetId）所以无影响。

- [ ] **Step 1: 写失败用例**

新建 `frontend/src/__tests__/PasswordSourceField.test.tsx`：

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { PasswordSourceField } from "../components/asset/PasswordSourceField";
import { credential_entity } from "../../wailsjs/go/models";

function makeCred(id: number, username: string): credential_entity.Credential {
  return { id, name: `cred-${id}`, username, type: "password" } as credential_entity.Credential;
}

function renderField(overrides: Partial<React.ComponentProps<typeof PasswordSourceField>> = {}) {
  const props: React.ComponentProps<typeof PasswordSourceField> = {
    source: "managed",
    onSourceChange: vi.fn(),
    password: "",
    onPasswordChange: vi.fn(),
    credentialId: 0,
    onCredentialIdChange: vi.fn(),
    managedPasswords: [makeCred(1, "alice"), makeCred(2, ""), makeCred(3, "bob")],
    onUsernameChange: vi.fn(),
    ...overrides,
  };
  return { ...render(<PasswordSourceField {...props} />), props };
}

describe("PasswordSourceField username 联动", () => {
  it("选中带 username 的密钥 → 触发 onUsernameChange", () => {
    const { props } = renderField();
    // 打开 Select 列表
    fireEvent.click(screen.getByRole("combobox", { name: /asset\.selectPassword/i }));
    // 点击 "cred-1 (alice)"
    fireEvent.click(screen.getByRole("option", { name: /cred-1 \(alice\)/ }));

    expect(props.onCredentialIdChange).toHaveBeenCalledWith(1);
    expect(props.onUsernameChange).toHaveBeenCalledWith("alice");
  });

  it("选中 username 为空的密钥 → 不触发 onUsernameChange", () => {
    const { props } = renderField();
    fireEvent.click(screen.getByRole("combobox", { name: /asset\.selectPassword/i }));
    fireEvent.click(screen.getByRole("option", { name: /^cred-2$/ }));

    expect(props.onCredentialIdChange).toHaveBeenCalledWith(2);
    expect(props.onUsernameChange).not.toHaveBeenCalled();
  });

  it("初次挂载（即使 credentialId 已有初值）→ 不触发 onUsernameChange", () => {
    const onUsernameChange = vi.fn();
    renderField({ credentialId: 1, onUsernameChange });
    expect(onUsernameChange).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: 运行测试，前两项应失败（onUsernameChange 未实现），第三项应通过**

> 注：如果 Task 5 已合入，前两项也通过。本测试主要充当回归保护。

Run: `cd frontend && pnpm test -- PasswordSourceField`
Expected: 三项 PASS（如果 Task 5 已实现）。

- [ ] **Step 3: 排错**

如果 `getByRole("combobox")` 找不到，原因通常是 Radix Select 在 jsdom/happy-dom 下需要 `pointer-events`。改用 `screen.getByText("asset.selectPasswordPlaceholder")` 或在 SelectTrigger 上加 testid 后用 `getByTestId` 定位。如果 `getByRole("option")` 找不到（Radix 把 popover 渲染到 portal 之外），可改用 `screen.findByText`。先尝试 role 查询，failing 时退化。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/__tests__/PasswordSourceField.test.tsx
git commit -m "✅ PasswordSourceField: 增加 username 联动单元测试"
```

---

## Task 7: 前端 — `GenerateKeyDialog` 加 username 输入框

**Files:**
- Modify: `frontend/src/components/settings/CredentialManager.tsx:302-431`

- [ ] **Step 1: 在 GenerateKeyDialog 内增加 username state，并在 useEffect reset 中重置**

定位到 L312-327 区域，把 state 与 reset 改为：

```tsx
const [name, setName] = useState("");
const [comment, setComment] = useState("");
const [username, setUsername] = useState("");
const [keyType, setKeyType] = useState("ed25519");
const [keySize, setKeySize] = useState(4096);
const [passphrase, setPassphrase] = useState("");
const [saving, setSaving] = useState(false);

useEffect(() => {
  if (open) {
    setName("");
    setComment("");
    setUsername("");
    setKeyType("ed25519");
    setKeySize(4096);
    setPassphrase("");
  }
}, [open]);
```

- [ ] **Step 2: 改 handleGenerate 把 username 传给 binding**

```tsx
const handleGenerate = async () => {
  setSaving(true);
  try {
    await GenerateSSHKey(name, comment, keyType, keySize, passphrase, username);
    toast.success(t("sshKey.generateSuccess"));
    onOpenChange(false);
    onSuccess();
  } catch (e) {
    toast.error(String(e));
  } finally {
    setSaving(false);
  }
};
```

- [ ] **Step 3: 在表单中插入 username 输入框（放在 comment 之后、keyType 之前）**

定位到 L379 后（comment 输入框那个 div 结束处），插入：

```tsx
<div className="grid gap-2">
  <Label>{t("credential.username")}</Label>
  <Input
    value={username}
    onChange={(e) => setUsername(e.target.value)}
    placeholder={t("credential.usernamePlaceholder")}
  />
</div>
```

- [ ] **Step 4: 启动 wails dev，让前端自动重生成 binding**

Run: `make dev`（在另一终端，或后台跑）
Expected: 启动后控制台输出 "Binding completed"，`frontend/wailsjs/go/app/App.d.ts` 中 `GenerateSSHKey` 签名末尾出现 `arg6: string`。停止 dev 进程。

> 如果不想跑完整 wails dev，也可以单独执行 `wails generate module` —— 但 `make dev` 是项目标准流程。

- [ ] **Step 5: 类型检查 + 测试通过**

Run: `cd frontend && pnpm tsc --noEmit && pnpm test -- --run`
Expected: 0 错误，已有测试不破。

- [ ] **Step 6: 提交（包括重生成的 wailsjs 文件）**

```bash
git add frontend/src/components/settings/CredentialManager.tsx frontend/wailsjs/
git commit -m "✨ GenerateKeyDialog: 增加关联用户名字段"
```

---

## Task 8: 前端 — `ImportKeyDialog` 加 username 输入框

**Files:**
- Modify: `frontend/src/components/settings/CredentialManager.tsx:433-555`

- [ ] **Step 1: 在 ImportKeyDialog 内增加 username state + reset**

```tsx
const [name, setName] = useState("");
const [comment, setComment] = useState("");
const [username, setUsername] = useState("");
const [pemContent, setPemContent] = useState("");
const [passphrase, setPassphrase] = useState("");
const [mode, setMode] = useState<"file" | "pem">("file");
const [saving, setSaving] = useState(false);

useEffect(() => {
  if (open) {
    setName("");
    setComment("");
    setUsername("");
    setPemContent("");
    setPassphrase("");
    setMode("file");
  }
}, [open]);
```

- [ ] **Step 2: 改 handleImportFile / handleImportPEM 把 username 传给 binding**

```tsx
const handleImportFile = async () => {
  setSaving(true);
  try {
    const result = await ImportSSHKeyFile(name, comment, passphrase, username);
    if (result) {
      toast.success(t("sshKey.importSuccess"));
      onOpenChange(false);
      onSuccess();
    }
  } catch (e) {
    toast.error(String(e));
  } finally {
    setSaving(false);
  }
};

const handleImportPEM = async () => {
  setSaving(true);
  try {
    await ImportSSHKeyPEM(name, comment, pemContent, passphrase, username);
    toast.success(t("sshKey.importSuccess"));
    onOpenChange(false);
    onSuccess();
  } catch (e) {
    toast.error(String(e));
  } finally {
    setSaving(false);
  }
};
```

- [ ] **Step 3: 在 comment 输入框之后插入 username 输入框**

定位 L508 之后（comment div 结束）插入：

```tsx
<div className="grid gap-2">
  <Label>{t("credential.username")}</Label>
  <Input
    value={username}
    onChange={(e) => setUsername(e.target.value)}
    placeholder={t("credential.usernamePlaceholder")}
  />
</div>
```

- [ ] **Step 4: 类型检查通过**

Run: `cd frontend && pnpm tsc --noEmit`
Expected: 退出码 0。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/settings/CredentialManager.tsx
git commit -m "✨ ImportKeyDialog: 增加关联用户名字段"
```

---

## Task 9: 前端 — 5 个 ConfigSection 接入 `onUsernameChange`

**Files:**
- Modify: `frontend/src/components/asset/SSHConfigSection.tsx:239-251`
- Modify: `frontend/src/components/asset/DatabaseConfigSection.tsx:94-104`
- Modify: `frontend/src/components/asset/RedisConfigSection.tsx:116-126`
- Modify: `frontend/src/components/asset/MongoDBConfigSection.tsx:145-155`
- Modify: `frontend/src/components/asset/KafkaConfigSection.tsx:185-195, 394-405`

- [ ] **Step 1: SSHConfigSection — 在 `<PasswordSourceField ...>` 调用末尾加一行**

定位 `frontend/src/components/asset/SSHConfigSection.tsx` L239 处的 `<PasswordSourceField`，在 `editAssetId={editAssetId}` 之后增加：

```tsx
onUsernameChange={setUsername}
```

完整 props 块应为：

```tsx
<PasswordSourceField
  source={passwordSource}
  onSourceChange={setPasswordSource}
  password={password}
  onPasswordChange={setPassword}
  credentialId={passwordCredentialId}
  onCredentialIdChange={setPasswordCredentialId}
  managedPasswords={managedPasswords}
  placeholder={t("asset.passwordPlaceholder")}
  hasExistingPassword={!!encryptedPassword}
  editAssetId={editAssetId}
  onUsernameChange={setUsername}
/>
```

- [ ] **Step 2: DatabaseConfigSection — 同样加 `onUsernameChange={setUsername}`**

L94 处的 `<PasswordSourceField>` 同样在最后加 `onUsernameChange={setUsername}`。

- [ ] **Step 3: RedisConfigSection — 同样加 `onUsernameChange={setUsername}`**

L116 处。

- [ ] **Step 4: MongoDBConfigSection — 同样加 `onUsernameChange={setUsername}`**

L145 处。

- [ ] **Step 5: KafkaConfigSection — 两处都加**

第一处 L185 setter 风格：

```tsx
<PasswordSourceField
  source={passwordSource}
  onSourceChange={setPasswordSource}
  password={password}
  onPasswordChange={setPassword}
  credentialId={passwordCredentialId}
  onCredentialIdChange={setPasswordCredentialId}
  managedPasswords={managedPasswords}
  hasExistingPassword={!!encryptedPassword}
  editAssetId={editAssetId}
  onUsernameChange={setUsername}
/>
```

第二处 L394 controller 风格（`value`/`onChange`）：

```tsx
<PasswordSourceField
  source={value.passwordSource}
  onSourceChange={(passwordSource) => onChange({ passwordSource })}
  password={value.password}
  onPasswordChange={(password) => onChange({ password })}
  credentialId={value.credentialId}
  onCredentialIdChange={(credentialId) => onChange({ credentialId })}
  managedPasswords={managedPasswords}
  hasExistingPassword={!!value.encryptedPassword}
  secretLabel={value.authType === "bearer" ? t("asset.kafkaBearerToken") : undefined}
  selectSecretLabel={value.authType === "bearer" ? t("asset.kafkaBearerToken") : undefined}
  onUsernameChange={(username) => onChange({ username })}
/>
```

- [ ] **Step 6: 类型检查 + 已有测试不破**

Run: `cd frontend && pnpm tsc --noEmit && pnpm test -- --run`
Expected: 0 错误，所有现有测试 PASS。

- [ ] **Step 7: 提交**

```bash
git add frontend/src/components/asset/SSHConfigSection.tsx \
        frontend/src/components/asset/DatabaseConfigSection.tsx \
        frontend/src/components/asset/RedisConfigSection.tsx \
        frontend/src/components/asset/MongoDBConfigSection.tsx \
        frontend/src/components/asset/KafkaConfigSection.tsx
git commit -m "✨ asset config: 选/换密钥时自动填充 username"
```

---

## Task 10: 前端 — `SSHConfigSection` 集成测试

**Files:**
- Create: `frontend/src/__tests__/SSHConfigSection.test.tsx`

> 以 SSH 为代表跑一份集成测试。其他 4 个 section 复用同一逻辑，单元测试已在 Task 6 覆盖核心行为，无需逐个写。

- [ ] **Step 1: 写测试**

新建 `frontend/src/__tests__/SSHConfigSection.test.tsx`，所有必填 props 已根据 `frontend/src/components/asset/SSHConfigSection.tsx:23-73` 的接口填齐。

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { SSHConfigSection } from "../components/asset/SSHConfigSection";
import { credential_entity } from "../../wailsjs/go/models";

function makeCred(id: number, username: string): credential_entity.Credential {
  return { id, name: `cred-${id}`, username, type: "password" } as credential_entity.Credential;
}

function renderSSH(overrides: Partial<React.ComponentProps<typeof SSHConfigSection>> = {}) {
  const setUsername = vi.fn();
  const setPasswordCredentialId = vi.fn();
  const props: React.ComponentProps<typeof SSHConfigSection> = {
    host: "10.0.0.1",
    setHost: vi.fn(),
    port: 22,
    setPort: vi.fn(),
    username: "",
    setUsername,
    authType: "password",
    setAuthType: vi.fn(),
    connectionType: "direct",
    setConnectionType: vi.fn(),
    password: "",
    setPassword: vi.fn(),
    encryptedPassword: "",
    passwordSource: "managed",
    setPasswordSource: vi.fn(),
    passwordCredentialId: 0,
    setPasswordCredentialId,
    managedPasswords: [makeCred(1, "alice"), makeCred(2, "")],
    keySource: "managed",
    setKeySource: vi.fn(),
    credentialId: 0,
    setCredentialId: vi.fn(),
    managedKeys: [],
    localKeys: [],
    setLocalKeys: vi.fn(),
    selectedKeyPaths: [],
    setSelectedKeyPaths: vi.fn(),
    privateKeyPassphrase: "",
    setPrivateKeyPassphrase: vi.fn(),
    scanningKeys: false,
    sshTunnelId: 0,
    setSshTunnelId: vi.fn(),
    proxyType: "",
    setProxyType: vi.fn(),
    proxyHost: "",
    setProxyHost: vi.fn(),
    proxyPort: 0,
    setProxyPort: vi.fn(),
    proxyUsername: "",
    setProxyUsername: vi.fn(),
    proxyPassword: "",
    setProxyPassword: vi.fn(),
    encryptedProxyPassword: "",
    ...overrides,
  };
  return { ...render(<SSHConfigSection {...props} />), setUsername, setPasswordCredentialId };
}

describe("SSHConfigSection 自动填用户名", () => {
  it("选中带 username 的密钥时调 setUsername", () => {
    const { setUsername } = renderSSH();

    fireEvent.click(screen.getByRole("combobox", { name: /asset\.selectPassword/i }));
    fireEvent.click(screen.getByRole("option", { name: /cred-1 \(alice\)/ }));

    expect(setUsername).toHaveBeenCalledWith("alice");
  });

  it("选中 username 为空的密钥时不调 setUsername", () => {
    const { setUsername } = renderSSH({
      username: "preexisting",
      managedPasswords: [makeCred(2, "")],
    });

    fireEvent.click(screen.getByRole("combobox", { name: /asset\.selectPassword/i }));
    fireEvent.click(screen.getByRole("option", { name: /^cred-2$/ }));

    expect(setUsername).not.toHaveBeenCalled();
  });
});
```

> 注：`SSHConfigSectionProps` 上 `jumpHostExcludeIds?` 和 `editAssetId?` 是可选的，已省略。`SSHConfigSection.tsx:23-73` 是当前接口快照，如发现字段已变化以源文件为准。

- [ ] **Step 2: 运行**

Run: `cd frontend && pnpm test -- SSHConfigSection`
Expected: 两项 PASS。如 `getByRole("combobox")` 失败，参考 Task 6 Step 3 的排错思路（Radix Select 在 jsdom/happy-dom 下的限制）。

- [ ] **Step 3: 提交**

```bash
git add frontend/src/__tests__/SSHConfigSection.test.tsx
git commit -m "✅ SSHConfigSection: 增加 username 自动填充集成测试"
```

---

## Task 11: 端到端冒烟测试（手工）

**Files:**
- 仅运行 `make dev`，无文件修改

> Wails 应用无法用脚本完整 e2e；此为手动冒烟。

- [ ] **Step 1: 启动 dev 服务**

Run: `make dev`
Expected: 弹出桌面应用窗口。

- [ ] **Step 2: 验证生成 SSH 密钥**

操作：设置 → 凭证管理 → 生成 SSH 密钥
- 输入 name = `key-alice`，username = `alice`，类型 ed25519
- 提交
- 在凭证列表里看到 `key-alice`，下拉可见 `key-alice (alice)`

- [ ] **Step 3: 验证导入 SSH 密钥（PEM 模式）**

操作：导入 SSH 密钥 → PEM 模式
- 粘贴任意一段无 passphrase 的 ed25519 PEM，name = `key-bob`，username = `bob`
- 提交
- 列表显示 `key-bob (bob)`

- [ ] **Step 4: 验证资产表单自动填用户名**

操作：资产 → 新建 SSH 资产
- 把密码源切到「使用已管理凭证」
- 选 `key-alice (alice)` → 用户名输入框立即变成 `alice`
- 切换到 `key-bob (bob)` → 输入框变成 `bob`
- 切换到一个 username 为空的旧凭证 → 输入框保持 `bob`
- 手动改 username 为 `custom` → 输入框正常接受输入
- 切回 `key-alice (alice)` → 输入框被覆盖为 `alice`（验证"总是覆盖"策略）

- [ ] **Step 5: 验证编辑模式不自动覆盖**

操作：编辑刚才创建的 SSH 资产
- 打开编辑表单时 username 字段保持原值（不被自动重写）
- 不动密钥下拉、直接保存 → 资产不变

- [ ] **Step 6: 重复 Step 4 抽查 Database 资产**

操作：新建 Database 资产，选带 username 的密码凭证 → 用户名输入框被填充。

- [ ] **Step 7: 提交手工冒烟通过的 commit message（如有微调）**

如果发现 bug，回到对应 Task 修；如全部通过，无需 commit。

---

## Task 12: 收尾 — Lint 与提交合并

- [ ] **Step 1: 后端 lint**

Run: `make lint`
Expected: 无新增错误。如有 warning 涉及到本次改动文件，按 `make lint-fix` 处理。

- [ ] **Step 2: 前端 lint**

Run: `cd frontend && pnpm lint`
Expected: 0 错误。失败用 `pnpm lint:fix`。

- [ ] **Step 3: 全测**

Run: `make test && cd frontend && pnpm test -- --run`
Expected: 全部 PASS。

- [ ] **Step 4: 查看提交历史**

Run: `git log --oneline main..HEAD`
Expected: 一系列 ✨/✅ 提交，每个提交单独可读。

---

## Non-Goals

- 不调整 `UpdateCredential`（SSH 密钥编辑对话框暂不支持改 username）。Service 层 `Update` 当前仅对 password 类型写 `Username`，本次保持不变。如未来需要在 SSH 密钥编辑表单露出 username 字段，再单开 spec/plan。
- 不改 `PasswordSourceField` 的 `onCredentialIdChange` 签名，新功能通过新增可选 prop 实现，不破坏现有 API。
- 不解析 SSH key comment 推断 username。
- 不强制 username 必填。
- 不调整 PasswordSourceField 下拉项展示格式。

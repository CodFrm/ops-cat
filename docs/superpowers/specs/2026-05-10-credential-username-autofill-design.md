# 密钥关联用户名 + 资产表单自动填充

**Date:** 2026-05-10
**Status:** Approved, ready for implementation plan

## 背景与目标

OpsKat 的 `Credential` 实体已经有 `Username` 字段，`CreatePassword` / `UpdateCredential` 已支持写入；密钥选择器 `PasswordSourceField` 在下拉项中已经把 `(username)` 显示出来。但当前流程存在三个缺口：

1. **生成 SSH 密钥** (`GenerateSSHKey`) 后端 / 前端均无 username 入口。
2. **导入 SSH 密钥** (`ImportSSHKeyFromFile` / `ImportSSHKeyFromPEM`) 后端 / 前端均无 username 入口。
3. **资产创建/编辑表单**选择密钥后不会用 credential.username 自动填充资产的 username 字段，必须手填。

本次目标：补齐前两个入口的 username 字段，并让资产表单在用户切换密钥时自动覆盖 username。

## 行为规范

| 场景 | 行为 |
|---|---|
| 生成 SSH 密钥对话框 | 新增可选输入框「关联用户名」，提交时随请求落库 |
| 导入 SSH 密钥（File / PEM）对话框 | 同上 |
| 资产表单（任意类型）选择/切换密钥 | 命中 credential.username 非空 → 覆盖资产 username 字段；为空 → 保持原值不变 |
| 资产表单 username 字段 | 仍可手动编辑；覆盖后不锁定，用户随时可改回 |
| 编辑已有资产打开表单 | 不自动覆盖（联动只在 `Select.onValueChange` 中实现，初次挂载不触发） |

### 决策记录

- **覆盖策略**：选/换密钥总是覆盖（用户可手动改回）。理由：选密钥这个行为本身就表明用户想用这个密钥的身份；如果不想覆盖，他不会换。
- **空 username**：切换到 username 为空的密钥时**不清空**资产 username 字段。理由：避免用户因为选错密钥而丢失刚手填的内容。
- **编辑模式**：仅在用户主动换密钥时触发，不在打开表单时自动同步。理由：保护现有资产配置不被意外修改。
- **导入/生成对话框 username 字段**：可选输入框，与现有 `CreatePassword` 行为一致。

## 数据流

```
Generate/Import Dialog ──username──▶ App binding ──▶ credential_mgr_svc ──▶ DB
                                                              │
Asset Form ◀──managedPasswords (含 username)── useCredentialStore
   │
   └─ PasswordSourceField.onValueChange(id):
         onCredentialIdChange(id)
         if (cred.username) onUsernameChange(cred.username)
```

## 改动清单

### 后端 (Go)

| 文件 | 改动 |
|---|---|
| `internal/service/credential_mgr_svc/credential_mgr.go` | `GenerateKeyRequest` 增 `Username string` 字段；`GenerateSSHKey` 写入 credential 时带上；`ImportSSHKeyFromFile` / `ImportSSHKeyFromPEM` 签名末尾加 `username string`，落库时填入 |
| `internal/app/app_credential.go` | `GenerateSSHKey` / `ImportSSHKeyFile` / `ImportSSHKeyPEM` 三个 binding 末尾加 `username string` 参数，透传到 service |
| `internal/service/credential_mgr_svc/mock_*/` | 通过 `go generate ./...` 重新生成 mock |

### 前端 (React + TypeScript)

| 文件 | 改动 |
|---|---|
| `frontend/wailsjs/go/app/App.{d.ts,js}`, `models.ts` | Wails 自动重生成（`make dev`） |
| `CredentialManager.tsx` — `GenerateKeyDialog` (≈L302-431) | 表单 state 加 `username`；新增可选输入框；提交时传给 binding |
| `CredentialManager.tsx` — `ImportKeyDialog` (≈L433-555) | 同上 |
| `PasswordSourceField.tsx` | 新增 prop `onUsernameChange?: (username: string) => void`；**仅 managed 模式**的 `Select.onValueChange` 内查 credential，若 `username` 非空则触发回调（非 managed 模式没有 credential 实体，不联动） |
| `SSHConfigSection.tsx` (≈L269-289 处) | 给 `PasswordSourceField` 传 `onUsernameChange={setUsername}` |
| `DatabaseConfigSection.tsx` / `RedisConfigSection.tsx` / `MongoDBConfigSection.tsx` / `KafkaConfigSection.tsx` | 同上 |

### i18n

复用已有 key，无需新增：

| Key | zh-CN | en |
|---|---|---|
| `credential.username` | 关联用户名 | Associated username |
| `credential.usernamePlaceholder` | 可选 | Associated username (optional) |

### 数据库迁移

无需新增迁移：`Credential.Username` 列在 entity 中已存在，已有迁移已建表。

## 测试

### 后端

`internal/service/credential_mgr_svc/credential_mgr_test.go`:
- `GenerateSSHKey` 新用例：`Username: "alice"` → 断言 `cred.Username == "alice"`
- `GenerateSSHKey` 既有用例：username 留空 → 断言 `cred.Username == ""`
- `ImportSSHKeyFromPEM` 新用例：传入 username → 落库后字段一致

### 前端

`PasswordSourceField.test.tsx`（新建或扩展）：
- 选择带 username 的 credential → `onUsernameChange("alice")` 被调用一次
- 选择 username 为空的 credential → `onUsernameChange` **不**被调用
- 初次挂载（即使带初始 credentialId）→ `onUsernameChange` 不触发

`SSHConfigSection.test.tsx` 等 5 个 section 至少 SSH 一份代表性测试：
- 渲染后切换密钥，username input 的值同步更新到密钥的 username
- 编辑模式：传入已有 credentialId 渲染，username input 保持初始值不变

## 非目标（明确不做）

- **不做反向同步**：修改资产 username 不会回写 credential
- **不做 SSH key comment 自动推断 username**（不解析 PEM 中的 `user@host` comment）
- **不强制 username 必填**，与现有 `CreatePassword` 一致
- **不调整 PasswordSourceField 下拉的展示格式**（已有 `name (username)` 后缀已足够）
- **不改 PasswordSourceField 的 onChange 回调签名**（保持 `onCredentialIdChange(id: number)` 不变以兼容现有调用方），新功能通过新增可选 prop 实现

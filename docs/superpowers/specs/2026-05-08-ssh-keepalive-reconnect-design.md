# SSH 保活与终端回车重连设计

- **日期**: 2026-05-08
- **关联 Issue**: #80
- **作者**: Claude (与用户协作)

## 背景

Issue #80 报告两个 SSH 终端体验问题：

1. **挂机一段时间后连接断开**: 用户离开终端一会后回到桌面，看到 `connect close`，必须手动重连。NAT 设备和云防火墙会清理空闲 TCP 连接，OpsKat 当前没有任何保活机制。
2. **重连步骤繁琐**: 当前重连只能通过终端右键菜单触发，没有快捷键，敲击常用按键也不会重连。

调查现有代码确认两点都是空白：

- `internal/service/ssh_svc/ssh.go:234, 253` 构造 `ssh.ClientConfig` 时没有任何保活字段。`golang.org/x/crypto/ssh` 不暴露 OpenSSH 风格的 `ServerAliveInterval`，必须自己跑心跳 goroutine。
- `internal/sshpool/pool.go:56` 的 `isAlive()` 仅在 `Pool.Get()` 复用时被动检查，没有主动定时心跳。
- `frontend/src/components/terminal/terminalRegistry.ts:64` 的 `ssh:closed` 事件只显示红字 `[Connection closed]` 并 `markClosed`，不监听任何按键。

## 目标

- 让 SSH 长连接在闲置数分钟后仍然存活，不被 NAT/防火墙清掉。
- 终端断开后，用户在终端中按回车即可触发重连，提示文案明确告知用户可以这样做。

## 非目标

- **不实现**保活失败后的自动重建（保留会话 ID、PTY 复用、未读输出回放）。保活失败 = 连接确实断了，走现有 `ssh:closed` 流程。
- **不实现**全局快捷键（如 `Ctrl+R` 重连）。可后续作为独立 issue。
- **不暴露**保活间隔为用户可配置项。全局固定 60 秒。
- **不修改**资产级别的连接参数 schema。

## 设计

### A. 后端：60 秒 SSH 保活

#### 新增共享 helper 包

`sshpool` 不导入 `ssh_svc`，`ssh_svc` 不导入 `sshpool` —— 两者是兄弟。为避免新增不必要的依赖边，新建共享包：

文件：`internal/pkg/sshkeepalive/keepalive.go`

```go
package sshkeepalive

import (
    "time"
    "golang.org/x/crypto/ssh"
)

// Interval is the global SSH keepalive interval.
const Interval = 60 * time.Second

// Start launches a goroutine that periodically sends an OpenSSH keepalive
// request on the given client. It returns a stop function that the caller
// MUST invoke when the client is being closed.
//
// The goroutine exits silently if SendRequest returns an error — the read
// loop on the client will detect EOF and surface it to the existing close
// path. Start does NOT close the client itself.
func Start(client *ssh.Client, interval time.Duration) (stop func())
```

跟项目里已有的 `internal/pkg/dirsync` 同类模式。

实现要点：

- `time.NewTicker(interval)`，每次触发调 `client.SendRequest("keepalive@openssh.com", true, nil)`。
- 第一次触发在 `interval` 之后，不在启动时立刻发。
- `stop` 通过 `chan struct{}` 通知 goroutine 退出，幂等（多次调用安全）。
- SendRequest 错误：日志 debug 级别 + 直接 return（停止 ticker），让上层 read loop 自然检测到连接死。

#### 接入两个 wrapper

| Wrapper | 文件 | 启动 | 停止 |
|---|---|---|---|
| `sharedClient` | `internal/service/ssh_svc/ssh.go:30` | `newSharedClient` 末尾，调 `sshkeepalive.Start` 并把返回的 stop 存到字段 | `release()` 在 refCount<=0 分支调 `stopKeepalive()` 后再 `client.Close()` |
| `poolEntry` | `internal/sshpool/pool.go:21` | `Pool.Get` 创建新 entry 后 | `poolEntry.close()` 头部，`client.Close` 之前调 stop |

两个 wrapper 都加字段 `stopKeepalive func()`。

#### 测试

文件：`internal/pkg/sshkeepalive/keepalive_test.go`

用 mock ssh server（`golang.org/x/crypto/ssh` 自带 test 工具或基于 `net.Pipe` 起一对 server/client）验证：

- 启动后 `interval` 经过会发出 `keepalive@openssh.com` global request
- 调用 stop 后不再发出请求
- stop 重复调用安全
- SendRequest 失败时 goroutine 退出（不 panic）

不修改现有 `ssh_test.go` 用例。

### B. 前端：回车重连

#### i18n 文案

`frontend/src/i18n/locales/zh-CN/common.json`：
```json
"ssh.session.closedHint": "[连接已断开 — 按回车重连]"
```

`frontend/src/i18n/locales/en/common.json`：
```json
"ssh.session.closedHint": "[Connection closed — press Enter to reconnect]"
```

#### terminalRegistry 改造

文件：`frontend/src/components/terminal/terminalRegistry.ts`

1. `InternalInstance` 增加可变字段：
   ```ts
   interface InternalInstance extends TerminalInstance {
     isClosed: boolean;
     dispose: () => void;
   }
   ```

2. 创建 instance 时注册 `term.onKey` 监听器（**只注册一次**，不每次断开都重新注册）：
   ```ts
   const onKeyDispose = term.onKey(({ key }) => {
     if (instance.isClosed && key === "\r") {
       instance.isClosed = false; // 立刻翻转防抖
       useTerminalStore.getState().reconnectBySession(sessionId);
     }
   });
   ```
   并把 `onKeyDispose` 加进 `dispose()` 释放链。

3. `ssh:closed` 事件处理改为：
   ```ts
   EventsOn(closedEvent, () => {
     const hint = i18n.t("common:ssh.session.closedHint");
     term.write(`\r\n\x1b[31m${hint}\x1b[0m\r\n`);
     instance.isClosed = true;
     useTerminalStore.getState().markClosed(sessionId);
   });
   ```
   （`i18n` 从 `@/i18n` 导入。）

4. `reconnect` 流程会调 `disposeTerminalInstance(sessionId)`（`terminalStore.ts:562`），其内部走 `instance.dispose()`，移除 `onKeyDispose` —— 不会泄漏监听器。

#### terminalStore 新增方法

文件：`frontend/src/stores/terminalStore.ts`

接口加 `reconnectBySession: (sessionId: string) => void;`，实现：

```ts
reconnectBySession: (sessionId) => {
  const { tabData } = get();
  const tabId = Object.keys(tabData).find(id => tabData[id]?.panes[sessionId]);
  if (tabId) get().reconnect(tabId);
},
```

复用现有 `reconnect(tabId)`，不重复其 disconnect / ConnectSSHAsync / 树替换逻辑。

#### 测试

文件：`frontend/src/components/terminal/__tests__/terminalRegistry.test.ts`（如不存在则新建）

mock xterm + Wails runtime + i18next：

- 触发 `ssh:closed` 事件 → `term.write` 收到含国际化文案的字符串、`isClosed=true`、`markClosed` 被调用
- isClosed=true 时模拟 `onKey({key: "\r"})` → `reconnectBySession` 被调用、`isClosed=false`
- 第二次 `\r` → `reconnectBySession` 不再被调用
- 非 Enter 键（如 `"a"`）→ 不触发

`terminalStore.reconnectBySession` 单测：构造 tabData，验证能反查到 tabId 并调用 `reconnect`。

## 错误处理

| 场景 | 行为 |
|---|---|
| 后端保活 SendRequest 失败 | ticker 退出；不主动 Close；read loop 自然报 EOF → `ssh:closed` 事件 → 用户看到断开提示 |
| 重连本身失败 | 走 `reconnect()` 现有错误路径；提示再次出现，用户可再次按 Enter |
| 用户在断开发生**之前**按 Enter | xterm 把按键作为正常输入发给 stdin，与现状一致 |
| 用户在断开发生**之后**且**重连进行中**按 Enter | `isClosed` 已被翻为 false；按键不再触发重连，被丢弃（pane 处于 connecting 状态，xterm 实例已 dispose）|
| 资产被删除 / 凭据失效 | `reconnect` 内部 ConnectSSHAsync 失败 → 现有错误路径 |

## 提交粒度

同一 branch 内拆 2 个 commit：

1. `✨ ssh: add 60s keepalive heartbeat for shared and pooled clients (#80)`
2. `✨ terminal: press Enter on closed terminal to reconnect (#80)`

Branch: `ssh-keepalive-and-enter-reconnect`，在 git worktree 中执行。

## 影响面

- 后端：每个活跃 SSH 连接多一个 goroutine、每 60s 一次极小的 SSH global request。10 个连接每分钟 10 个 packet，可忽略。
- 前端：每个 xterm 实例多一个 `onKey` 监听器，无运行时开销。
- 兼容性：所有 SSH 服务器都支持 `keepalive@openssh.com` global request（OpenSSH 标准）。

## 风险

- **goroutine 泄漏**：如果 stop 没被调用，保活 goroutine 会一直跑直到 client 死掉。`sharedClient.release` 和 `poolEntry.close` 是关闭路径的唯一收口，覆盖率高，但实施阶段需要看遍所有 close/release 路径确认没有早期 return 跳过。
- **i18n 加载时机**：`closed` 事件回调里调 `i18n.t`，需要确认 i18next 已初始化。前端 bootstrap 通常 i18n 早于 terminal 创建，应该没问题；测试时显式 mock。

# SSH 保活与终端回车重连 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 SSH 长连接 60 秒发送一次保活心跳避免被 NAT/防火墙清掉；终端断开后用户可在 xterm 内按回车直接重连。

**Architecture:** 后端在 `internal/pkg/sshkeepalive` 新建共享 helper 包，由 `ssh_svc.sharedClient` 与 `sshpool.poolEntry` 各自调用启停。前端 `terminalRegistry` 在断开时翻一个 `isClosed` 标记，注册一次性 `onKey` 监听，回车时调用新增的 `terminalStore.reconnectBySession`，复用既有的 `reconnect(tabId)` 流程。

**Tech Stack:**
- Backend: Go 1.25, `golang.org/x/crypto/ssh`, goconvey + testify
- Frontend: React 19 + TypeScript, xterm.js 6, Zustand 5, vitest + happy-dom + RTL, i18next

**Spec:** `docs/superpowers/specs/2026-05-08-ssh-keepalive-reconnect-design.md`

---

## Worktree 准备

> 实现在独立的 git worktree 中进行（用户要求）。在主仓库的根目录执行下面命令前，先确认 `git status` 没有未提交的改动需要先 commit/stash。

- [ ] **Step 0.1: 创建 worktree 和分支**

Run:
```bash
git worktree add -b ssh-keepalive-and-enter-reconnect ../opskat-ssh-keepalive main
cd ../opskat-ssh-keepalive
```

> 注意：spec 文档已在 `mobile-poc` 分支提交，新分支基于 `main`。后续 PR 提到 spec 时引用其 commit hash 即可。

如果 `cd` 失败或路径已存在，先 `git worktree list` 确认是否已经创建过。

---

## Phase 1：后端共享 helper 包

### Task 1: 新建 `internal/pkg/sshkeepalive`

**Files:**
- Create: `internal/pkg/sshkeepalive/keepalive.go`
- Create: `internal/pkg/sshkeepalive/keepalive_test.go`

设计原则：
- `Start` 接受 `Pinger` 接口（`*ssh.Client` 自动满足），便于注入 mock 测试，无需起真实 ssh server
- `interval` 是参数（不是常量），生产代码传 `Interval`，测试传 `time.Millisecond` 级别

- [ ] **Step 1.1: 写失败测试**

创建 `internal/pkg/sshkeepalive/keepalive_test.go`：

```go
package sshkeepalive

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type fakePinger struct {
	mu       sync.Mutex
	count    int32
	returnFn func() error
}

func (f *fakePinger) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	atomic.AddInt32(&f.count, 1)
	f.mu.Lock()
	fn := f.returnFn
	f.mu.Unlock()
	if fn != nil {
		return false, nil, fn()
	}
	return true, nil, nil
}

func (f *fakePinger) calls() int32 {
	return atomic.LoadInt32(&f.count)
}

func TestStart(t *testing.T) {
	Convey("Start sends keepalive ticks", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 10*time.Millisecond)
		defer stop()

		time.Sleep(55 * time.Millisecond)
		So(fp.calls(), ShouldBeGreaterThanOrEqualTo, 3)
	})

	Convey("Start does not fire before the first interval", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 100*time.Millisecond)
		defer stop()

		time.Sleep(20 * time.Millisecond)
		So(fp.calls(), ShouldEqual, 0)
	})

	Convey("stop halts the ticker", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 10*time.Millisecond)
		time.Sleep(35 * time.Millisecond)
		stop()
		baseline := fp.calls()

		time.Sleep(50 * time.Millisecond)
		So(fp.calls(), ShouldEqual, baseline)
	})

	Convey("stop is idempotent", t, func() {
		fp := &fakePinger{}
		stop := Start(fp, 10*time.Millisecond)
		stop()
		stop() // must not panic
		stop()
		So(true, ShouldBeTrue)
	})

	Convey("ping error stops the goroutine", t, func() {
		fp := &fakePinger{returnFn: func() error { return errors.New("boom") }}
		stop := Start(fp, 5*time.Millisecond)
		defer stop()

		time.Sleep(40 * time.Millisecond)
		// First failing call exits the goroutine; allow at most a couple
		// scheduled ticks before the error is observed.
		So(fp.calls(), ShouldBeLessThanOrEqualTo, 2)
	})
}
```

- [ ] **Step 1.2: 运行测试，确认失败**

Run:
```bash
go test ./internal/pkg/sshkeepalive/...
```

Expected: 编译失败（`undefined: Start`、`undefined: Pinger` 之类）。

- [ ] **Step 1.3: 实现 helper**

创建 `internal/pkg/sshkeepalive/keepalive.go`：

```go
// Package sshkeepalive runs an OpenSSH-style keepalive heartbeat over an
// ssh.Client (or any compatible Pinger), so long-lived SSH sessions don't
// get reaped by NAT/firewall idle timeouts.
package sshkeepalive

import (
	"sync"
	"time"
)

// Interval is the global SSH keepalive heartbeat interval.
const Interval = 60 * time.Second

// Pinger is the subset of *ssh.Client used to send keepalive global requests.
// Defining it as an interface keeps this package decoupled from net/ssh and
// makes it trivial to test with a fake.
type Pinger interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
}

// Start launches a goroutine that sends an OpenSSH "keepalive@openssh.com"
// global request on p every interval. It returns a stop function the caller
// MUST invoke when shutting down. stop is idempotent.
//
// If SendRequest returns an error, the goroutine exits silently. Start does
// NOT close the underlying connection — the read loop on the client will
// detect EOF and surface it through the existing close path.
func Start(p Pinger, interval time.Duration) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	stopFn := func() { once.Do(func() { close(done) }) }

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, _, err := p.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					return
				}
			}
		}
	}()

	return stopFn
}
```

- [ ] **Step 1.4: 运行测试，确认通过**

Run:
```bash
go test ./internal/pkg/sshkeepalive/... -v
```

Expected: 5 个 Convey 全 PASS。

- [ ] **Step 1.5: 提交**

```bash
git add internal/pkg/sshkeepalive/
git commit -m "$(cat <<'EOF'
✨ 新增 sshkeepalive 共享心跳包 (#80)

Why: NAT/防火墙会清理空闲 SSH 连接,需要主动发送 keepalive。
sharedClient 与 poolEntry 都需要,放到共享包避免重复实现。
EOF
)"
```

---

## Phase 2：接入 `ssh_svc.sharedClient`

### Task 2: 在 `sharedClient` 起停 keepalive

**Files:**
- Modify: `internal/service/ssh_svc/ssh.go:21-59`

- [ ] **Step 2.1: 修改 `sharedClient` 结构体加 stop 字段**

打开 `internal/service/ssh_svc/ssh.go`，找到 21-28 行的结构体定义，改为：

```go
// sharedClient 封装 SSH 连接，支持引用计数共享
type sharedClient struct {
	client        *ssh.Client
	mu            sync.Mutex
	refCount      int
	closers       []io.Closer // 跳板机 client 等额外资源
	closed        bool
	stopKeepalive func()
}
```

- [ ] **Step 2.2: 在 `newSharedClient` 启动 keepalive**

找到 30-36 行 `newSharedClient`，改为：

```go
func newSharedClient(client *ssh.Client, closers []io.Closer) *sharedClient {
	sc := &sharedClient{
		client:   client,
		refCount: 1,
		closers:  closers,
	}
	sc.stopKeepalive = sshkeepalive.Start(client, sshkeepalive.Interval)
	return sc
}
```

并在文件顶部 import 块（3-19 行）末尾加：

```go
"github.com/opskat/opskat/internal/pkg/sshkeepalive"
```

> Tip: 让 goimports / golangci-lint --fix 帮你排序 import。

- [ ] **Step 2.3: 在 `release` 真正关闭分支调 stop**

找到 44-59 行 `release`，改为：

```go
func (sc *sharedClient) release() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.refCount--
	if sc.refCount <= 0 && !sc.closed {
		sc.closed = true
		if sc.stopKeepalive != nil {
			sc.stopKeepalive()
		}
		if err := sc.client.Close(); err != nil {
			logger.Default().Warn("close client", zap.Error(err))
		}
		for _, c := range sc.closers {
			if err := c.Close(); err != nil {
				logger.Default().Warn("close jump host resource", zap.Error(err))
			}
		}
	}
}
```

- [ ] **Step 2.4: 修复测试中可能的 nil-stopKeepalive 引用**

`internal/service/ssh_svc/ssh_test.go:451, 480` 直接构造 `&sharedClient{client: &ssh.Client{}}`。当前不会调 `release` 的关闭分支，所以无需改。但为防御未来用例，运行：

```bash
go test ./internal/service/ssh_svc/... -count=1
```

Expected: 全部 PASS。如果有用例 panic 在 `stopKeepalive == nil`，修测试构造时显式给 `stopKeepalive: func(){}`。

- [ ] **Step 2.5: 提交**

```bash
git add internal/service/ssh_svc/
git commit -m "$(cat <<'EOF'
✨ ssh_svc: sharedClient 启动 60s 保活心跳 (#80)

Why: 共享 SSH client 在 PTY/SFTP/proxy 间复用,挂机时易被 NAT 清掉。
保活在 newSharedClient 启动、refCount 归零关闭时停止。
EOF
)"
```

---

## Phase 3：接入 `sshpool.poolEntry`

### Task 3: 在 `poolEntry` 起停 keepalive

**Files:**
- Modify: `internal/sshpool/pool.go:20-82`

- [ ] **Step 3.1: 加 stop 字段**

打开 `internal/sshpool/pool.go`，找到 20-29 行结构体，改为：

```go
// poolEntry 连接池条目
type poolEntry struct {
	client        *ssh.Client
	closers       []io.Closer // 跳板机等中间连接
	assetID       int64
	lastUsed      time.Time
	refCount      int
	mu            sync.Mutex
	closed        bool
	stopKeepalive func()
}
```

- [ ] **Step 3.2: 在创建 entry 时启动 keepalive**

找到 137-143 行 `Pool.Get` 中创建新 entry 的位置：

```go
entry = &poolEntry{
    client:   client,
    closers:  closers,
    assetID:  assetID,
    lastUsed: time.Now(),
    refCount: 1,
}
```

改为：

```go
entry = &poolEntry{
    client:        client,
    closers:       closers,
    assetID:       assetID,
    lastUsed:      time.Now(),
    refCount:      1,
    stopKeepalive: sshkeepalive.Start(client, sshkeepalive.Interval),
}
```

并在 import 块（3-13 行）加：

```go
"github.com/opskat/opskat/internal/pkg/sshkeepalive"
```

> 注意：`Pool.Get` 里有"拨号期间另一个 goroutine 已建好连接"的去重分支（145-165 行），那条分支会 close 我们刚 dial 出的 client。但我们还没把 entry 装进 map，stopKeepalive 也没启动 —— 那条分支不需要改。**关键**：只有真正放进 `p.entries[assetID] = entry`（166 行）的 entry 才有 stopKeepalive，确保了对称。

- [ ] **Step 3.3: 在 `close` 调 stop**

找到 67-82 行 `close`，改为：

```go
// close 关闭连接及所有中间连接
func (e *poolEntry) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	if e.stopKeepalive != nil {
		e.stopKeepalive()
	}
	if err := e.client.Close(); err != nil {
		logger.Default().Warn("close ssh client", zap.Int64("assetID", e.assetID), zap.Error(err))
	}
	for _, c := range e.closers {
		if err := c.Close(); err != nil {
			logger.Default().Warn("close intermediate connection", zap.Int64("assetID", e.assetID), zap.Error(err))
		}
	}
}
```

- [ ] **Step 3.4: 跑包内测试**

Run:
```bash
go test ./internal/sshpool/... -count=1
```

Expected: 现有 frame_test.go 的用例仍 PASS（保活改动不影响 frame 编解码）。

- [ ] **Step 3.5: 跑全量编译 + 测试**

Run:
```bash
go build ./...
go test ./... -count=1
```

Expected: 编译通过、全部测试 PASS。

- [ ] **Step 3.6: 提交**

```bash
git add internal/sshpool/
git commit -m "$(cat <<'EOF'
✨ sshpool: poolEntry 启动 60s 保活心跳 (#80)

Why: 连接池里的 SSH client 服务于 opsctl/远程命令等场景,
同样需要主动保活;否则空闲后再次复用会拿到死连接。
EOF
)"
```

---

## Phase 4：前端国际化文案

### Task 4: 添加 closedHint i18n key

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN/common.json:549-556`
- Modify: `frontend/src/i18n/locales/en/common.json:565-571`

- [ ] **Step 4.1: 加 zh-CN key**

打开 `frontend/src/i18n/locales/zh-CN/common.json`，把 549-556 行的 `session` 块：

```json
    "session": {
      "reconnect": "重新连接",
      "disconnect": "断开连接",
      "splitH": "水平分割",
      "splitV": "垂直分割",
      "connected": "已连接",
      "disconnected": "已断开"
    },
```

改为（增加 `closedHint`）：

```json
    "session": {
      "reconnect": "重新连接",
      "disconnect": "断开连接",
      "splitH": "水平分割",
      "splitV": "垂直分割",
      "connected": "已连接",
      "disconnected": "已断开",
      "closedHint": "[连接已断开 — 按回车重连]"
    },
```

- [ ] **Step 4.2: 加 en key**

打开 `frontend/src/i18n/locales/en/common.json`，把 565-571 行的 `session` 块改为：

```json
    "session": {
      "reconnect": "Reconnect",
      "disconnect": "Disconnect",
      "splitH": "Split Horizontal",
      "splitV": "Split Vertical",
      "connected": "Connected",
      "disconnected": "Disconnected",
      "closedHint": "[Connection closed — press Enter to reconnect]"
    },
```

- [ ] **Step 4.3: 跑前端测试 + lint**

Run:
```bash
cd frontend && pnpm lint && pnpm test --run
```

Expected: 全部通过（i18n key 增加不影响现有用例）。

> **Step 4 不单独 commit**，下面 Task 6 一起提交。

---

## Phase 5：terminalStore 新增 `reconnectBySession`

### Task 5: 在 store 加 sessionId → tabId 反查并复用 reconnect

**Files:**
- Modify: `frontend/src/stores/terminalStore.ts:362, 534`

- [ ] **Step 5.1: 加接口声明**

打开 `frontend/src/stores/terminalStore.ts`，找到 362 行：

```ts
  reconnect: (tabId: string) => void;
```

下方插入一行：

```ts
  reconnect: (tabId: string) => void;
  reconnectBySession: (sessionId: string) => void;
```

- [ ] **Step 5.2: 加实现**

找到 `reconnect: (tabId) => { ... }` 的定义（534 行起）。在 `reconnect` 实现块**结束的右大括号 + 逗号**之后插入新方法：

```ts
  reconnectBySession: (sessionId) => {
    const { tabData } = get();
    const tabId = Object.keys(tabData).find((id) => Boolean(tabData[id]?.panes[sessionId]));
    if (tabId) get().reconnect(tabId);
  },
```

> 定位提示：`reconnect` 实现 ~36 行，结束在 580 行左右。在它的关闭 `},` 之后、下一个方法（`disconnect:` 或类似）之前插入。

- [ ] **Step 5.3: 编译 / 类型检查**

Run:
```bash
cd frontend && pnpm tsc --noEmit
```

Expected: 0 errors。

> **Step 5 不单独 commit**，下面 Task 6 一起提交。

---

## Phase 6：terminalRegistry 加 isClosed + onKey

### Task 6: 断开后回车触发 reconnect

**Files:**
- Modify: `frontend/src/components/terminal/terminalRegistry.ts`
- Create: `frontend/src/__tests__/terminalRegistry.test.ts`

- [ ] **Step 6.1: 写失败测试**

创建 `frontend/src/__tests__/terminalRegistry.test.ts`：

```ts
import { describe, it, expect, vi, beforeEach } from "vitest";

// Capture the EventsOn handlers so we can fire them manually.
const eventHandlers = new Map<string, (...args: unknown[]) => void>();
vi.mock("../../wailsjs/runtime/runtime", () => ({
  EventsOn: (event: string, handler: (...args: unknown[]) => void) => {
    eventHandlers.set(event, handler);
  },
  EventsOff: (event: string) => {
    eventHandlers.delete(event);
  },
}));

vi.mock("../../wailsjs/go/app/App", () => ({
  WriteSSH: vi.fn().mockResolvedValue(undefined),
}));

// Capture the term.onKey handler.
let capturedOnKey: ((e: { key: string }) => void) | null = null;
const writeSpy = vi.fn();
const disposeSpy = vi.fn();

vi.mock("@xterm/xterm", () => ({
  Terminal: vi.fn().mockImplementation(() => ({
    loadAddon: vi.fn(),
    open: vi.fn(),
    write: writeSpy,
    onData: vi.fn(() => ({ dispose: vi.fn() })),
    onKey: vi.fn((handler: (e: { key: string }) => void) => {
      capturedOnKey = handler;
      return { dispose: vi.fn() };
    }),
    dispose: disposeSpy,
  })),
}));

vi.mock("@xterm/addon-fit", () => ({ FitAddon: vi.fn().mockImplementation(() => ({})) }));
vi.mock("@xterm/addon-search", () => ({ SearchAddon: vi.fn().mockImplementation(() => ({})) }));
vi.mock("@xterm/xterm/css/xterm.css", () => ({}));

const reconnectBySessionMock = vi.fn();
vi.mock("@/stores/terminalStore", () => ({
  useTerminalStore: {
    getState: () => ({
      markClosed: vi.fn(),
      reconnectBySession: reconnectBySessionMock,
    }),
  },
}));

vi.mock("@/data/terminalFonts", () => ({ withTerminalFontFallback: (s: string) => s }));
vi.mock("@/lib/terminalEncode", () => ({ bytesToBase64: () => "" }));

vi.mock("@/i18n", () => ({
  default: { t: (key: string) => `<<${key}>>` },
}));

import { getOrCreateTerminal, disposeTerminal } from "@/components/terminal/terminalRegistry";

describe("terminalRegistry", () => {
  beforeEach(() => {
    eventHandlers.clear();
    capturedOnKey = null;
    writeSpy.mockClear();
    disposeSpy.mockClear();
    reconnectBySessionMock.mockClear();
  });

  it("writes the i18n closed hint and marks closed when ssh:closed fires", () => {
    getOrCreateTerminal("sess-1", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    const handler = eventHandlers.get("ssh:closed:sess-1");
    expect(handler).toBeDefined();
    handler?.();
    const written = writeSpy.mock.calls.map((c) => c[0]).join("");
    expect(written).toContain("<<ssh.session.closedHint>>");
    disposeTerminal("sess-1");
  });

  it("triggers reconnectBySession on Enter after close, only once", () => {
    getOrCreateTerminal("sess-2", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    eventHandlers.get("ssh:closed:sess-2")?.();
    expect(capturedOnKey).toBeTruthy();

    capturedOnKey?.({ key: "\r" });
    expect(reconnectBySessionMock).toHaveBeenCalledWith("sess-2");
    expect(reconnectBySessionMock).toHaveBeenCalledTimes(1);

    // Second Enter must not retrigger
    capturedOnKey?.({ key: "\r" });
    expect(reconnectBySessionMock).toHaveBeenCalledTimes(1);
    disposeTerminal("sess-2");
  });

  it("ignores non-Enter keys after close", () => {
    getOrCreateTerminal("sess-3", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    eventHandlers.get("ssh:closed:sess-3")?.();
    capturedOnKey?.({ key: "a" });
    capturedOnKey?.({ key: "\n" });
    expect(reconnectBySessionMock).not.toHaveBeenCalled();
    disposeTerminal("sess-3");
  });

  it("does not trigger reconnect when not closed", () => {
    getOrCreateTerminal("sess-4", { fontSize: 14, fontFamily: "mono", scrollback: 1000 });
    capturedOnKey?.({ key: "\r" });
    expect(reconnectBySessionMock).not.toHaveBeenCalled();
    disposeTerminal("sess-4");
  });
});
```

- [ ] **Step 6.2: 跑测试，确认失败**

Run:
```bash
cd frontend && pnpm test --run src/__tests__/terminalRegistry.test.ts
```

Expected: 多个用例 FAIL（registry 当前不调用 i18n.t、不注册 onKey、没有 isClosed 翻转）。

- [ ] **Step 6.3: 改 registry 实现**

打开 `frontend/src/components/terminal/terminalRegistry.ts`，整体替换为：

```ts
import { Terminal as XTerminal, type ITheme } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import "@xterm/xterm/css/xterm.css";
import { WriteSSH } from "../../../wailsjs/go/app/App";
import { EventsOn, EventsOff } from "../../../wailsjs/runtime/runtime";
import { bytesToBase64 } from "@/lib/terminalEncode";
import { useTerminalStore } from "@/stores/terminalStore";
import { withTerminalFontFallback } from "@/data/terminalFonts";
import i18n from "@/i18n";

export interface TerminalInstance {
  term: XTerminal;
  fitAddon: FitAddon;
  searchAddon: SearchAddon;
  container: HTMLDivElement;
}

interface InternalInstance extends TerminalInstance {
  isClosed: boolean;
  dispose: () => void;
}

const registry = new Map<string, InternalInstance>();

export function getOrCreateTerminal(
  sessionId: string,
  init: { fontSize: number; fontFamily: string; theme?: ITheme; scrollback: number }
): TerminalInstance {
  const cached = registry.get(sessionId);
  if (cached) return cached;

  const container = document.createElement("div");
  container.style.height = "100%";
  container.style.width = "100%";

  const term = new XTerminal({
    cursorBlink: true,
    fontSize: init.fontSize,
    fontFamily: withTerminalFontFallback(init.fontFamily),
    theme: init.theme,
    scrollback: init.scrollback,
  });

  const fitAddon = new FitAddon();
  const searchAddon = new SearchAddon();
  term.loadAddon(fitAddon);
  term.loadAddon(searchAddon);
  term.open(container);

  const onDataDispose = term.onData((data) => {
    WriteSSH(sessionId, bytesToBase64(new TextEncoder().encode(data))).catch(console.error);
  });

  const dataEvent = "ssh:data:" + sessionId;
  EventsOn(dataEvent, (dataB64: string) => {
    const binary = atob(dataB64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    term.write(bytes);
  });

  // Forward declare so the onKey closure can read isClosed via the instance.
  const instance: InternalInstance = {
    term,
    fitAddon,
    searchAddon,
    container,
    isClosed: false,
    dispose: () => {
      onDataDispose.dispose();
      onKeyDispose.dispose();
      EventsOff(dataEvent);
      EventsOff(closedEvent);
      term.dispose();
      registry.delete(sessionId);
    },
  };

  const onKeyDispose = term.onKey(({ key }) => {
    if (instance.isClosed && key === "\r") {
      instance.isClosed = false;
      useTerminalStore.getState().reconnectBySession(sessionId);
    }
  });

  const closedEvent = "ssh:closed:" + sessionId;
  EventsOn(closedEvent, () => {
    const hint = i18n.t("ssh.session.closedHint");
    term.write(`\r\n\x1b[31m${hint}\x1b[0m\r\n`);
    instance.isClosed = true;
    useTerminalStore.getState().markClosed(sessionId);
  });

  registry.set(sessionId, instance);
  return instance;
}

export function disposeTerminal(sessionId: string): void {
  const inst = registry.get(sessionId);
  if (inst) inst.dispose();
}

export function getTerminalInstance(sessionId: string): TerminalInstance | undefined {
  return registry.get(sessionId);
}
```

> 关键点：`onKeyDispose` 用 `var`-style 提升不可行（`const` 无 hoist）。解决方法是把 `instance` 先声明，再在它后面声明 `onKeyDispose`，并在 `dispose` 里通过闭包引用 `onKeyDispose`（`dispose` 在调用时才求值，那时 `onKeyDispose` 已定义，TS 编译器在 strict 模式下会报 used-before-defined —— 如果报错，把 `onKeyDispose` 用 `let onKeyDispose: { dispose: () => void };` 先声明再赋值）。

- [ ] **Step 6.4: 跑测试，确认通过**

Run:
```bash
cd frontend && pnpm test --run src/__tests__/terminalRegistry.test.ts
```

Expected: 4 个用例全 PASS。

- [ ] **Step 6.5: 跑 lint + 类型 + 全量前端测试**

Run:
```bash
cd frontend && pnpm tsc --noEmit && pnpm lint && pnpm test --run
```

Expected: 0 errors，所有测试 PASS。

- [ ] **Step 6.6: 提交（合并 Task 4/5/6）**

```bash
git add frontend/src/i18n/locales frontend/src/stores/terminalStore.ts \
        frontend/src/components/terminal/terminalRegistry.ts \
        frontend/src/__tests__/terminalRegistry.test.ts
git commit -m "$(cat <<'EOF'
✨ terminal: 断开后按回车自动重连 (#80)

Why: 当前重连只能走右键菜单,体验差。
新增 reconnectBySession,registry 在 ssh:closed 后翻转 isClosed,
在 onKey 里捕获 Enter 触发重连。提示文案改为含国际化引导。
EOF
)"
```

---

## Phase 7：手动验收

### Task 7: 跑 dev、人工验证两条路径

**Files:** 无

- [ ] **Step 7.1: 启动 dev**

Run:
```bash
make dev
```

- [ ] **Step 7.2: 验证保活**

操作：
1. 在 OpsKat 里连接一台 SSH 资产
2. 用 `tcpdump -i any -nn 'host <ip> and port 22'`（或在服务器侧 `ss -tnp`）观察连接
3. 不在终端输入任何内容，挂机至少 90 秒
4. 在终端按回车，应当**仍能正常**响应（连接没掉）

如果有方便的 NAT 环境（比如本地经过 docker 网桥），等几分钟后回到终端输入命令，应当不再出现 `connect close`。

- [ ] **Step 7.3: 验证回车重连**

操作：
1. 连接 SSH 后，在远端杀掉自己的 sshd session（或拔网线）让连接掉线
2. 终端应显示 `[连接已断开 — 按回车重连]`（中文）/ `[Connection closed — press Enter to reconnect]`（英文）
3. 按回车，应触发重连（看到连接进度提示，最后回到正常 shell）
4. 再断开一次，按字母 `a`，**不应**触发重连
5. 断开后连按 5 次回车，**只**应触发一次重连（防抖）

如果上面任何一步表现异常，回到对应任务调查并修复。

- [ ] **Step 7.4: 准备 PR**

Run:
```bash
git push -u origin ssh-keepalive-and-enter-reconnect
gh pr create --title "✨ ssh: 60s 保活心跳 + 断开后回车重连 (#80)" --body "$(cat <<'EOF'
## Summary

- 在 `internal/pkg/sshkeepalive` 新建共享心跳包，每 60s 发送 `keepalive@openssh.com` 全局请求
- `ssh_svc.sharedClient` 与 `sshpool.poolEntry` 都启用保活，连接关闭时停止
- 终端断开后，xterm 内按回车直接触发重连，提示文案告知用户可以这样做

Closes #80。

详细设计：`docs/superpowers/specs/2026-05-08-ssh-keepalive-reconnect-design.md`。

## Test plan

- [x] `go test ./internal/pkg/sshkeepalive/... -v`
- [x] `go test ./...`
- [x] `cd frontend && pnpm test --run`
- [x] `cd frontend && pnpm tsc --noEmit && pnpm lint`
- [ ] 手动：挂机 90s 后回到终端可正常输入（连接未被 NAT 清掉）
- [ ] 手动：远端断开 SSH 后，xterm 显示提示，回车触发重连，非回车键和重复回车均符合预期
EOF
)"
```

> 注意：如果用户不允许直接 push / 开 PR，停在 Step 7.3，等用户指示。

- [ ] **Step 7.5: 完成**

汇报给用户：worktree 路径、分支名、PR 链接（如果开了）。

---

## 自审清单（agent 在所有任务完成后跑）

- [ ] **Spec 覆盖**：
  - 后端 60s 保活 ✓ Task 1-3
  - sharedClient + poolEntry 双接入 ✓ Task 2-3
  - 回车重连 + 提示文案 ✓ Task 4-6
  - 双向 i18n ✓ Task 4
  - reconnectBySession 反查 ✓ Task 5
  - 防抖（连按 Enter） ✓ Task 6 测试覆盖
  - 测试覆盖（后端 + 前端） ✓ Task 1.1, 6.1
- [ ] **没有遗留 placeholder**
- [ ] **类型一致**：`stopKeepalive func()` 在 `sharedClient` 和 `poolEntry` 命名一致；`reconnectBySession(sessionId)` 在 store 接口和实现签名一致
- [ ] **commit 颗粒**：5 个 commit，对应 4 个改动 + 1 个文档（已在 mobile-poc 分支）

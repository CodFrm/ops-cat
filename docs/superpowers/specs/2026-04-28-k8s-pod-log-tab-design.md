# K8s Pod 日志独立 Tab 设计

## 目标

将 Pod 日志从 Pod 详情面板中分离出来，改为通过独立 tab 打开。每个日志 tab 拥有独立的状态（容器选择、tail 行数、日志流、终端实例），支持同时打开多个 Pod 的日志 tab，关闭时自动停止对应日志流。

## 范围

- `frontend/src/components/k8s/K8sClusterPage.tsx` — tab 系统、日志状态管理
- `frontend/src/components/k8s/K8sLogsPanel.tsx` — 日志面板组件改造
- `frontend/src/components/k8s/K8sLogTerminal.tsx` — xterm 终端组件
- 新增 `log:` 前缀 tab 类型

## 当前状态

日志相关状态在 `K8sClusterPage` 中是全局的：
- `logStreamID`, `logLines`, `logContainer`, `logTailLines`, `logError`
- 日志面板 `<K8sLogsPanel>` 直接嵌入 Pod 详情面板中
- 所有 Pod 共享同一个日志流和终端实例

## 设计方案

### 核心变化：日志状态按 tab 隔离

从全局 state 改为按 tab ID 索引的 Map：

```typescript
interface LogTabState {
  logStreamID: string | null;
  logContainer: string;
  logTailLines: number;
  logError: string | null;
  // xterm 实例通过 ref 或组件内部管理
}

const [logTabStates, setLogTabStates] = useState<Record<string, LogTabState>>({});
```

### 状态管理

**初始化**：当打开 `log:ns:podName` tab 时：
```tsx
const key = `log:${ns}:${podName}`;
const state = logTabStates[key];
if (!state) {
  const defaultContainer = pod.containers[0]?.name || "";
  setLogTabStates(prev => ({
    ...prev,
    [key]: {
      logStreamID: null,
      logContainer: defaultContainer,
      logTailLines: 200,
      logError: null,
    }
  }));
}
```

**更新**：提供 `updateLogTabState(tabId, updater)` 辅助函数，类似：
```tsx
const updateLogTabState = (tabId: string, patch: Partial<LogTabState>) => {
  setLogTabStates(prev => ({
    ...prev,
    [tabId]: { ...prev[tabId], ...patch },
  }));
};
```

**清理**：`closeTab` 时如果关闭的是日志 tab，停止日志流并删除对应状态。

### Tab 类型扩展

新增 `log:` 前缀 tab：
- `id` 格式：`log:${ns}:${podName}`
- `label`：`日志: ${podName}`
- Tab 图标：ScrollText

### Pod 详情面板调整

在 Pod 详情面板中移除 `<K8sLogsPanel>`，改为添加一行操作：

```tsx
<div className="flex items-center gap-2 mt-2">
  <button
    onClick={() => openLogTab(detail.namespace, detail.name)}
    className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs hover:bg-muted"
  >
    <ScrollText className="h-3 w-3" />
    {t("asset.k8sViewPodLogs")}
  </button>
</div>
```

或者直接在容器表格的每行添加"日志"操作列，点击打开该容器的日志 tab。

### 日志面板渲染

新增日志 tab 渲染分支：

```tsx
{activeTabId.startsWith("log:") &&
  (() => {
    const parts = activeTabId.split(":");
    const ns = parts[1];
    const podName = parts.slice(2).join(":");
    const key = activeTabId;
    const state = logTabStates[key];
    const detail = podDetails[`${ns}/${podName}`];
    if (!state || !detail) return null;

    return (
      <div className="max-w-5xl mx-auto p-4">
        <K8sLogsPanel
          tabId={key}
          containers={detail.containers}
          namespace={ns}
          podName={podName}
          logContainer={state.logContainer}
          logTailLines={state.logTailLines}
          logStreamID={state.logStreamID}
          logError={state.logError}
          updateState={(patch) => updateLogTabState(key, patch)}
        />
      </div>
    );
  })()
}
```

### `K8sLogsPanel` 改造

- 接收 `tabId` prop，用于区分不同 tab
- 日志流事件监听按 `tabId` 区分（`k8s:log:${tabId}:${streamID}`）
- 但后端事件格式是 `k8s:log:${streamID}`，所以需要在回调中检查 `logStreamIDRef` 是否匹配当前 tab 的 streamID
- 或者让 `K8sLogsPanel` 内部管理自己的 `logStreamIDRef`

由于 `StartK8sPodLogs` 返回的是全局唯一的 `streamID`，事件也是按 `streamID` 发送的，最安全的做法是：

**方案**：每个 `K8sLogsPanel` 实例内部持有自己的 `logStreamIDRef`，在事件回调中检查当前 streamID 是否属于自己。

```tsx
// K8sLogsPanel.tsx
const myStreamIDRef = useRef<string | null>(null);

const start = () => {
  StartK8sPodLogs(assetId, ns, podName, container, tailLines).then((streamID) => {
    myStreamIDRef.current = streamID;
    updateState({ logStreamID: streamID });
    EventsOn(`k8s:log:${streamID}`, (data) => {
      if (myStreamIDRef.current !== streamID) return; // 已过时
      terminalRef.current?.write(atob(data));
    });
    // ...
  });
};
```

### 关闭日志 tab 时的清理

```tsx
const closeTab = (id: InnerTabId) => {
  if (id.startsWith("log:")) {
    const state = logTabStates[id];
    if (state?.logStreamID) {
      StopK8sPodLogs(state.logStreamID);
    }
    setLogTabStates(prev => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
  }
  // ...existing close logic...
};
```

### i18n 新增

```json
"asset.k8sViewPodLogs": "查看日志"
```

## 风险

- 每个日志 tab 创建一个 xterm 实例，同时打开过多 tab 可能导致内存占用增加
- `EventsOn`/`EventsOff` 需要确保正确配对，避免事件泄漏
- 后端事件格式是否需要调整（当前是 `k8s:log:${streamID}`，无需调整）

## 测试策略

- 前端 lint 和 vitest 通过
- 手动验证：打开 Pod A 日志 tab → 打开 Pod B 日志 tab → 两者同时运行 → 关闭 Pod A 日志 tab → Pod B 日志不受影响 → 切换回 Pod 详情面板确认日志按钮正常

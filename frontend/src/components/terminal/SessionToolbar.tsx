import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Columns2, Rows2, RotateCcw } from "lucide-react";
import { Button } from "@opskat/ui";
import { useTerminalStore } from "@/stores/terminalStore";
import { useTabStore, type TerminalTabMeta } from "@/stores/tabStore";

interface SessionToolbarProps {
  tabId: string;
}

function useUptime(connectedAt: number | undefined, connected: boolean, active: boolean): string {
  const [elapsed, setElapsed] = useState("");
  useEffect(() => {
    if (!connected || !connectedAt) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setElapsed("");
      return;
    }
    const update = () => {
      const secs = Math.floor((Date.now() - connectedAt) / 1000);
      const h = Math.floor(secs / 3600);
      const m = Math.floor((secs % 3600) / 60);
      const s = secs % 60;
      setElapsed(
        h > 0
          ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
          : `${m}:${String(s).padStart(2, "0")}`
      );
    };
    update();
    if (!active) return; // 非活跃 tab 不启动 interval，切回时 effect 会重入并 update 一次
    const id = setInterval(update, 1000);
    return () => clearInterval(id);
  }, [connectedAt, connected, active]);
  return elapsed;
}

export function SessionToolbar({ tabId }: SessionToolbarProps) {
  const { t } = useTranslation();
  const tabData = useTerminalStore((s) => s.tabData[tabId]);
  const splitPane = useTerminalStore((s) => s.splitPane);
  const reconnect = useTerminalStore((s) => s.reconnect);
  const isActive = useTabStore((s) => s.activeTabId === tabId);

  // hooks 必须在所有条件分支之前调用
  const activePane = tabData ? tabData.panes[tabData.activePaneId] : undefined;
  const activeConnected = activePane?.connected ?? false;
  const uptime = useUptime(activePane?.connectedAt, activeConnected, isActive);

  const tabMeta = useTabStore((s) => {
    const t = s.tabs.find((t) => t.id === tabId);
    return t?.meta as TerminalTabMeta | undefined;
  });

  if (!tabData) return null;

  // 连接中的 tab 没有有效的 pane，不显示工具栏
  if (Object.keys(tabData.panes).length === 0) return null;

  const paneValues = Object.values(tabData.panes);
  const anyConnected = paneValues.some((p) => p.connected);

  const hostInfo =
    tabMeta?.username && tabMeta?.host
      ? `${tabMeta.username}@${tabMeta.host}${tabMeta.port !== 22 ? `:${tabMeta.port}` : ""}`
      : tabMeta?.host
        ? `${tabMeta.host}${tabMeta.port !== 22 ? `:${tabMeta.port}` : ""}`
        : "";

  return (
    <div className="flex items-center gap-1.5 px-2 py-1 border-b bg-background shrink-0 text-xs">
      {/* 连接状态指示器 */}
      <span
        className={`h-2 w-2 rounded-full shrink-0 ${anyConnected ? "bg-green-500" : "bg-destructive"}`}
        title={anyConnected ? t("ssh.session.connected") : t("ssh.session.disconnected")}
      />

      {/* 主机信息 */}
      {hostInfo && <span className="font-mono text-muted-foreground select-text truncate max-w-48">{hostInfo}</span>}

      {/* 连接时长 */}
      {uptime && (
        <>
          <span className="text-muted-foreground/40">|</span>
          <span className="font-mono text-muted-foreground tabular-nums">{uptime}</span>
        </>
      )}

      <div className="flex-1" />

      {/* 分割窗格按钮 */}
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("ssh.session.splitH")}
        disabled={!anyConnected}
        onClick={() => splitPane(tabId, "horizontal")}
      >
        <Rows2 className="h-3.5 w-3.5" />
      </Button>
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("ssh.session.splitV")}
        disabled={!anyConnected}
        onClick={() => splitPane(tabId, "vertical")}
      >
        <Columns2 className="h-3.5 w-3.5" />
      </Button>

      {/* 重新连接 */}
      <Button variant="ghost" size="icon-xs" title={t("ssh.session.reconnect")} onClick={() => reconnect(tabId)}>
        <RotateCcw className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

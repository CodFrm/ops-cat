import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Columns2, Rows2, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useTerminalStore } from "@/stores/terminalStore";

interface SessionToolbarProps {
  tabId: string;
}

function useUptime(connectedAt: number | undefined, connected: boolean): string {
  const [elapsed, setElapsed] = useState("");
  useEffect(() => {
    if (!connected || !connectedAt) {
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
    const id = setInterval(update, 1000);
    return () => clearInterval(id);
  }, [connectedAt, connected]);
  return elapsed;
}

export function SessionToolbar({ tabId }: SessionToolbarProps) {
  const { t } = useTranslation();
  const tab = useTerminalStore((s) => s.tabs.find((t) => t.id === tabId));
  const splitPane = useTerminalStore((s) => s.splitPane);
  const reconnect = useTerminalStore((s) => s.reconnect);

  // hooks 必须在所有条件分支之前调用
  const activePane = tab ? tab.panes[tab.activePaneId] : undefined;
  const activeConnected = activePane?.connected ?? false;
  const uptime = useUptime(activePane?.connectedAt, activeConnected);

  if (!tab) return null;

  // 连接中的 tab 没有有效的 pane，不显示工具栏
  if (Object.keys(tab.panes).length === 0) return null;

  const paneValues = Object.values(tab.panes);
  const anyConnected = paneValues.some((p) => p.connected);

  const hostInfo = tab.username && tab.host
    ? `${tab.username}@${tab.host}${tab.port !== 22 ? `:${tab.port}` : ""}`
    : tab.host
      ? `${tab.host}${tab.port !== 22 ? `:${tab.port}` : ""}`
      : "";

  return (
    <div className="flex items-center gap-1.5 px-2 py-1 border-b bg-background shrink-0 text-xs">
      {/* 连接状态指示器 */}
      <span
        className={`h-2 w-2 rounded-full shrink-0 ${
          anyConnected ? "bg-green-500" : "bg-destructive"
        }`}
        title={anyConnected ? t("ssh.session.connected") : t("ssh.session.disconnected")}
      />

      {/* 主机信息 */}
      {hostInfo && (
        <span className="font-mono text-muted-foreground select-text truncate max-w-48">
          {hostInfo}
        </span>
      )}

      {/* 连接时长 */}
      {uptime && (
        <>
          <span className="text-muted-foreground/40">|</span>
          <span className="font-mono text-muted-foreground tabular-nums">
            {uptime}
          </span>
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
      <Button
        variant="ghost"
        size="icon-xs"
        title={t("ssh.session.reconnect")}
        onClick={() => reconnect(tabId)}
      >
        <RotateCcw className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import {
  IconLayoutSidebar,
  IconLayoutSidebarFilled,
  IconLayoutSidebarRight,
  IconLayoutSidebarRightFilled,
} from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger, Tooltip, TooltipContent, TooltipTrigger, cn } from "@opskat/ui";
import { CommandPalette } from "@/components/command/CommandPalette";
import { useFullscreen } from "@/hooks/useFullscreen";
import { Environment } from "../../../wailsjs/runtime/runtime";
import type { asset_entity } from "../../../wailsjs/go/models";

interface TopBarProps {
  commandOpen: boolean;
  onCommandOpenChange: (open: boolean) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  assetTreeCollapsed: boolean;
  onToggleAssetTree: () => void;
  aiPanelCollapsed: boolean;
  onToggleAIPanel: () => void;
}

export function TopBar({
  commandOpen,
  onCommandOpenChange,
  onConnectAsset,
  assetTreeCollapsed,
  onToggleAssetTree,
  aiPanelCollapsed,
  onToggleAIPanel,
}: TopBarProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();

  const [platform, setPlatform] = useState<"darwin" | "windows" | "other">("other");
  useEffect(() => {
    Environment()
      .then((env) => {
        if (env.platform === "darwin") setPlatform("darwin");
        else if (env.platform === "windows") setPlatform("windows");
        else setPlatform("other");
      })
      .catch(() => {});
  }, []);

  // macOS 红绿灯位（非全屏时预留 80px）；Windows 把 WindowControls 区域让出来
  const leftReserve = platform === "darwin" && !isFullscreen ? "pl-20" : "pl-2";
  const rightReserve = platform === "windows" ? "pr-[140px]" : "pr-2";

  return (
    <div
      className={cn(
        "flex h-10 w-full shrink-0 items-center gap-2 border-b border-panel-divider bg-sidebar",
        leftReserve,
        rightReserve
      )}
      style={{ "--wails-draggable": "drag" } as React.CSSProperties}
    >
      {/* 左侧占位，让中间搜索框居中 */}
      <div className="flex-1" />

      {/* 中间：搜索/命令框（按钮形态，点击打开 Popover） */}
      <div
        className="flex w-full max-w-md justify-center"
        style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
      >
        <Popover open={commandOpen} onOpenChange={onCommandOpenChange}>
          <PopoverTrigger asChild>
            <button
              type="button"
              className={cn(
                "flex h-7 w-full items-center gap-2 rounded-md border border-input/50 bg-background/60 px-2.5 text-xs",
                "text-muted-foreground transition-colors hover:bg-background hover:text-foreground",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
              )}
            >
              <Search className="h-4 w-4 shrink-0" />
              <span className="flex-1 truncate text-left">{t("topBar.searchPlaceholder")}</span>
              <kbd className="hidden shrink-0 rounded bg-muted px-1 py-0.5 font-mono text-[10px] sm:inline-block">
                {platform === "darwin" ? "⌘P" : "Ctrl+P"}
              </kbd>
            </button>
          </PopoverTrigger>
          <PopoverContent
            align="center"
            sideOffset={4}
            className="w-[min(640px,90vw)] p-0"
            onOpenAutoFocus={(e) => {
              // 让 CommandPalette 内部的 input 自己取焦
              e.preventDefault();
            }}
          >
            <CommandPalette
              open={commandOpen}
              onClose={() => onCommandOpenChange(false)}
              onConnectAsset={onConnectAsset}
            />
          </PopoverContent>
        </Popover>
      </div>

      {/* 右侧占位 + 控制按钮 */}
      <div className="flex flex-1 items-center justify-end gap-0.5">
        <div style={{ "--wails-draggable": "no-drag" } as React.CSSProperties} className="flex items-center gap-0.5">
          {/* 切换资产列表：实线 = 打开，虚线 = 隐藏 */}
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={onToggleAssetTree}
                aria-pressed={!assetTreeCollapsed}
                className={cn(
                  "flex h-7 w-7 items-center justify-center rounded-md transition-colors hover:bg-accent/50",
                  assetTreeCollapsed ? "text-muted-foreground hover:text-foreground" : "text-foreground"
                )}
              >
                {assetTreeCollapsed ? (
                  <IconLayoutSidebar className="h-4 w-4" stroke={1.75} />
                ) : (
                  <IconLayoutSidebarFilled className="h-4 w-4" />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {assetTreeCollapsed ? t("topBar.showAssetTree") : t("topBar.hideAssetTree")}
            </TooltipContent>
          </Tooltip>

          {/* 切换 AI 对话：实线 = 打开，虚线 = 隐藏 */}
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={onToggleAIPanel}
                aria-pressed={!aiPanelCollapsed}
                className={cn(
                  "flex h-7 w-7 items-center justify-center rounded-md transition-colors hover:bg-accent/50",
                  aiPanelCollapsed ? "text-muted-foreground hover:text-foreground" : "text-foreground"
                )}
              >
                {aiPanelCollapsed ? (
                  <IconLayoutSidebarRight className="h-4 w-4" stroke={1.75} />
                ) : (
                  <IconLayoutSidebarRightFilled className="h-4 w-4" />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {aiPanelCollapsed ? t("topBar.showAIPanel") : t("topBar.hideAIPanel")}
            </TooltipContent>
          </Tooltip>
        </div>
      </div>
    </div>
  );
}

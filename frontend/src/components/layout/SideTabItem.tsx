import { useContext } from "react";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import {
  cn,
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@opskat/ui";
import { useTabStore, type Tab } from "@/stores/tabStore";
import { useTabDragAndDrop } from "@/hooks/useTabDragAndDrop";
import type { HighlightSegment } from "@/lib/highlightMatch";
import { SideTabDragContext } from "./SideTabDragContext";

interface SideTabItemProps {
  tab: Tab;
  isActive: boolean;
  collapsed: boolean;
  labelSegments: HighlightSegment[];
  icon: React.ComponentType<{ className?: string; style?: React.CSSProperties }>;
  iconStyle?: React.CSSProperties;
  indicatorColor?: string;
  extra?: React.ReactNode;
  onActivate: () => void;
  onClose: () => void;
}

export function SideTabItem({
  tab,
  isActive,
  collapsed,
  labelSegments,
  icon: Icon,
  iconStyle,
  indicatorColor,
  extra,
  onActivate,
  onClose,
}: SideTabItemProps) {
  const { t } = useTranslation();
  const ctx = useContext(SideTabDragContext);
  if (!ctx) throw new Error("SideTabItem must be inside SideTabDragContext");
  const dragProps = useTabDragAndDrop(tab.id, ctx);
  const globalIndex = ctx.tabs.findIndex((x) => x.id === tab.id);
  const total = ctx.tabs.length;
  const plainLabel = tab.label;

  const row = (
    <div
      className={cn(
        "group relative flex items-center gap-2 py-1.5 text-sm cursor-pointer select-none rounded-sm",
        collapsed ? "justify-center px-1.5" : "px-2",
        isActive ? "bg-primary/10 text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
      )}
      title={collapsed ? plainLabel : undefined}
      onClick={onActivate}
      {...dragProps}
    >
      {(isActive || indicatorColor) && (
        <span
          className={cn("absolute left-0 top-1 bottom-1 w-0.5 rounded-r", !indicatorColor && "bg-primary")}
          style={indicatorColor ? { backgroundColor: indicatorColor } : undefined}
        />
      )}
      <Icon className="h-3.5 w-3.5 shrink-0" style={iconStyle} />
      {!collapsed && (
        <>
          <span className="truncate min-w-0 flex-1">
            {labelSegments.map((seg, i) =>
              seg.match ? (
                <mark key={i} className="bg-transparent text-primary font-medium">
                  {seg.text}
                </mark>
              ) : (
                <span key={i}>{seg.text}</span>
              )
            )}
          </span>
          {extra}
          <button
            onClick={(e) => {
              e.stopPropagation();
              onClose();
            }}
            className={cn(
              "shrink-0 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-opacity",
              isActive ? "opacity-100" : "opacity-0 group-hover:opacity-100"
            )}
            aria-label={t("tab.close")}
          >
            <X className="h-3 w-3" />
          </button>
        </>
      )}
    </div>
  );

  if (collapsed) {
    return (
      <ContextMenu>
        <ContextMenuTrigger>
          <Tooltip delayDuration={300}>
            <TooltipTrigger asChild>{row}</TooltipTrigger>
            <TooltipContent side="right">{plainLabel}</TooltipContent>
          </Tooltip>
        </ContextMenuTrigger>
        <SideTabContextMenu tabKey={tab.id} globalIndex={globalIndex} total={total} onClose={onClose} />
      </ContextMenu>
    );
  }

  return (
    <ContextMenu>
      <ContextMenuTrigger>{row}</ContextMenuTrigger>
      <SideTabContextMenu tabKey={tab.id} globalIndex={globalIndex} total={total} onClose={onClose} />
    </ContextMenu>
  );
}

interface ContextMenuInnerProps {
  tabKey: string;
  globalIndex: number;
  total: number;
  onClose: () => void;
}

function SideTabContextMenu({ tabKey, globalIndex, total, onClose }: ContextMenuInnerProps) {
  const { t } = useTranslation();
  return (
    <ContextMenuContent>
      <ContextMenuItem onClick={onClose}>{t("tab.close")}</ContextMenuItem>
      <ContextMenuItem onClick={() => useTabStore.getState().closeOtherTabs(tabKey)} disabled={total <= 1}>
        {t("tab.closeOthers")}
      </ContextMenuItem>
      <ContextMenuSeparator />
      <ContextMenuItem onClick={() => useTabStore.getState().closeLeftTabs(tabKey)} disabled={globalIndex <= 0}>
        {t("tab.closeLeft")}
      </ContextMenuItem>
      <ContextMenuItem
        onClick={() => useTabStore.getState().closeRightTabs(tabKey)}
        disabled={globalIndex >= total - 1}
      >
        {t("tab.closeRight")}
      </ContextMenuItem>
      {total > 1 && (
        <>
          <ContextMenuSeparator />
          <ContextMenuItem
            onClick={() => useTabStore.getState().moveTabTo(tabKey, globalIndex - 1)}
            disabled={globalIndex <= 0}
          >
            {t("tab.moveLeft")}
          </ContextMenuItem>
          <ContextMenuItem
            onClick={() => useTabStore.getState().moveTabTo(tabKey, globalIndex + 1)}
            disabled={globalIndex >= total - 1}
          >
            {t("tab.moveRight")}
          </ContextMenuItem>
          <ContextMenuItem onClick={() => useTabStore.getState().moveTabTo(tabKey, 0)} disabled={globalIndex <= 0}>
            {t("tab.moveToStart")}
          </ContextMenuItem>
          <ContextMenuItem
            onClick={() => useTabStore.getState().moveTabTo(tabKey, total - 1)}
            disabled={globalIndex >= total - 1}
          >
            {t("tab.moveToEnd")}
          </ContextMenuItem>
        </>
      )}
    </ContextMenuContent>
  );
}

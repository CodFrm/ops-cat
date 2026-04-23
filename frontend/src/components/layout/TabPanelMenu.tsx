import { useTranslation } from "react-i18next";
import { MoreHorizontal } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@opskat/ui";
import { useLayoutStore } from "@/stores/layoutStore";
import { useTabStore } from "@/stores/tabStore";
import { useShortcutStore, formatBinding } from "@/stores/shortcutStore";

interface TabPanelMenuProps {
  mode: "top" | "side";
  onOpenFilter: () => void;
}

export function TabPanelMenu({ mode, onOpenFilter }: TabPanelMenuProps) {
  const { t } = useTranslation();
  const setLayout = useLayoutStore((s) => s.setLayout);
  const toggleVisible = useLayoutStore((s) => s.toggleVisible);
  const switchPanel = useLayoutStore((s) => s.switchPanel);
  const activeSidePanel = useLayoutStore((s) => s.activeSidePanel);
  const shortcuts = useShortcutStore((s) => s.shortcuts);

  const closeAll = () => {
    const { tabs, closeTab } = useTabStore.getState();
    [...tabs].forEach((tab) => closeTab(tab.id));
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="shrink-0 rounded-sm p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label="tab panel menu"
          style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-[220px]" onCloseAutoFocus={(e) => e.preventDefault()}>
        {mode === "top" ? (
          <DropdownMenuItem onClick={() => setLayout("left")}>{t("sideTabs.switchToLeft")}</DropdownMenuItem>
        ) : (
          <DropdownMenuItem onClick={() => setLayout("top")}>{t("sideTabs.switchToTop")}</DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onSelect={() => {
            // Defer so DropdownMenu's DismissableLayer fully unmounts before the
            // Popover's DismissableLayer mounts; otherwise the trailing pointer event
            // on the now-detached menu item is interpreted as "outside" and the
            // Popover closes the moment it opens.
            setTimeout(onOpenFilter, 0);
          }}
        >
          <span className="flex-1">{t("shortcut.panel.filter")}</span>
          <span className="text-xs text-muted-foreground ml-4">{formatBinding(shortcuts["panel.filter"])}</span>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={closeAll}>{t("sideTabs.closeAll")}</DropdownMenuItem>
        {mode === "side" && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={switchPanel}>
              <span className="flex-1">
                {activeSidePanel === "assets" ? t("sideTabs.tabsPanel") : t("sideTabs.assetsPanel")}
              </span>
              <span className="text-xs text-muted-foreground ml-4">{formatBinding(shortcuts["panel.switch"])}</span>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={toggleVisible}>
              <span className="flex-1">{t("sideTabs.hidePanel")}</span>
              <span className="text-xs text-muted-foreground ml-4">{formatBinding(shortcuts["panel.sidebar"])}</span>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

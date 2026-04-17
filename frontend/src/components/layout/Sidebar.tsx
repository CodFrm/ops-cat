import {
  Home,
  Settings,
  KeyRound,
  PanelLeftClose,
  PanelLeftOpen,
  EyeOff,
  Bot,
  ScrollText,
  ArrowRightLeft,
  Server,
  LayoutList,
} from "lucide-react";
import logoLight from "@/assets/images/logo.png";
import logoDark from "@/assets/images/logo-dark.png";
import { useTranslation } from "react-i18next";
import { cn, Tooltip, TooltipContent, TooltipTrigger } from "@opskat/ui";
import { ModeToggle } from "@/components/mode-toggle";
import { useFullscreen } from "@/hooks/useFullscreen";
import { useLayoutStore, type SidePanel } from "@/stores/layoutStore";

interface SidebarProps {
  activePage: string;
  onPageChange: (page: string) => void;
  sidebarCollapsed: boolean;
  onToggleSidebar: () => void;
  onHideSidebar: () => void;
  aiPanelCollapsed: boolean;
  onToggleAIPanel: () => void;
}

export function Sidebar({
  activePage,
  onPageChange,
  sidebarCollapsed,
  onToggleSidebar,
  onHideSidebar,
  aiPanelCollapsed,
  onToggleAIPanel,
}: SidebarProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();

  const tabBarLayout = useLayoutStore((s) => s.tabBarLayout);
  const activeSidePanel = useLayoutStore((s) => s.activeSidePanel);
  const leftPanelVisible = useLayoutStore((s) => s.leftPanelVisible);
  const setActivePanel = useLayoutStore((s) => s.setActivePanel);
  const toggleVisible = useLayoutStore((s) => s.toggleVisible);

  const navItems = [
    { id: "home", icon: Home, label: t("nav.home") },
    { id: "forward", icon: ArrowRightLeft, label: t("nav.forward") },
    { id: "sshkeys", icon: KeyRound, label: t("nav.sshKeys") },
    { id: "audit", icon: ScrollText, label: t("nav.audit") },
  ];

  const sidePanels: Array<{ id: SidePanel; icon: typeof Server; label: string }> = [
    { id: "assets", icon: Server, label: t("sideTabs.assetsPanel") },
    { id: "tabs", icon: LayoutList, label: t("sideTabs.tabsPanel") },
  ];

  const onClickSidePanel = (id: SidePanel) => {
    if (id === activeSidePanel && leftPanelVisible) {
      toggleVisible();
    } else {
      setActivePanel(id);
      if (!leftPanelVisible) toggleVisible();
    }
  };

  return (
    <div className="flex h-full w-14 flex-col items-center border-r border-panel-divider bg-sidebar/80 backdrop-blur-sm">
      {/* Drag region for Wails window */}
      <div
        className={`${isFullscreen ? "h-2" : "h-10"} w-full shrink-0`}
        style={{ "--wails-draggable": "drag" } as React.CSSProperties}
      />

      {/* App logo */}
      <div className="mb-3 flex h-9 w-9 items-center justify-center">
        <img src={logoLight} alt="opskat" className="h-8 w-8 rounded-lg dark:hidden" />
        <img src={logoDark} alt="opskat" className="h-8 w-8 rounded-lg hidden dark:block" />
      </div>

      {/* Navigation */}
      <div className="flex flex-col items-center gap-1">
        {navItems.map((item) => (
          <Tooltip key={item.id}>
            <TooltipTrigger asChild>
              <button
                className={cn(
                  "relative flex h-9 w-9 items-center justify-center rounded-lg transition-colors duration-150",
                  activePage === item.id
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                )}
                onClick={() => onPageChange(item.id)}
              >
                {activePage === item.id && (
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 -translate-x-[calc(50%+1px)] h-4 w-1 rounded-full bg-primary" />
                )}
                <item.icon className="h-4 w-4" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right">{item.label}</TooltipContent>
          </Tooltip>
        ))}

        {tabBarLayout === "left" && (
          <>
            <div className="my-1 h-px w-6 bg-border" />
            {sidePanels.map(({ id, icon: Icon, label }) => {
              const isActive = id === activeSidePanel && leftPanelVisible;
              return (
                <Tooltip key={id}>
                  <TooltipTrigger asChild>
                    <button
                      className={cn(
                        "relative flex h-9 w-9 items-center justify-center rounded-lg transition-colors duration-150",
                        isActive
                          ? "bg-accent text-accent-foreground"
                          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                      )}
                      onClick={() => onClickSidePanel(id)}
                      aria-label={label}
                    >
                      {isActive && (
                        <span className="absolute left-0 top-1/2 -translate-y-1/2 -translate-x-[calc(50%+1px)] h-4 w-1 rounded-full bg-primary" />
                      )}
                      <Icon className="h-4 w-4" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="right">{label}</TooltipContent>
                </Tooltip>
              );
            })}
          </>
        )}
      </div>

      <div className="mt-auto flex flex-col items-center gap-1 pb-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className={cn(
                "relative flex h-9 w-9 items-center justify-center rounded-lg transition-colors duration-150",
                !aiPanelCollapsed
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
              )}
              onClick={onToggleAIPanel}
            >
              <Bot className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">{t("nav.ai")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className={cn(
                "relative flex h-9 w-9 items-center justify-center rounded-lg transition-colors duration-150",
                activePage === "settings"
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
              )}
              onClick={() => onPageChange("settings")}
            >
              {activePage === "settings" && (
                <span className="absolute left-0 top-1/2 -translate-y-1/2 -translate-x-[calc(50%+1px)] h-4 w-1 rounded-full bg-primary" />
              )}
              <Settings className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">{t("nav.settings")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors duration-150"
              onClick={onToggleSidebar}
            >
              {sidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">{t("panel.toggleSidebar")}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors duration-150"
              onClick={onHideSidebar}
            >
              <EyeOff className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="right">{t("panel.hideSidebar")}</TooltipContent>
        </Tooltip>
        <ModeToggle />
      </div>
    </div>
  );
}

import { ChevronLeft, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@opskat/ui";

interface EdgeRevealStripProps {
  onClick: () => void;
  side?: "left" | "right";
  titleKey?: string;
}

export function EdgeRevealStrip({ onClick, side = "left", titleKey }: EdgeRevealStripProps) {
  const { t } = useTranslation();
  const isLeft = side === "left";
  const Icon = isLeft ? ChevronRight : ChevronLeft;
  const fallbackKey = isLeft ? "panel.showSidebar" : "panel.showAIPanel";

  return (
    <button
      className={cn(
        "fixed top-0 bottom-0 z-40 w-1 hover:w-5 overflow-hidden cursor-pointer flex items-center justify-center transition-all duration-200 bg-transparent hover:bg-muted/60 hover:backdrop-blur-sm group",
        isLeft ? "left-0" : "right-0"
      )}
      onClick={onClick}
      title={t(titleKey ?? fallbackKey)}
    >
      <Icon className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity duration-200" />
    </button>
  );
}

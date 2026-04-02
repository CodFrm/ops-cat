import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { cn, Button } from "@opskat/ui";
import { RotateCcw } from "lucide-react";
import {
  useShortcutStore,
  SHORTCUT_ACTIONS,
  DEFAULT_SHORTCUTS,
  formatBinding,
  isMac,
  type ShortcutAction,
  type ShortcutBinding,
} from "@/stores/shortcutStore";

export function ShortcutSettings() {
  const { t } = useTranslation();
  const { shortcuts, updateShortcut, resetShortcut, resetAll, setIsRecording } = useShortcutStore();
  const [recording, setRecording] = useState<ShortcutAction | null>(null);

  const startRecording = (action: ShortcutAction) => {
    setRecording(action);
    setIsRecording(true);
  };

  const stopRecording = useCallback(() => {
    setRecording(null);
    setIsRecording(false);
  }, [setIsRecording]);

  const handleRecord = useCallback(
    (e: KeyboardEvent) => {
      if (!recording) return;

      if (e.key === "Escape") {
        stopRecording();
        return;
      }

      // Ignore modifier-only presses
      if (["Meta", "Control", "Shift", "Alt"].includes(e.key)) return;

      e.preventDefault();
      e.stopPropagation();

      const binding: ShortcutBinding = {
        code: e.code,
        mod: isMac ? e.metaKey : e.ctrlKey,
        shift: e.shiftKey,
        alt: e.altKey,
      };

      updateShortcut(recording, binding);
      stopRecording();
    },
    [recording, updateShortcut, stopRecording]
  );

  useEffect(() => {
    if (!recording) return;
    window.addEventListener("keydown", handleRecord, true);
    return () => window.removeEventListener("keydown", handleRecord, true);
  }, [recording, handleRecord]);

  // Cancel recording on click outside
  useEffect(() => {
    if (!recording) return;
    const cancel = () => stopRecording();
    // Delay to avoid the click that started recording
    const timer = setTimeout(() => {
      window.addEventListener("mousedown", cancel);
    }, 100);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("mousedown", cancel);
    };
  }, [recording, stopRecording]);

  const isCustomized = (action: ShortcutAction) => {
    const def = DEFAULT_SHORTCUTS[action];
    const cur = shortcuts[action];
    return cur.code !== def.code || cur.mod !== def.mod || cur.shift !== def.shift || cur.alt !== def.alt;
  };

  return (
    <div className="space-y-4">
      <div className="space-y-0.5">
        {SHORTCUT_ACTIONS.map((action) => (
          <div key={action} className="flex items-center justify-between py-2 px-2 rounded hover:bg-muted/50">
            <span className="text-sm">{t(`shortcut.${action}`)}</span>
            <div className="flex items-center gap-1">
              <button
                className={cn(
                  "px-2.5 py-1 rounded text-xs font-mono min-w-[80px] text-center transition-colors",
                  recording === action
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted hover:bg-muted/80 cursor-pointer"
                )}
                onClick={(e) => {
                  e.stopPropagation();
                  startRecording(action);
                }}
              >
                {recording === action ? t("shortcut.recording") : formatBinding(shortcuts[action])}
              </button>
              {isCustomized(action) && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6"
                  onClick={() => resetShortcut(action)}
                  title={t("shortcut.reset")}
                >
                  <RotateCcw className="h-3 w-3" />
                </Button>
              )}
            </div>
          </div>
        ))}
      </div>
      <Button variant="outline" size="sm" onClick={resetAll}>
        {t("shortcut.resetAll")}
      </Button>
    </div>
  );
}

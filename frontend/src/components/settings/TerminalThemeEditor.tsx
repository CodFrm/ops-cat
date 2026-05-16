import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Input, Label, Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@opskat/ui";
import { TerminalTheme } from "@/data/terminalThemes";
import { toast } from "sonner";

interface TerminalThemeEditorProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  theme?: TerminalTheme; // 编辑时传入，新建时不传
  onSave: (theme: TerminalTheme) => void;
}

const defaultCustomTheme: Omit<TerminalTheme, "id" | "name"> = {
  background: "#1e1e2e",
  foreground: "#cdd6f4",
  cursor: "#f5e0dc",
  cursorAccent: "#1e1e2e",
  selectionBackground: "#45475a",
  black: "#45475a",
  red: "#f38ba8",
  green: "#a6e3a1",
  yellow: "#f9e2af",
  blue: "#89b4fa",
  magenta: "#f5c2e7",
  cyan: "#94e2d5",
  white: "#bac2de",
  brightBlack: "#585b70",
  brightRed: "#f38ba8",
  brightGreen: "#a6e3a1",
  brightYellow: "#f9e2af",
  brightBlue: "#89b4fa",
  brightMagenta: "#f5c2e7",
  brightCyan: "#94e2d5",
  brightWhite: "#a6adc8",
};

const colorFields: { key: keyof Omit<TerminalTheme, "id" | "name">; labelKey: string }[] = [
  { key: "background", labelKey: "terminal.color.background" },
  { key: "foreground", labelKey: "terminal.color.foreground" },
  { key: "cursor", labelKey: "terminal.color.cursor" },
  { key: "cursorAccent", labelKey: "terminal.color.cursorAccent" },
  { key: "selectionBackground", labelKey: "terminal.color.selection" },
  { key: "black", labelKey: "terminal.color.black" },
  { key: "red", labelKey: "terminal.color.red" },
  { key: "green", labelKey: "terminal.color.green" },
  { key: "yellow", labelKey: "terminal.color.yellow" },
  { key: "blue", labelKey: "terminal.color.blue" },
  { key: "magenta", labelKey: "terminal.color.magenta" },
  { key: "cyan", labelKey: "terminal.color.cyan" },
  { key: "white", labelKey: "terminal.color.white" },
  { key: "brightBlack", labelKey: "terminal.color.brightBlack" },
  { key: "brightRed", labelKey: "terminal.color.brightRed" },
  { key: "brightGreen", labelKey: "terminal.color.brightGreen" },
  { key: "brightYellow", labelKey: "terminal.color.brightYellow" },
  { key: "brightBlue", labelKey: "terminal.color.brightBlue" },
  { key: "brightMagenta", labelKey: "terminal.color.brightMagenta" },
  { key: "brightCyan", labelKey: "terminal.color.brightCyan" },
  { key: "brightWhite", labelKey: "terminal.color.brightWhite" },
];

interface TerminalThemeDraftState {
  sourceKey: string;
  name: string;
  colors: Record<string, string>;
}

function getThemeEditorSourceKey(theme?: TerminalTheme) {
  return theme?.id || "new";
}

function getThemeColors(theme?: TerminalTheme): Record<string, string> {
  const colors: Record<string, string> = {};
  for (const field of colorFields) {
    colors[field.key] = theme?.[field.key] || defaultCustomTheme[field.key] || "";
  }
  return colors;
}

function createThemeDraftState(sourceKey: string, theme?: TerminalTheme): TerminalThemeDraftState {
  return {
    sourceKey,
    name: theme?.name || "",
    colors: getThemeColors(theme),
  };
}

export function TerminalThemeEditor({ open, onOpenChange, theme, onSave }: TerminalThemeEditorProps) {
  const { t } = useTranslation();
  const sourceKey = getThemeEditorSourceKey(theme);
  const initialDraft = createThemeDraftState(sourceKey, theme);
  const [draftState, setDraftState] = useState(initialDraft);
  const draft = draftState.sourceKey === sourceKey ? draftState : initialDraft;
  const { name, colors } = draft;

  const updateDraft = (patch: Partial<Omit<TerminalThemeDraftState, "sourceKey">>) => {
    setDraftState((current) => ({
      ...(current.sourceKey === sourceKey ? current : initialDraft),
      ...patch,
      sourceKey,
    }));
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setDraftState(initialDraft);
    }
    onOpenChange(nextOpen);
  };

  const handleColorChange = (key: string, value: string) => {
    updateDraft({ colors: { ...colors, [key]: value } });
  };

  const handleSave = () => {
    if (!name.trim()) {
      toast.error(t("terminal.nameRequired"));
      return;
    }
    const result: TerminalTheme = {
      id: theme?.id || `custom-${Date.now()}`,
      name: name.trim(),
      ...defaultCustomTheme,
    };
    for (const f of colorFields) {
      if (colors[f.key]) {
        (result as unknown as Record<string, string>)[f.key] = colors[f.key];
      }
    }
    onSave(result);
    handleOpenChange(false);
  };

  const handleExport = () => {
    const result: TerminalTheme = {
      id: theme?.id || `custom-${Date.now()}`,
      name: name.trim() || "Untitled",
      ...defaultCustomTheme,
    };
    for (const f of colorFields) {
      if (colors[f.key]) {
        (result as unknown as Record<string, string>)[f.key] = colors[f.key];
      }
    }
    const json = JSON.stringify(result, null, 2);
    navigator.clipboard.writeText(json);
    toast.success(t("terminal.exportedToClipboard"));
  };

  const handleImport = async () => {
    try {
      const text = await navigator.clipboard.readText();
      const parsed = JSON.parse(text) as TerminalTheme;
      if (!parsed.background || !parsed.foreground) {
        toast.error(t("terminal.invalidThemeJson"));
        return;
      }
      updateDraft({ name: parsed.name || "", colors: getThemeColors(parsed) });
      toast.success(t("terminal.importedFromClipboard"));
    } catch {
      toast.error(t("terminal.invalidThemeJson"));
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{theme ? t("terminal.editTheme") : t("terminal.newTheme")}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid gap-2">
            <Label>{t("terminal.themeName")}</Label>
            <Input
              value={name}
              onChange={(e) => updateDraft({ name: e.target.value })}
              placeholder={t("terminal.themeNamePlaceholder")}
            />
          </div>

          {/* 预览条 */}
          <div
            className="rounded-md p-3 font-mono text-sm"
            style={{
              background: colors.background || "#1e1e2e",
              color: colors.foreground || "#cdd6f4",
            }}
          >
            <div>$ ls -la</div>
            <div className="flex gap-2 flex-wrap">
              {["black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"].map((c) => (
                <span key={c} style={{ color: colors[c] }}>
                  {c}
                </span>
              ))}
            </div>
            <div className="flex gap-2 flex-wrap">
              {[
                "brightBlack",
                "brightRed",
                "brightGreen",
                "brightYellow",
                "brightBlue",
                "brightMagenta",
                "brightCyan",
                "brightWhite",
              ].map((c) => (
                <span key={c} style={{ color: colors[c] }}>
                  {c.replace("bright", "b.")}
                </span>
              ))}
            </div>
          </div>

          {/* 基础色 */}
          <div className="space-y-2">
            <h4 className="text-sm font-medium">{t("terminal.baseColors")}</h4>
            <div className="grid grid-cols-2 gap-2">
              {colorFields.slice(0, 5).map((f) => (
                <div key={f.key} className="flex items-center gap-2">
                  <input
                    type="color"
                    value={colors[f.key] || "#000000"}
                    onChange={(e) => handleColorChange(f.key, e.target.value)}
                    className="h-8 w-8 rounded border cursor-pointer shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <Label className="text-xs truncate block">{t(f.labelKey)}</Label>
                    <Input
                      value={colors[f.key] || ""}
                      onChange={(e) => handleColorChange(f.key, e.target.value)}
                      className="h-7 text-xs font-mono"
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* ANSI 标准色 */}
          <div className="space-y-2">
            <h4 className="text-sm font-medium">{t("terminal.normalColors")}</h4>
            <div className="grid grid-cols-2 gap-2">
              {colorFields.slice(5, 13).map((f) => (
                <div key={f.key} className="flex items-center gap-2">
                  <input
                    type="color"
                    value={colors[f.key] || "#000000"}
                    onChange={(e) => handleColorChange(f.key, e.target.value)}
                    className="h-8 w-8 rounded border cursor-pointer shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <Label className="text-xs truncate block">{t(f.labelKey)}</Label>
                    <Input
                      value={colors[f.key] || ""}
                      onChange={(e) => handleColorChange(f.key, e.target.value)}
                      className="h-7 text-xs font-mono"
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* ANSI 亮色 */}
          <div className="space-y-2">
            <h4 className="text-sm font-medium">{t("terminal.brightColors")}</h4>
            <div className="grid grid-cols-2 gap-2">
              {colorFields.slice(13).map((f) => (
                <div key={f.key} className="flex items-center gap-2">
                  <input
                    type="color"
                    value={colors[f.key] || "#000000"}
                    onChange={(e) => handleColorChange(f.key, e.target.value)}
                    className="h-8 w-8 rounded border cursor-pointer shrink-0"
                  />
                  <div className="min-w-0 flex-1">
                    <Label className="text-xs truncate block">{t(f.labelKey)}</Label>
                    <Input
                      value={colors[f.key] || ""}
                      onChange={(e) => handleColorChange(f.key, e.target.value)}
                      className="h-7 text-xs font-mono"
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        <DialogFooter className="flex-row gap-2">
          <div className="flex gap-2 mr-auto">
            <Button variant="outline" size="sm" onClick={handleImport}>
              {t("terminal.importJson")}
            </Button>
            <Button variant="outline" size="sm" onClick={handleExport}>
              {t("terminal.exportJson")}
            </Button>
          </div>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleSave}>{t("action.save")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

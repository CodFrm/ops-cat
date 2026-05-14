import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  cn,
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Separator,
} from "@opskat/ui";
import { useTheme, useResolvedTheme } from "@/components/theme-provider";
import { Plus, Pencil, Trash2 } from "lucide-react";
import { useTerminalThemeStore, SCROLLBACK_MIN, SCROLLBACK_MAX } from "@/stores/terminalThemeStore";
import { builtinThemes, defaultLightTheme, defaultDarkTheme, TerminalTheme } from "@/data/terminalThemes";
import {
  CUSTOM_TERMINAL_FONT_PRESET_ID,
  DEFAULT_TERMINAL_FONT_PRESET_ID,
  buildTerminalFontGroups,
  loadInstalledFonts,
  quoteFamilyName,
  resolveDefaultFontPrimary,
  terminalFontPresets,
} from "@/data/terminalFonts";
import { TerminalThemeEditor } from "@/components/settings/TerminalThemeEditor";

export function AppearanceSection() {
  const { t, i18n } = useTranslation();
  const { theme, setTheme } = useTheme();

  const [startupTab, setStartupTab] = useState(() => localStorage.getItem("startup_tab") || "last");

  const handleLanguageChange = (lng: string) => {
    i18n.changeLanguage(lng);
    localStorage.setItem("language", lng);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("appearance.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-2">
          <Label>{t("theme.label")}</Label>
          <Select value={theme} onValueChange={setTheme as (v: string) => void}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="light">{t("theme.light")}</SelectItem>
              <SelectItem value="dark">{t("theme.dark")}</SelectItem>
              <SelectItem value="system">{t("theme.system")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Separator />
        <div className="grid gap-2">
          <Label>{t("language.label")}</Label>
          <Select value={i18n.language} onValueChange={handleLanguageChange}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="zh-CN">{t("language.zh-CN")}</SelectItem>
              <SelectItem value="en">{t("language.en")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Separator />
        <div className="grid gap-2">
          <Label>{t("appearance.startupTab")}</Label>
          <Select
            value={startupTab}
            onValueChange={(v) => {
              localStorage.setItem("startup_tab", v);
              setStartupTab(v);
            }}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="last">{t("appearance.startupTabLast")}</SelectItem>
              <SelectItem value="home">{t("appearance.startupTabHome")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </Card>
  );
}

export function TerminalSection() {
  const { t } = useTranslation();
  const {
    selectedThemeId,
    setSelectedThemeId,
    fontSize,
    setFontSize,
    fontPresetId,
    customFontFamily,
    setFontPresetId,
    setCustomFontFamily,
    scrollback,
    setScrollback,
    customThemes,
    addCustomTheme,
    updateCustomTheme,
    removeCustomTheme,
  } = useTerminalThemeStore();
  const resolvedTheme = useResolvedTheme();
  const [themeEditorOpen, setThemeEditorOpen] = useState(false);
  const [editingTheme, setEditingTheme] = useState<TerminalTheme | undefined>(undefined);
  const [installedFonts, setInstalledFonts] = useState<string[] | null | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    loadInstalledFonts().then((fonts) => {
      if (!cancelled) setInstalledFonts(fonts);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const { recommendedFonts, otherFonts } = useMemo(() => buildTerminalFontGroups(installedFonts), [installedFonts]);
  const fontOptions = useMemo(() => [...recommendedFonts, ...otherFonts], [recommendedFonts, otherFonts]);

  const defaultResolvedName = useMemo(() => resolveDefaultFontPrimary(installedFonts ?? null), [installedFonts]);

  // Validity check: keep the currently-stored id as the select value as long
  // as it resolves to something — either a hardcoded preset, the custom mode,
  // or a system family we just loaded. Unknown values during initial load are
  // optimistically preserved so the dropdown doesn't flash "Default".
  const fontSelectValue = useMemo(() => {
    if (fontPresetId === CUSTOM_TERMINAL_FONT_PRESET_ID || fontPresetId === DEFAULT_TERMINAL_FONT_PRESET_ID) {
      return fontPresetId;
    }
    const selectedOption = fontOptions.find((option) => option.value === fontPresetId || option.name === fontPresetId);
    if (selectedOption) return selectedOption.value;
    if (installedFonts === undefined) return fontPresetId;
    return terminalFontPresets.some((preset) => preset.id === fontPresetId)
      ? fontPresetId
      : DEFAULT_TERMINAL_FONT_PRESET_ID;
  }, [fontOptions, fontPresetId, installedFonts]);

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("terminal.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Font family */}
          <div className="grid gap-2">
            <Label>{t("terminal.fontFamily")}</Label>
            <Select value={fontSelectValue} onValueChange={setFontPresetId}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="min-w-[18rem] max-h-[28rem]">
                <SelectItem value={DEFAULT_TERMINAL_FONT_PRESET_ID}>
                  <span className="min-w-0 flex-1 truncate">
                    {t("terminal.defaultFont")}
                    {defaultResolvedName && (
                      <span
                        className="text-muted-foreground ml-2"
                        style={{ fontFamily: quoteFamilyName(defaultResolvedName) }}
                      >
                        ({defaultResolvedName})
                      </span>
                    )}
                  </span>
                </SelectItem>
                <SelectItem value={CUSTOM_TERMINAL_FONT_PRESET_ID}>{t("terminal.customFont")}</SelectItem>
                {recommendedFonts.length > 0 && (
                  <>
                    <SelectSeparator />
                    <SelectGroup>
                      <SelectLabel>{t("terminal.fontRecommended")}</SelectLabel>
                      {recommendedFonts.map((font) => (
                        <SelectItem key={`rec-${font.value}`} value={font.value}>
                          <span className="min-w-0 flex-1 truncate" style={{ fontFamily: quoteFamilyName(font.name) }}>
                            {font.name}
                          </span>
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </>
                )}
                {otherFonts.length > 0 && (
                  <>
                    <SelectSeparator />
                    <SelectGroup>
                      <SelectLabel>{t("terminal.fontOther")}</SelectLabel>
                      {otherFonts.map((font) => (
                        <SelectItem key={`oth-${font.value}`} value={font.value}>
                          <span className="min-w-0 flex-1 truncate" style={{ fontFamily: quoteFamilyName(font.name) }}>
                            {font.name}
                          </span>
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </>
                )}
              </SelectContent>
            </Select>
            {fontPresetId === CUSTOM_TERMINAL_FONT_PRESET_ID && (
              <Input
                value={customFontFamily}
                onChange={(e) => setCustomFontFamily(e.target.value)}
                placeholder={t("terminal.customFontPlaceholder")}
              />
            )}
          </div>

          {/* Font size */}
          <div className="grid gap-2">
            <Label>{t("terminal.fontSize")}</Label>
            <div className="flex items-center gap-2">
              <Input
                type="number"
                min={8}
                max={32}
                value={fontSize}
                onChange={(e) => setFontSize(Number(e.target.value))}
                className="w-24"
              />
              <span className="text-sm text-muted-foreground">px</span>
            </div>
          </div>

          {/* Scrollback */}
          <div className="grid gap-2">
            <Label>{t("terminal.scrollback")}</Label>
            <div className="flex items-center gap-2">
              <Input
                type="number"
                min={SCROLLBACK_MIN}
                max={SCROLLBACK_MAX}
                step={100}
                value={scrollback}
                onChange={(e) => setScrollback(Number(e.target.value))}
                className="w-32"
              />
              <span className="text-sm text-muted-foreground">{t("terminal.scrollbackUnit")}</span>
            </div>
            <p className="text-xs text-muted-foreground">{t("terminal.scrollbackHint")}</p>
          </div>

          <Separator />

          {/* Builtin themes */}
          <div className="space-y-2">
            <Label>{t("terminal.builtinThemes")}</Label>
            <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-2">
              {/* Default (follows app theme) */}
              <button
                onClick={() => setSelectedThemeId("default")}
                className={cn(
                  "rounded-md border p-2 text-left transition-all hover:ring-2 hover:ring-primary/50",
                  selectedThemeId === "default" && "ring-2 ring-primary"
                )}
              >
                {(() => {
                  const dt = resolvedTheme === "dark" ? defaultDarkTheme : defaultLightTheme;
                  return (
                    <div
                      className="rounded h-10 mb-1.5 flex items-end p-1 gap-0.5"
                      style={{ background: dt.background }}
                    >
                      {[dt.red, dt.green, dt.yellow, dt.blue, dt.magenta, dt.cyan].map((c, i) => (
                        <div key={i} className="w-2 h-3 rounded-sm" style={{ background: c }} />
                      ))}
                    </div>
                  );
                })()}
                <div className="text-xs truncate font-medium">{t("terminal.default")}</div>
              </button>
              {builtinThemes.map((bt) => (
                <button
                  key={bt.id}
                  onClick={() => setSelectedThemeId(bt.id)}
                  className={cn(
                    "rounded-md border p-2 text-left transition-all hover:ring-2 hover:ring-primary/50",
                    selectedThemeId === bt.id && "ring-2 ring-primary"
                  )}
                >
                  {/* Color preview */}
                  <div className="rounded h-10 mb-1.5 flex items-end p-1 gap-0.5" style={{ background: bt.background }}>
                    {[bt.red, bt.green, bt.yellow, bt.blue, bt.magenta, bt.cyan].map((c, i) => (
                      <div key={i} className="w-2 h-3 rounded-sm" style={{ background: c }} />
                    ))}
                  </div>
                  <div className="text-xs truncate font-medium">{bt.name}</div>
                </button>
              ))}
            </div>
          </div>

          <Separator />

          {/* Custom themes */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>{t("terminal.customThemes")}</Label>
              <Button
                variant="outline"
                size="sm"
                className="gap-1"
                onClick={() => {
                  setEditingTheme(undefined);
                  setThemeEditorOpen(true);
                }}
              >
                <Plus className="h-3.5 w-3.5" />
                {t("terminal.newTheme")}
              </Button>
            </div>
            {customThemes.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("terminal.noCustomThemes")}</p>
            ) : (
              <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-2">
                {customThemes.map((ct) => (
                  <div
                    key={ct.id}
                    className={cn(
                      "group relative rounded-md border p-2 text-left transition-all hover:ring-2 hover:ring-primary/50 cursor-pointer",
                      selectedThemeId === ct.id && "ring-2 ring-primary"
                    )}
                    onClick={() => setSelectedThemeId(ct.id)}
                  >
                    <div
                      className="rounded h-10 mb-1.5 flex items-end p-1 gap-0.5"
                      style={{ background: ct.background }}
                    >
                      {[ct.red, ct.green, ct.yellow, ct.blue, ct.magenta, ct.cyan].map((c, i) => (
                        <div key={i} className="w-2 h-3 rounded-sm" style={{ background: c }} />
                      ))}
                    </div>
                    <div className="text-xs truncate font-medium">{ct.name}</div>
                    {/* Edit/Delete */}
                    <div className="absolute top-1 right-1 hidden group-hover:flex gap-0.5">
                      <button
                        className="rounded p-0.5 bg-background/80 hover:bg-muted"
                        onClick={(e) => {
                          e.stopPropagation();
                          setEditingTheme(ct);
                          setThemeEditorOpen(true);
                        }}
                      >
                        <Pencil className="h-3 w-3" />
                      </button>
                      <button
                        className="rounded p-0.5 bg-background/80 hover:bg-destructive/20"
                        onClick={(e) => {
                          e.stopPropagation();
                          removeCustomTheme(ct.id);
                        }}
                      >
                        <Trash2 className="h-3 w-3" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Terminal theme editor */}
      <TerminalThemeEditor
        open={themeEditorOpen}
        onOpenChange={setThemeEditorOpen}
        theme={editingTheme}
        onSave={(theme) => {
          if (editingTheme) {
            updateCustomTheme(theme);
          } else {
            addCustomTheme(theme);
          }
          setSelectedThemeId(theme.id);
        }}
      />
    </>
  );
}

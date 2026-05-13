export interface TerminalFontPreset {
  id: string;
  name: string;
  fontFamily: string;
}

export interface TerminalFontOption {
  value: string;
  name: string;
}

export const DEFAULT_TERMINAL_FONT_PRESET_ID = "default";
export const CUSTOM_TERMINAL_FONT_PRESET_ID = "custom";
// Nerd Font families come first so per-glyph CSS font fallback supplies icons
// (apple, folder, clock, etc.) for prompts like powerlevel10k / starship when
// the user's primary font is a non-patched monospace. Both naming conventions
// are listed: short "NFM" (nerd-fonts v3.1+) and long "Nerd Font Mono" (v3.0).
export const DEFAULT_TERMINAL_FONT_FALLBACKS = [
  "'JetBrainsMono NFM'",
  "'JetBrainsMono Nerd Font Mono'",
  "'MesloLGM NF'",
  "'MesloLGM Nerd Font'",
  "'FiraCode NFM'",
  "'FiraCode Nerd Font Mono'",
  "'JetBrains Mono'",
  "'Fira Code'",
  "'Cascadia Code'",
  "Menlo",
  "monospace",
];
export const DEFAULT_TERMINAL_FONT_FAMILY = DEFAULT_TERMINAL_FONT_FALLBACKS.join(", ");
const TRAILING_GENERIC_FONT_FAMILY_RE = /(?:,\s*(?:ui-monospace|monospace|serif|sans-serif)\s*)+$/i;

export const terminalFontPresets: TerminalFontPreset[] = [
  {
    id: DEFAULT_TERMINAL_FONT_PRESET_ID,
    name: "Default",
    fontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
  },
  {
    id: "jetbrains-mono",
    name: "JetBrains Mono",
    fontFamily: "'JetBrains Mono'",
  },
  {
    id: "fira-code",
    name: "Fira Code",
    fontFamily: "'Fira Code'",
  },
  {
    id: "cascadia-code",
    name: "Cascadia Code",
    fontFamily: "'Cascadia Code'",
  },
  {
    id: "sf-mono",
    name: "SF Mono",
    fontFamily: "'SF Mono'",
  },
  {
    id: "menlo",
    name: "Menlo",
    fontFamily: "Menlo",
  },
  {
    id: "monaco",
    name: "Monaco",
    fontFamily: "Monaco",
  },
  {
    id: "consolas",
    name: "Consolas",
    fontFamily: "Consolas",
  },
  {
    id: "source-code-pro",
    name: "Source Code Pro",
    fontFamily: "'Source Code Pro'",
  },
  {
    id: "hack",
    name: "Hack",
    fontFamily: "Hack",
  },
  {
    id: "ibm-plex-mono",
    name: "IBM Plex Mono",
    fontFamily: "'IBM Plex Mono'",
  },
  {
    id: "roboto-mono",
    name: "Roboto Mono",
    fontFamily: "'Roboto Mono'",
  },
  {
    id: "noto-sans-mono",
    name: "Noto Sans Mono",
    fontFamily: "'Noto Sans Mono'",
  },
  {
    id: "ubuntu-mono",
    name: "Ubuntu Mono",
    fontFamily: "'Ubuntu Mono'",
  },
  {
    id: "dejavu-sans-mono",
    name: "DejaVu Sans Mono",
    fontFamily: "'DejaVu Sans Mono'",
  },
];

export function normalizeTerminalFontFamily(fontFamily: string): string {
  return fontFamily.trim();
}

export function resolveTerminalFontFamily(fontFamily: string): string {
  const normalized = normalizeTerminalFontFamily(fontFamily);
  return normalized || DEFAULT_TERMINAL_FONT_FAMILY;
}

function splitFontFamilyList(fontFamily: string): string[] {
  const families: string[] = [];
  let current = "";
  let quote: string | undefined;

  for (const char of fontFamily) {
    if ((char === "'" || char === '"') && !quote) {
      quote = char;
      current += char;
      continue;
    }
    if (char === quote) {
      quote = undefined;
      current += char;
      continue;
    }
    if (char === "," && !quote) {
      const family = current.trim();
      if (family) families.push(family);
      current = "";
      continue;
    }
    current += char;
  }

  const family = current.trim();
  if (family) families.push(family);
  return families;
}

function normalizeFontFamilyToken(fontFamily: string): string {
  return fontFamily
    .trim()
    .replace(/^['"]|['"]$/g, "")
    .toLowerCase();
}

export function withTerminalFontFallback(fontFamily: string): string {
  const normalized = normalizeTerminalFontFamily(fontFamily);
  if (!normalized || normalized === DEFAULT_TERMINAL_FONT_FAMILY) return DEFAULT_TERMINAL_FONT_FAMILY;

  const primaryFontFamily = normalized.replace(TRAILING_GENERIC_FONT_FAMILY_RE, "").trim() || normalized;
  const primaryFonts = splitFontFamilyList(primaryFontFamily);
  const usedFontNames = new Set(primaryFonts.map(normalizeFontFamilyToken));
  const fallbackFonts = DEFAULT_TERMINAL_FONT_FALLBACKS.filter(
    (fallbackFont) => !usedFontNames.has(normalizeFontFamilyToken(fallbackFont))
  );

  return [...primaryFonts, ...fallbackFonts].join(", ");
}

export function findTerminalFontPreset(id: string): TerminalFontPreset | undefined {
  return terminalFontPresets.find((preset) => preset.id === id);
}

// Recommended families shown at the top of the settings font dropdown when
// they are detected as installed. Order is curated (not alphabetical): Nerd
// Fonts first so prompts with powerline / icon glyphs render out of the box,
// then plain mono fonts in popularity order. Both v3.1+ short and v3.0 long
// Nerd Font naming conventions are listed.
export const RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES: string[] = [
  "JetBrainsMono NFM",
  "JetBrainsMono Nerd Font Mono",
  "MesloLGM NF",
  "MesloLGM Nerd Font",
  "MesloLGS NF",
  "MesloLGS Nerd Font",
  "FiraCode NFM",
  "FiraCode Nerd Font Mono",
  "Hack NFM",
  "Hack Nerd Font Mono",
  "CaskaydiaCove NF",
  "CaskaydiaCove Nerd Font",
  "JetBrains Mono",
  "Fira Code",
  "Cascadia Code",
  "Cascadia Mono",
  "SF Mono",
  "Menlo",
  "Monaco",
  "Consolas",
  "Source Code Pro",
  "Hack",
  "IBM Plex Mono",
  "Roboto Mono",
  "Noto Sans Mono",
  "Ubuntu Mono",
  "DejaVu Sans Mono",
  "Iosevka",
];

const terminalFontPresetByName = new Map(
  terminalFontPresets
    .filter((preset) => preset.id !== DEFAULT_TERMINAL_FONT_PRESET_ID && preset.id !== CUSTOM_TERMINAL_FONT_PRESET_ID)
    .map((preset) => [preset.name, preset])
);

// Full curated list, including names that don't map to a hardcoded preset. When
// both queryLocalFonts() and the Wails backend are unavailable we still want the
// user to be able to pick a Nerd Font / mono family — selecting an unknown name
// just stores it as fontFamily and the CSS layer skips it if not actually
// installed (the fallback chain takes over).
const staticTerminalFontOptions: TerminalFontOption[] =
  RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES.map(terminalFontOptionForFamily);

function terminalFontOptionForFamily(name: string): TerminalFontOption {
  const preset = terminalFontPresetByName.get(name);
  return {
    value: preset?.id ?? name,
    name,
  };
}

export function buildTerminalFontGroups(installedFonts: string[] | null | undefined): {
  recommendedFonts: TerminalFontOption[];
  otherFonts: TerminalFontOption[];
} {
  if (installedFonts === undefined) return { recommendedFonts: [], otherFonts: [] };

  // Local Font Access is Chromium-only and may be unavailable in desktop
  // webviews. Keep the settings useful by falling back to the curated list.
  if (installedFonts === null) return { recommendedFonts: staticTerminalFontOptions, otherFonts: [] };

  const installedFontNames = Array.from(new Set(installedFonts.filter(Boolean))).sort((a, b) => a.localeCompare(b));
  const installed = new Set(installedFontNames);
  const recommendedSet = new Set(RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES);
  const recommendedFonts = RECOMMENDED_TERMINAL_FONT_FAMILY_NAMES.filter((name) => installed.has(name)).map(
    terminalFontOptionForFamily
  );
  const otherFonts = installedFontNames.filter((name) => !recommendedSet.has(name)).map(terminalFontOptionForFamily);
  return { recommendedFonts, otherFonts };
}

// Returns the option to render for `fontPresetId` when it is otherwise absent
// from the rendered groups — e.g. a preset id like "fira-code" saved before the
// system-font picker existed, or a family name that used to be installed but is
// gone now. Returning the orphan keeps the Select trigger from showing a blank
// value. Returns null while installedFonts is still loading and for the
// default/custom sentinels (which are always rendered separately).
export function resolveFontPresetOrphan(
  fontPresetId: string,
  renderedFonts: TerminalFontOption[],
  installedFonts: string[] | null | undefined
): TerminalFontOption | null {
  if (fontPresetId === CUSTOM_TERMINAL_FONT_PRESET_ID || fontPresetId === DEFAULT_TERMINAL_FONT_PRESET_ID) {
    return null;
  }
  if (installedFonts === undefined) return null;
  if (renderedFonts.some((opt) => opt.value === fontPresetId || opt.name === fontPresetId)) return null;
  const preset = terminalFontPresets.find((p) => p.id === fontPresetId);
  if (preset) return { value: preset.id, name: preset.name };
  return { value: fontPresetId, name: fontPresetId };
}

// Quote a family name for use in CSS font-family. Pure identifiers can be
// passed unquoted; anything with spaces, dots, or apostrophes is single-quoted.
// Control characters are stripped, and backslashes must be escaped before
// apostrophes — otherwise a trailing `\` would escape the closing quote and
// break the declaration.
export function quoteFamilyName(name: string): string {
  name = cleanFamilyName(name);
  if (/^[a-zA-Z][a-zA-Z0-9_-]*$/.test(name)) return name;
  return `'${name.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;
}

interface QueryLocalFontsApi {
  queryLocalFonts?: () => Promise<Array<{ family: string }>>;
}

interface WailsSystemFontsApi {
  go?: {
    app?: {
      App?: {
        ListSystemFonts?: () => Promise<string[]>;
      };
    };
  };
}

// Returns the first family in the default fallback chain that is installed on
// this system — i.e. the family the browser will actually use for the bulk of
// terminal glyphs when fontFamily === DEFAULT_TERMINAL_FONT_FAMILY. Returns
// null when no chain entry is installed (browser will use generic monospace).
export function resolveDefaultFontPrimary(installedFonts: string[] | null): string | null {
  if (!installedFonts || installedFonts.length === 0) return null;
  const installed = new Set(installedFonts);
  for (const entry of DEFAULT_TERMINAL_FONT_FALLBACKS) {
    const name = entry.replace(/^['"]|['"]$/g, "");
    if (name === "monospace") return null;
    if (installed.has(name)) return name;
  }
  return null;
}

function normalizeInstalledFontFamilies(fonts: string[]): string[] {
  const families = new Set<string>();
  for (const font of fonts) {
    const family = cleanFamilyName(font);
    if (family) families.add(family);
  }
  return Array.from(families).sort((a, b) => a.localeCompare(b));
}

function cleanFamilyName(name: string): string {
  // eslint-disable-next-line no-control-regex
  return name.replace(/[\u0000-\u001f\u007f]/g, "").trim();
}

async function loadInstalledFontsFromBrowser(): Promise<string[] | null> {
  const api = window as unknown as QueryLocalFontsApi;
  if (typeof api.queryLocalFonts !== "function") return null;
  try {
    const fonts = await api.queryLocalFonts();
    if (!Array.isArray(fonts)) return null;
    return normalizeInstalledFontFamilies(fonts.map((font) => font.family));
  } catch {
    return null;
  }
}

async function loadInstalledFontsFromWails(): Promise<string[] | null> {
  const api = window as unknown as WailsSystemFontsApi;
  const listSystemFonts = api.go?.app?.App?.ListSystemFonts;
  if (typeof listSystemFonts !== "function") return null;
  try {
    const fonts = await listSystemFonts();
    if (!Array.isArray(fonts)) return null;
    return normalizeInstalledFontFamilies(fonts);
  } catch {
    return null;
  }
}

// Returns deduplicated installed font family names, sorted alphabetically. In a
// Chromium browser it uses Local Font Access; in Wails desktop webviews it
// falls back to the Go backend. Resolves to null when both sources are
// unavailable or denied, and callers should fall back to the curated list.
export async function loadInstalledFonts(): Promise<string[] | null> {
  if (typeof window === "undefined") return null;
  return (await loadInstalledFontsFromBrowser()) ?? (await loadInstalledFontsFromWails());
}

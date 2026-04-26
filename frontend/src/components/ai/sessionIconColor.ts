const LEADING_PUNCT_RE = /^[!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~]+/u;
const ASCII_LETTER_RE = /^[a-zA-Z]$/;

export function getSessionIconLetter(title: string): string {
  const trimmed = title.trim().replace(LEADING_PUNCT_RE, "");
  if (!trimmed) return "?";
  // Array.from correctly handles surrogate pairs (emoji), taking the true first code point
  const first = Array.from(trimmed)[0];
  if (!first) return "?";
  if (ASCII_LETTER_RE.test(first)) return first.toUpperCase();
  return first;
}

export interface SessionIconColor {
  bg: string;
  fg: string;
}

// 8 色固定调色板，OKLCH 写法 —— Wails 内嵌 Webkit 已支持。
// fg 统一白色（与 ~0.55-0.68 亮度的 bg 对比足够），简化心智负担。
const PALETTE: SessionIconColor[] = [
  { bg: "oklch(0.55 0.18 264)", fg: "#ffffff" }, // indigo
  { bg: "oklch(0.62 0.18 28)", fg: "#ffffff" }, // red-orange
  { bg: "oklch(0.62 0.16 145)", fg: "#ffffff" }, // emerald
  { bg: "oklch(0.65 0.18 65)", fg: "#1a1a1a" }, // amber (深字配浅底)
  { bg: "oklch(0.58 0.20 305)", fg: "#ffffff" }, // violet
  { bg: "oklch(0.62 0.15 215)", fg: "#ffffff" }, // sky
  { bg: "oklch(0.60 0.18 350)", fg: "#ffffff" }, // pink
  { bg: "oklch(0.55 0.16 95)", fg: "#ffffff" }, // olive
];

function hash(s: string): number {
  // djb2 变体，纯函数稳定
  let h = 5381;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) + h + s.charCodeAt(i)) | 0;
  }
  return h >>> 0;
}

export function getSessionIconColor(title: string): SessionIconColor {
  const idx = hash(title) % PALETTE.length;
  return PALETTE[idx];
}

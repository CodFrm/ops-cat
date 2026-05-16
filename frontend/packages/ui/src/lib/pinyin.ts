// pinyin-pro 完整字典 535KB，首屏不必加载。模块加载时异步预热；加载完成前
// pinyinMatch 退回纯文本匹配——用户首屏几百毫秒内一般不会立刻输入搜索。
type PinyinFn = typeof import("pinyin-pro").pinyin;
let pinyinFn: PinyinFn | null = null;
let pinyinLoading: Promise<void> | null = null;

function ensurePinyinLoaded(): Promise<void> {
  if (pinyinFn) return Promise.resolve();
  if (pinyinLoading) return pinyinLoading;
  pinyinLoading = import("pinyin-pro").then((m) => {
    pinyinFn = m.pinyin;
  });
  return pinyinLoading;
}

void ensurePinyinLoaded();

/** 测试或显式预热时使用。 */
export const __ensurePinyinReady = ensurePinyinLoaded;

/**
 * Check if `text` matches `query` by original text, full pinyin, initials, or mixed pinyin.
 * E.g. "中转站" matches "zzz", "zhongzhuanzhan", "zhongzz", "中转".
 */
export function pinyinMatch(text: string, query: string): boolean {
  if (!query) return true;
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase();

  // 1. Original text match
  if (lowerText.includes(lowerQuery)) return true;

  if (!pinyinFn) return false;

  // 2. Get pinyin arrays for each character
  const pinyinArr = pinyinFn(text, { toneType: "none", type: "array" });
  const firstArr = pinyinFn(text, {
    pattern: "first",
    toneType: "none",
    type: "array",
  });

  // 3. Initials-only match (e.g. "zzz" matches "中转站")
  const initials = firstArr.join("").toLowerCase();
  if (initials.includes(lowerQuery)) return true;

  // 4. Full pinyin match (e.g. "zhongzhuanzhan" matches "中转站")
  const fullPinyin = pinyinArr.join("").toLowerCase();
  if (fullPinyin.includes(lowerQuery)) return true;

  // 5. Mixed match: query consumed char-by-char against each character's pinyin
  //    e.g. "zhongzz" matches "中转站" (zhong=中, z=转, z=站)
  if (mixedPinyinMatch(pinyinArr, firstArr, lowerQuery)) return true;

  return false;
}

/**
 * Try to match query against pinyin arrays starting from each position in text.
 * For each starting position, attempt to consume the entire query.
 */
function mixedPinyinMatch(pinyinArr: string[], firstArr: string[], query: string): boolean {
  for (let start = 0; start < pinyinArr.length; start++) {
    if (tryConsume(pinyinArr, firstArr, start, query, 0)) return true;
  }
  return false;
}

/**
 * Recursively consume query characters against pinyin of text characters.
 * At each text character position, try matching:
 *   - Full pinyin prefix (greedy, longest first)
 *   - Initial letter only
 */
function tryConsume(
  pinyinArr: string[],
  firstArr: string[],
  charIdx: number,
  query: string,
  queryIdx: number
): boolean {
  if (queryIdx >= query.length) return true;
  if (charIdx >= pinyinArr.length) return false;

  const py = pinyinArr[charIdx].toLowerCase();
  const initial = firstArr[charIdx].toLowerCase();
  const remaining = query.substring(queryIdx);

  // Try matching full pinyin prefix (longest match first for greedy)
  for (let len = Math.min(py.length, remaining.length); len >= 1; len--) {
    if (py.startsWith(remaining.substring(0, len))) {
      if (tryConsume(pinyinArr, firstArr, charIdx + 1, query, queryIdx + len)) {
        return true;
      }
    }
  }

  // Try matching just the initial
  if (remaining[0] === initial) {
    if (tryConsume(pinyinArr, firstArr, charIdx + 1, query, queryIdx + 1)) {
      return true;
    }
  }

  return false;
}

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

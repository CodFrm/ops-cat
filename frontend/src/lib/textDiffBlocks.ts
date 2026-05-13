export type TextDiffBlockKind = "insert" | "delete" | "modify";

export interface TextDiffBlock {
  id: string;
  kind: TextDiffBlockKind;
  originalStartLine: number;
  originalEndLine: number;
  modifiedStartLine: number;
  modifiedEndLine: number;
}

function splitLines(value: string): string[] {
  const normalized = value.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  return normalized.length === 0 ? [] : normalized.split("\n");
}

export function buildTextDiffBlocks(original: string, modified: string): TextDiffBlock[] {
  const originalLines = splitLines(original);
  const modifiedLines = splitLines(modified);
  const rows = originalLines.length + 1;
  const cols = modifiedLines.length + 1;
  const lcs = Array.from({ length: rows }, () => Array<number>(cols).fill(0));

  for (let i = originalLines.length - 1; i >= 0; i -= 1) {
    for (let j = modifiedLines.length - 1; j >= 0; j -= 1) {
      lcs[i][j] =
        originalLines[i] === modifiedLines[j]
          ? lcs[i + 1][j + 1] + 1
          : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }

  const blocks: TextDiffBlock[] = [];
  let i = 0;
  let j = 0;

  const pushBlock = (originalStart: number, originalEnd: number, modifiedStart: number, modifiedEnd: number) => {
    const hasOriginal = originalEnd >= originalStart;
    const hasModified = modifiedEnd >= modifiedStart;
    if (!hasOriginal && !hasModified) return;
    const kind: TextDiffBlockKind = hasOriginal && hasModified ? "modify" : hasOriginal ? "delete" : "insert";
    blocks.push({
      id: `${kind}:${originalStart}-${originalEnd}:${modifiedStart}-${modifiedEnd}`,
      kind,
      originalStartLine: hasOriginal ? originalStart + 1 : originalStart,
      originalEndLine: hasOriginal ? originalEnd + 1 : originalStart,
      modifiedStartLine: hasModified ? modifiedStart + 1 : modifiedStart,
      modifiedEndLine: hasModified ? modifiedEnd + 1 : modifiedStart,
    });
  };

  while (i < originalLines.length || j < modifiedLines.length) {
    if (i < originalLines.length && j < modifiedLines.length && originalLines[i] === modifiedLines[j]) {
      i += 1;
      j += 1;
      continue;
    }

    const originalStart = i;
    const modifiedStart = j;
    while (i < originalLines.length || j < modifiedLines.length) {
      if (i < originalLines.length && j < modifiedLines.length && originalLines[i] === modifiedLines[j]) break;
      if (j >= modifiedLines.length || (i < originalLines.length && lcs[i + 1][j] >= lcs[i][j + 1])) {
        i += 1;
      } else {
        j += 1;
      }
    }
    pushBlock(originalStart, i - 1, modifiedStart, j - 1);
  }

  return blocks;
}

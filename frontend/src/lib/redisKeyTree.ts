export const DEFAULT_REDIS_KEY_SEPARATOR = ":";

export interface RedisTreeNode {
  name: string;
  fullKey: string | null;
  children: Map<string, RedisTreeNode>;
  keyCount: number;
}

export interface RedisFlatTreeRow {
  depth: number;
  name: string;
  fullKey: string | null;
  keyCount: number;
  isExpanded: boolean;
  hasChildren: boolean;
  nodeId: string;
}

export function buildKeyTree(keys: string[], separator: string): RedisTreeNode {
  const root: RedisTreeNode = { name: "", fullKey: null, children: new Map(), keyCount: 0 };
  const sep = separator || DEFAULT_REDIS_KEY_SEPARATOR;
  for (const key of keys) {
    const parts = key.split(sep);
    let node = root;
    for (let i = 0; i < parts.length; i++) {
      const segment = parts[i];
      const isLast = i === parts.length - 1;
      if (isLast) {
        const existing = node.children.get(segment);
        if (existing) {
          if (existing.fullKey === null) {
            existing.fullKey = key;
            existing.keyCount++;
          }
        } else {
          node.children.set(segment, { name: segment, fullKey: key, children: new Map(), keyCount: 1 });
        }
      } else {
        if (!node.children.has(segment)) {
          node.children.set(segment, { name: segment, fullKey: null, children: new Map(), keyCount: 0 });
        }
        node = node.children.get(segment)!;
      }
    }
    let n = root;
    for (let i = 0; i < parts.length - 1; i++) {
      if (n.children.has(parts[i])) {
        n = n.children.get(parts[i])!;
        n.keyCount++;
      }
    }
    root.keyCount++;
  }
  return root;
}

export function flattenTree(root: RedisTreeNode, expandedSet: Set<string>, separator: string): RedisFlatTreeRow[] {
  const result: RedisFlatTreeRow[] = [];
  const sep = separator || DEFAULT_REDIS_KEY_SEPARATOR;
  const walk = (node: RedisTreeNode, depth: number, prefix: string) => {
    const entries = Array.from(node.children.values()).sort((a, b) => {
      const aIsFolder = a.children.size > 0 ? 0 : 1;
      const bIsFolder = b.children.size > 0 ? 0 : 1;
      if (aIsFolder !== bIsFolder) return aIsFolder - bIsFolder;
      return a.name.localeCompare(b.name);
    });
    for (const child of entries) {
      const nodeId = prefix ? `${prefix}${sep}${child.name}` : child.name;
      const isExpanded = expandedSet.has(nodeId);
      result.push({
        depth,
        name: child.name,
        fullKey: child.fullKey,
        keyCount: child.keyCount,
        isExpanded,
        hasChildren: child.children.size > 0,
        nodeId,
      });
      if (child.children.size > 0 && isExpanded) {
        walk(child, depth + 1, nodeId);
      }
    }
  };
  walk(root, 0, "");
  return result;
}

export function isDefaultRedisKeyFilter(pattern: string) {
  const trimmed = pattern.trim();
  return trimmed === "" || trimmed === "*";
}

function escapeRegex(value: string) {
  return value.replace(/[|\\{}()[\]^$+*?.]/g, "\\$&");
}

export function makeLocalKeyMatcher(pattern: string): (key: string) => boolean {
  const trimmed = pattern.trim();
  if (isDefaultRedisKeyFilter(trimmed)) return () => true;
  if (/[*?]/.test(trimmed)) {
    const source = trimmed
      .split("")
      .map((char) => {
        if (char === "*") return ".*";
        if (char === "?") return ".";
        return escapeRegex(char);
      })
      .join("");
    const re = new RegExp(`^${source}$`, "i");
    return (key) => re.test(key);
  }
  const needle = trimmed.toLocaleLowerCase();
  return (key) => key.toLocaleLowerCase().includes(needle);
}

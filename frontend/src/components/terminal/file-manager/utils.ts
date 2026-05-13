import { sftp_svc } from "../../../../wailsjs/go/models";

export const HANDLE_PX = 4;

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + " " + units[i];
}

export function formatDate(timestamp: number): string {
  const d = new Date(timestamp * 1000);
  return d.toLocaleDateString(undefined, {
    month: "2-digit",
    day: "2-digit",
  });
}

export function normalizeRemotePath(basePath: string, nextPath: string): string {
  const raw = nextPath.trim();
  if (!raw) return basePath || "/";
  const combined = raw.startsWith("/") ? raw : `${basePath === "/" ? "" : basePath}/${raw}`;
  const parts = combined.split("/");
  const normalized: string[] = [];
  for (const part of parts) {
    if (!part || part === ".") continue;
    if (part === "..") {
      normalized.pop();
      continue;
    }
    normalized.push(part);
  }
  return "/" + normalized.join("/");
}

export function getEntryPath(currentPath: string, entry: sftp_svc.FileEntry): string {
  return currentPath === "/" ? "/" + entry.name : currentPath + "/" + entry.name;
}

export function getParentPath(currentPath: string): string {
  return currentPath.replace(/\/[^/]+\/?$/, "") || "/";
}

export function sortEntries(entries: sftp_svc.FileEntry[]): sftp_svc.FileEntry[] {
  return [...entries].sort((a, b) => {
    if (a.isDir && !b.isDir) return -1;
    if (!a.isDir && b.isDir) return 1;
    return a.name.localeCompare(b.name);
  });
}

// Go os.FileMode 类型标志位（与 io/fs 保持一致）
const MODE_DIR = 1 << 31;
const MODE_SYMLINK = 1 << 28;
const PERM_MASK = 0o777;

export type PermBits = { r: boolean; w: boolean; x: boolean };
export type Perms = { u: PermBits; g: PermBits; o: PermBits };

// formatMode 把 Go FileMode 渲染成 "drwxr-xr-x" 风格
export function formatMode(mode: number, isDir: boolean): string {
  if (mode === 0 && !isDir) return "----------";
  let prefix = "-";
  if (mode & MODE_DIR || isDir) prefix = "d";
  else if (mode & MODE_SYMLINK) prefix = "l";

  const perm = mode & PERM_MASK;
  const bit = (shift: number) => (perm >> shift) & 0b111;
  const triple = (v: number) => `${v & 4 ? "r" : "-"}${v & 2 ? "w" : "-"}${v & 1 ? "x" : "-"}`;
  return prefix + triple(bit(6)) + triple(bit(3)) + triple(bit(0));
}

export function modeToOctal(mode: number): string {
  return (mode & PERM_MASK).toString(8).padStart(3, "0");
}

export function modeToPerms(mode: number): Perms {
  const perm = mode & PERM_MASK;
  const fromBits = (v: number): PermBits => ({ r: !!(v & 4), w: !!(v & 2), x: !!(v & 1) });
  return {
    u: fromBits((perm >> 6) & 0b111),
    g: fromBits((perm >> 3) & 0b111),
    o: fromBits(perm & 0b111),
  };
}

export function permsToMode(perms: Perms): number {
  const toBits = (p: PermBits) => (p.r ? 4 : 0) | (p.w ? 2 : 0) | (p.x ? 1 : 0);
  return (toBits(perms.u) << 6) | (toBits(perms.g) << 3) | toBits(perms.o);
}

// parseOctal 接受 "0755" / "755" / 数字字符串，越界返回 null
export function parseOctal(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^[0-7]{1,4}$/.test(trimmed)) return null;
  const v = parseInt(trimmed, 8);
  if (v < 0 || v > 0o777) return null;
  return v;
}

// validateEntryName 校验文件/目录名；返回错误 key（i18n）或 null。
export function validateEntryName(name: string): "errNameEmpty" | "errNameInvalid" | null {
  const trimmed = name.trim();
  if (!trimmed) return "errNameEmpty";
  if (trimmed === "." || trimmed === "..") return "errNameInvalid";
  if (trimmed.includes("/")) return "errNameInvalid";
  return null;
}

export function joinRemotePath(basePath: string, name: string): string {
  return basePath === "/" ? "/" + name : basePath + "/" + name;
}

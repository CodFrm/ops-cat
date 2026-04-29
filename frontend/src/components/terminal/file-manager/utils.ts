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

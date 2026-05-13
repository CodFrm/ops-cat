import { sftp_svc } from "../../../../wailsjs/go/models";

export interface CtxMenuState {
  x: number;
  y: number;
  entry: sftp_svc.FileEntry | null;
}

export interface DeleteTarget {
  path: string;
  name: string;
  isDir: boolean;
}

export type EditingState = { mode: "create-file" | "create-dir" } | { mode: "rename"; targetName: string };

export interface PermissionTarget {
  path: string;
  name: string;
  mode: number;
  isDir: boolean;
}

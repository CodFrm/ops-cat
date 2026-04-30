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

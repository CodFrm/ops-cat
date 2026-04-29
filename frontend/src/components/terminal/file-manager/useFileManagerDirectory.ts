import { useCallback, useEffect, useRef, useState } from "react";
import { SFTPListDir } from "../../../../wailsjs/go/app/App";
import { sftp_svc } from "../../../../wailsjs/go/models";
import { useSFTPStore } from "@/stores/sftpStore";
import { normalizeRemotePath } from "./utils";

export function useFileManagerDirectory(tabId: string, sessionId: string) {
  const storedPath = useSFTPStore((s) => s.fileManagerPaths[tabId]);
  const currentPath = storedPath || "/";
  const setCurrentPath = useSFTPStore((s) => s.setFileManagerPath);
  const [pathInput, setPathInput] = useState(currentPath);
  const [entries, setEntries] = useState<sftp_svc.FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const loadRequestRef = useRef(0);
  const currentPathRef = useRef(currentPath);
  currentPathRef.current = currentPath;

  const loadDir = useCallback(
    async (dirPath: string) => {
      const requestId = ++loadRequestRef.current;
      const normalizedPath = normalizeRemotePath(currentPathRef.current, dirPath);
      setLoading(true);
      setError(null);
      setSelected(null);
      try {
        const result = await SFTPListDir(sessionId, normalizedPath);
        if (requestId !== loadRequestRef.current) return false;
        setEntries(result || []);
        setCurrentPath(tabId, normalizedPath);
        setPathInput(normalizedPath);
        return true;
      } catch (e) {
        if (requestId !== loadRequestRef.current) return false;
        setError(String(e));
        return false;
      } finally {
        if (requestId === loadRequestRef.current) {
          setLoading(false);
        }
      }
    },
    [sessionId, setCurrentPath, tabId]
  );

  useEffect(() => {
    setPathInput(currentPath);
  }, [currentPath]);

  return {
    currentPath,
    currentPathRef,
    entries,
    error,
    loading,
    loadDir,
    pathInput,
    selected,
    setError,
    setPathInput,
    setSelected,
    storedPath,
  };
}

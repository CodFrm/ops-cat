import { describe, it, expect, beforeEach, vi } from "vitest";
import { useSFTPStore, type SFTPTransfer } from "../stores/sftpStore";
import { SFTPUpload, SFTPDownload, SFTPCancelTransfer } from "../../wailsjs/go/app/App";

function makeTransfer(
  id: string,
  sessionId: string,
  status: SFTPTransfer["status"] = "active",
  tabId = `tab-${sessionId}`
): SFTPTransfer {
  return {
    transferId: id,
    tabId,
    sessionId,
    direction: "upload",
    currentFile: "test.txt",
    filesCompleted: 0,
    filesTotal: 1,
    bytesDone: 0,
    bytesTotal: 100,
    speed: 0,
    status,
  };
}

describe("sftpStore", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSFTPStore.setState({
      transfers: {},
      fileManagerOpenTabs: {},
      fileManagerPaths: {},
      fileManagerWidth: 280,
    });
  });

  describe("clearTransfer", () => {
    it("removes a specific transfer", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1"),
          t2: makeTransfer("t2", "s1"),
        },
      });

      useSFTPStore.getState().clearTransfer("t1");

      const transfers = useSFTPStore.getState().transfers;
      expect(transfers).not.toHaveProperty("t1");
      expect(transfers).toHaveProperty("t2");
    });
  });

  describe("clearCompleted", () => {
    it("removes all non-active transfers", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1", "active"),
          t2: makeTransfer("t2", "s1", "done"),
          t3: makeTransfer("t3", "s1", "error"),
        },
      });

      useSFTPStore.getState().clearCompleted();

      const transfers = useSFTPStore.getState().transfers;
      expect(Object.keys(transfers)).toEqual(["t1"]);
    });

    it("keeps all active transfers", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1", "active"),
          t2: makeTransfer("t2", "s2", "active"),
        },
      });

      useSFTPStore.getState().clearCompleted();
      expect(Object.keys(useSFTPStore.getState().transfers)).toHaveLength(2);
    });
  });

  describe("clearCompletedForSession", () => {
    it("removes non-active transfers for a specific session", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1", "done"),
          t2: makeTransfer("t2", "s1", "active"),
          t3: makeTransfer("t3", "s2", "done"),
        },
      });

      useSFTPStore.getState().clearCompletedForSession("s1");

      const transfers = useSFTPStore.getState().transfers;
      expect(transfers).not.toHaveProperty("t1");
      expect(transfers).toHaveProperty("t2");
      expect(transfers).toHaveProperty("t3");
    });
  });

  describe("clearCompletedForTab", () => {
    it("removes non-active transfers for a specific tab only", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1", "done", "tab1"),
          t2: makeTransfer("t2", "s1", "active", "tab1"),
          t3: makeTransfer("t3", "s2", "done", "tab2"),
        },
      });

      useSFTPStore.getState().clearCompletedForTab("tab1");

      const transfers = useSFTPStore.getState().transfers;
      expect(transfers).not.toHaveProperty("t1");
      expect(transfers).toHaveProperty("t2");
      expect(transfers).toHaveProperty("t3");
    });
  });

  describe("getSessionTransfers", () => {
    it("returns only transfers for the given session", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1"),
          t2: makeTransfer("t2", "s2"),
          t3: makeTransfer("t3", "s1"),
        },
      });

      const result = useSFTPStore.getState().getSessionTransfers("s1");
      expect(result).toHaveLength(2);
      expect(result.map((t) => t.transferId).sort()).toEqual(["t1", "t3"]);
    });

    it("returns empty array when no transfers for session", () => {
      const result = useSFTPStore.getState().getSessionTransfers("unknown");
      expect(result).toEqual([]);
    });
  });

  describe("getTabTransfers", () => {
    it("returns only transfers for the given tab", () => {
      useSFTPStore.setState({
        transfers: {
          t1: makeTransfer("t1", "s1", "active", "tab1"),
          t2: makeTransfer("t2", "s2", "active", "tab2"),
          t3: makeTransfer("t3", "s3", "active", "tab1"),
        },
      });

      const result = useSFTPStore.getState().getTabTransfers("tab1");
      expect(result).toHaveLength(2);
      expect(result.map((t) => t.transferId).sort()).toEqual(["t1", "t3"]);
    });
  });

  describe("toggleFileManager", () => {
    it("toggles file manager open state for a tab", () => {
      useSFTPStore.getState().toggleFileManager("tab1");
      expect(useSFTPStore.getState().fileManagerOpenTabs["tab1"]).toBe(true);

      useSFTPStore.getState().toggleFileManager("tab1");
      expect(useSFTPStore.getState().fileManagerOpenTabs["tab1"]).toBe(false);
    });
  });

  describe("setFileManagerPath", () => {
    it("stores current file manager path per tab", () => {
      useSFTPStore.getState().setFileManagerPath("tab1", "/var/log");
      useSFTPStore.getState().setFileManagerPath("tab2", "/srv/app");

      expect(useSFTPStore.getState().fileManagerPaths["tab1"]).toBe("/var/log");
      expect(useSFTPStore.getState().fileManagerPaths["tab2"]).toBe("/srv/app");
    });
  });

  describe("setFileManagerWidth", () => {
    it("sets width within bounds", () => {
      useSFTPStore.getState().setFileManagerWidth(400);
      expect(useSFTPStore.getState().fileManagerWidth).toBe(400);
    });

    it("clamps to minimum 200", () => {
      useSFTPStore.getState().setFileManagerWidth(50);
      expect(useSFTPStore.getState().fileManagerWidth).toBe(200);
    });

    it("clamps to maximum 600", () => {
      useSFTPStore.getState().setFileManagerWidth(1000);
      expect(useSFTPStore.getState().fileManagerWidth).toBe(600);
    });
  });

  describe("cancelTransfer", () => {
    it("calls backend SFTPCancelTransfer", () => {
      useSFTPStore.getState().cancelTransfer("t1");
      expect(SFTPCancelTransfer).toHaveBeenCalledWith("t1");
    });
  });

  describe("startUpload", () => {
    it("calls backend and initializes transfer on success", async () => {
      vi.mocked(SFTPUpload).mockResolvedValue("transfer-123");

      const result = await useSFTPStore.getState().startUpload({ tabId: "tab1", sessionId: "s1" }, "/remote/path");
      expect(result).toBe("transfer-123");
      expect(SFTPUpload).toHaveBeenCalledWith("s1", "/remote/path");
      expect(useSFTPStore.getState().transfers["transfer-123"]).toBeDefined();
      expect(useSFTPStore.getState().transfers["transfer-123"].tabId).toBe("tab1");
      expect(useSFTPStore.getState().transfers["transfer-123"].sessionId).toBe("s1");
      expect(useSFTPStore.getState().transfers["transfer-123"].status).toBe("active");
      expect(useSFTPStore.getState().transfers["transfer-123"].direction).toBe("upload");
    });

    it("returns null when backend returns empty", async () => {
      vi.mocked(SFTPUpload).mockResolvedValue("");

      const result = await useSFTPStore.getState().startUpload({ tabId: "tab1", sessionId: "s1" }, "/remote/path");
      expect(result).toBeNull();
    });

    it("keeps transfers scoped to their initiating tabs", async () => {
      vi.mocked(SFTPUpload).mockResolvedValueOnce("transfer-1").mockResolvedValueOnce("transfer-2");

      await useSFTPStore.getState().startUpload({ tabId: "tab1", sessionId: "s1" }, "/remote/file1");
      await useSFTPStore.getState().startUpload({ tabId: "tab2", sessionId: "s2" }, "/remote/file2");

      expect(
        useSFTPStore
          .getState()
          .getTabTransfers("tab1")
          .map((transfer) => transfer.transferId)
      ).toEqual(["transfer-1"]);
      expect(
        useSFTPStore
          .getState()
          .getTabTransfers("tab2")
          .map((transfer) => transfer.transferId)
      ).toEqual(["transfer-2"]);
      expect(SFTPUpload).toHaveBeenNthCalledWith(1, "s1", "/remote/file1");
      expect(SFTPUpload).toHaveBeenNthCalledWith(2, "s2", "/remote/file2");
    });
  });

  describe("startDownload", () => {
    it("calls backend and initializes download transfer", async () => {
      vi.mocked(SFTPDownload).mockResolvedValue("dl-123");

      const result = await useSFTPStore
        .getState()
        .startDownload({ tabId: "tab1", sessionId: "s1" }, "/remote/file.txt");
      expect(result).toBe("dl-123");
      expect(useSFTPStore.getState().transfers["dl-123"].tabId).toBe("tab1");
      expect(useSFTPStore.getState().transfers["dl-123"].direction).toBe("download");
    });
  });
});

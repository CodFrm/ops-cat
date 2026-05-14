import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  buildExternalEditErrors,
  buildExternalEditAttentionItems,
  buildExternalEditConflicts,
  buildExternalEditDocuments,
  buildExternalEditRecoveries,
  useExternalEditStore,
} from "../stores/externalEditStore";
import type { ExternalEditSession } from "../lib/externalEditApi";

function makeSession(partial: Partial<ExternalEditSession> & { id: string }): ExternalEditSession {
  return {
    assetId: 101,
    assetName: "asset-101",
    documentKey: "101:/srv/app/demo.txt",
    sessionId: "ssh-b",
    remotePath: "/srv/app/demo.txt",
    remoteRealPath: "/srv/app/demo.txt",
    localPath: `/tmp/${partial.id}.txt`,
    workspaceRoot: "/tmp",
    workspaceDir: `/tmp/${partial.id}`,
    editorId: "system-text",
    editorName: "System Text Editor",
    editorPath: "/bin/editor",
    originalSha256: "a",
    originalSize: 1,
    originalModTime: 1,
    originalEncoding: "utf-8",
    lastLocalSha256: "b",
    dirty: true,
    state: "dirty",
    hidden: false,
    expired: false,
    createdAt: 1,
    updatedAt: 10,
    lastLaunchedAt: 10,
    lastSyncedAt: 1,
    ...partial,
    id: partial.id,
  };
}

function makeClipboardResidueSession(
  partial: Partial<ExternalEditSession> & { id?: string } = {}
): ExternalEditSession {
  return makeSession({
    id: "clipboard",
    documentKey: "101:/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
    remotePath: "/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
    remoteRealPath: "/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
    localPath:
      "C:\\Users\\asus\\AppData\\Local\\com.golutra\\clipboard-images\\clipboard-6607ba08467079f385199f18c460e71b33008a531841e07a90f9b4b613629f88.png",
    workspaceDir: "C:\\Users\\asus\\AppData\\Local\\com.golutra\\clipboard-images",
    state: "conflict",
    recordState: "error",
    resumeRequired: true,
    lastError: { step: "write_remote_file", summary: "failed", suggestion: "retry", at: 1 },
    updatedAt: 50,
    ...partial,
  });
}

beforeEach(() => {
  useExternalEditStore.setState({
    sessions: {},
    loading: false,
    savingSessionId: null,
    autoSavePhases: {},
    pendingConflict: null,
    compareResult: null,
    mergeResult: null,
    selectedError: null,
  });
  vi.stubGlobal("window", {
    ...window,
    go: {
      app: {
        App: {},
      },
    },
  });
});

describe("buildExternalEditDocuments", () => {
  it("merges sessions that point to the same logical document", () => {
    const documents = buildExternalEditDocuments({
      old: {
        id: "old",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-b",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "dirty",
        hidden: false,
        expired: false,
        createdAt: 1,
        updatedAt: 10,
        lastLaunchedAt: 10,
        lastSyncedAt: 1,
      },
      newer: {
        id: "newer",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-c",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "dirty",
        hidden: false,
        expired: false,
        createdAt: 1,
        updatedAt: 20,
        lastLaunchedAt: 20,
        lastSyncedAt: 1,
      },
    });

    expect(documents).toHaveLength(1);
    expect(documents[0]?.session.id).toBe("newer");
    expect(documents[0]?.documentKey).toBe("101:/srv/app/demo.txt");
  });

  it("prefers an actionable draft over a stale copy", () => {
    const documents = buildExternalEditDocuments({
      stale: {
        id: "stale",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-b",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "stale",
        hidden: false,
        expired: false,
        createdAt: 1,
        updatedAt: 30,
        lastLaunchedAt: 30,
        lastSyncedAt: 1,
      },
      draft: {
        id: "draft",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-c",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "conflict",
        hidden: false,
        expired: false,
        createdAt: 1,
        updatedAt: 20,
        lastLaunchedAt: 20,
        lastSyncedAt: 1,
      },
    });

    expect(documents).toHaveLength(1);
    expect(documents[0]?.session.id).toBe("draft");
  });
});

describe("buildExternalEditConflicts", () => {
  it("links the original draft with the latest reread snapshot", () => {
    const conflicts = buildExternalEditConflicts({
      draft: {
        id: "draft",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-b",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "conflict",
        hidden: false,
        expired: false,
        createdAt: 1,
        updatedAt: 10,
        lastLaunchedAt: 10,
        lastSyncedAt: 1,
      },
      snapshot: {
        id: "snapshot",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-c",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo-new.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo-new",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "c",
        originalSize: 1,
        originalModTime: 2,
        originalEncoding: "utf-8",
        lastLocalSha256: "c",
        dirty: false,
        state: "clean",
        hidden: false,
        expired: false,
        sourceSessionId: "draft",
        createdAt: 2,
        updatedAt: 20,
        lastLaunchedAt: 20,
        lastSyncedAt: 20,
      },
      stale: {
        id: "stale",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-b",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo-old.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo-old",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "stale",
        hidden: false,
        expired: false,
        supersededBySessionId: "snapshot",
        createdAt: 1,
        updatedAt: 30,
        lastLaunchedAt: 30,
        lastSyncedAt: 1,
      },
    });

    expect(conflicts).toHaveLength(1);
    expect(conflicts[0]?.primaryDraft.id).toBe("draft");
    expect(conflicts[0]?.retainedDraft?.id).toBe("stale");
    expect(conflicts[0]?.activeDraft?.id).toBe("snapshot");
    expect(conflicts[0]?.latestSnapshot?.id).toBe("snapshot");
  });

  it("keeps retained-only reread documents out of the conflict view", () => {
    const conflicts = buildExternalEditConflicts({
      retained: {
        id: "retained",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-b",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo-old.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo-old",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "a",
        originalSize: 1,
        originalModTime: 1,
        originalEncoding: "utf-8",
        lastLocalSha256: "b",
        dirty: true,
        state: "stale",
        hidden: false,
        expired: false,
        supersededBySessionId: "active",
        createdAt: 1,
        updatedAt: 30,
        lastLaunchedAt: 30,
        lastSyncedAt: 1,
      },
      active: {
        id: "active",
        assetId: 101,
        assetName: "asset-101",
        documentKey: "101:/srv/app/demo.txt",
        sessionId: "ssh-c",
        remotePath: "/srv/app/demo.txt",
        remoteRealPath: "/srv/app/demo.txt",
        localPath: "/tmp/demo-new.txt",
        workspaceRoot: "/tmp",
        workspaceDir: "/tmp/demo-new",
        editorId: "system-text",
        editorName: "System Text Editor",
        editorPath: "/bin/editor",
        originalSha256: "c",
        originalSize: 1,
        originalModTime: 2,
        originalEncoding: "utf-8",
        lastLocalSha256: "d",
        dirty: true,
        state: "dirty",
        hidden: false,
        expired: false,
        sourceSessionId: "retained",
        createdAt: 2,
        updatedAt: 40,
        lastLaunchedAt: 40,
        lastSyncedAt: 20,
      },
    });

    expect(conflicts).toHaveLength(0);
  });
});

describe("buildExternalEditAttentionItems", () => {
  it("keeps only the highest priority visible pending record per document family", () => {
    const sessions = {
      conflict: makeSession({ id: "conflict", state: "conflict", updatedAt: 30 }),
      error: makeSession({
        id: "error",
        state: "dirty",
        recordState: "error",
        lastError: { step: "write_remote_file", summary: "failed", suggestion: "retry", at: 1 },
        updatedAt: 20,
      }),
      recovery: makeSession({
        id: "recovery",
        state: "dirty",
        resumeRequired: true,
        saveMode: "manual_restored",
        updatedAt: 10,
      }),
    };

    expect(buildExternalEditRecoveries(sessions).map((entry) => entry.session.id)).toEqual([]);
    expect(
      buildExternalEditAttentionItems(sessions).map(
        (entry) => `${entry.type}:${entry.decisionType || "none"}:${entry.sourceType || "none"}:${entry.session.id}`
      )
    ).toEqual(["conflict:conflict:recovery:conflict"]);
  });

  it("projects recovery records into pending decision items and keeps remote-missing independent", () => {
    const recovery = makeSession({
      id: "recovery",
      state: "dirty",
      resumeRequired: true,
      saveMode: "manual_restored",
      updatedAt: 30,
    });
    const remoteMissing = makeSession({
      id: "missing",
      state: "remote_missing",
      recordState: "conflict",
      updatedAt: 20,
    });

    expect(
      buildExternalEditAttentionItems({ recovery }).map((entry) => ({
        type: entry.type,
        decisionType: entry.decisionType,
        sourceType: entry.sourceType,
        sessionId: entry.session.id,
      }))
    ).toEqual([
      {
        type: "pending",
        decisionType: "pending",
        sourceType: "recovery",
        sessionId: "recovery",
      },
    ]);
    expect(
      buildExternalEditAttentionItems({ remoteMissing }).map((entry) => ({
        type: entry.type,
        decisionType: entry.decisionType,
        sourceType: entry.sourceType,
        sessionId: entry.session.id,
      }))
    ).toEqual([
      {
        type: "remote_missing",
        decisionType: undefined,
        sourceType: undefined,
        sessionId: "missing",
      },
    ]);
  });

  it("does not turn retained stale plus active reread drafts into a conflict attention item", () => {
    const sessions = {
      retained: makeSession({
        id: "retained",
        state: "stale",
        supersededBySessionId: "active",
        updatedAt: 30,
      }),
      active: makeSession({
        id: "active",
        state: "clean",
        dirty: false,
        sourceSessionId: "retained",
        updatedAt: 40,
      }),
    };

    expect(buildExternalEditConflicts(sessions)).toHaveLength(0);
    expect(buildExternalEditAttentionItems(sessions)).toHaveLength(0);
  });

  it("filters clipboard residue from documents, conflicts, errors, recoveries, and attention items", () => {
    const clipboard = makeClipboardResidueSession();
    const valid = makeSession({ id: "valid", documentKey: "101:/srv/app/demo.txt", state: "dirty", updatedAt: 10 });
    const sessions = { clipboard, valid };

    expect(buildExternalEditDocuments(sessions).map((entry) => entry.session.id)).toEqual(["valid"]);
    expect(buildExternalEditConflicts(sessions)).toHaveLength(0);
    expect(buildExternalEditErrors(sessions)).toHaveLength(0);
    expect(buildExternalEditRecoveries(sessions)).toHaveLength(0);
    expect(buildExternalEditAttentionItems(sessions)).toHaveLength(0);
  });
});

describe("external edit clipboard residue runtime state", () => {
  it("promotes manual refresh conflict results into the pending conflict entry", async () => {
    const refreshed = makeSession({
      id: "reread-active",
      documentKey: "101:/srv/app/ee68_c_conflict.txt",
      remotePath: "/srv/app/ee68_c_conflict.txt",
      remoteRealPath: "/srv/app/ee68_c_conflict.txt",
      state: "conflict",
      recordState: "conflict",
      dirty: true,
      resumeRequired: true,
      sourceSessionId: "stale-original",
    });
    Object.assign(window.go!.app!.App!, {
      RefreshExternalEditSession: vi.fn().mockResolvedValue(refreshed),
    });

    const session = await useExternalEditStore.getState().refreshSession(refreshed.id);

    const state = useExternalEditStore.getState();
    expect(session).toBe(refreshed);
    expect(state.sessions[refreshed.id]).toEqual(refreshed);
    expect(state.pendingConflict?.status).toBe("conflict_remote_changed");
    expect(state.pendingConflict?.session?.id).toBe(refreshed.id);
    expect(state.pendingConflict?.conflict?.primaryDraftSessionId).toBe(refreshed.id);
  });

  it("reuses the existing recovery capability for continue-edit without a recovery detail shell", async () => {
    const recovery = makeSession({
      id: "recovery",
      documentKey: "101:/srv/app/recovery.txt",
      remotePath: "/srv/app/recovery.txt",
      remoteRealPath: "/srv/app/recovery.txt",
      state: "dirty",
      resumeRequired: true,
      updatedAt: 20,
    });
    const resumed = { ...recovery, resumeRequired: false, updatedAt: 30 };
    Object.assign(window.go!.app!.App!, {
      RecoverExternalEditSession: vi.fn().mockResolvedValue(resumed),
    });
    useExternalEditStore.setState({
      sessions: { [recovery.id]: recovery },
    });

    const result = await useExternalEditStore.getState().continuePendingSession(recovery.id, "recovery");
    const state = useExternalEditStore.getState();

    expect(result).toEqual(resumed);
    expect(state.sessions[recovery.id]).toEqual(resumed);
    expect(state.pendingConflict).toBeNull();
    expect(state.selectedError).toBeNull();
  });

  it("projects runtime pending-review sessions into the unified pending attention items", async () => {
    const runtimePending = makeSession({
      id: "runtime-pending",
      documentKey: "101:/srv/app/runtime-pending.txt",
      remotePath: "/srv/app/runtime-pending.txt",
      remoteRealPath: "/srv/app/runtime-pending.txt",
      state: "dirty",
      recordState: "active",
      saveMode: "auto_live",
      pendingReview: true,
      resumeRequired: false,
      updatedAt: 30,
    });

    expect(
      buildExternalEditAttentionItems({ [runtimePending.id]: runtimePending }).map((entry) => ({
        type: entry.type,
        decisionType: entry.decisionType,
        sourceType: entry.sourceType,
        sessionId: entry.session.id,
      }))
    ).toEqual([
      {
        type: "pending",
        decisionType: "pending",
        sourceType: "runtime",
        sessionId: "runtime-pending",
      },
    ]);
  });

  it("continues runtime pending-review sessions through the real Wails contract", async () => {
    const runtimePending = makeSession({
      id: "runtime-pending",
      documentKey: "101:/srv/app/runtime-pending.txt",
      remotePath: "/srv/app/runtime-pending.txt",
      remoteRealPath: "/srv/app/runtime-pending.txt",
      state: "dirty",
      recordState: "active",
      saveMode: "auto_live",
      pendingReview: true,
      resumeRequired: false,
      updatedAt: 20,
    });
    const continued = { ...runtimePending, pendingReview: false, updatedAt: 30 };
    Object.assign(window.go!.app!.App!, {
      ContinueExternalEditSession: vi.fn().mockResolvedValue(continued),
    });
    useExternalEditStore.setState({
      sessions: { [runtimePending.id]: runtimePending },
    });

    const result = await useExternalEditStore.getState().continuePendingSession(runtimePending.id, "runtime");
    const state = useExternalEditStore.getState();

    expect(result).toEqual(continued);
    expect(state.sessions[runtimePending.id]).toEqual(continued);
    expect(state.pendingConflict).toBeNull();
  });


  it("scrubs clipboard residue from pending dialogs, modal state, and event paths", () => {
    const clipboard = makeClipboardResidueSession();
    const valid = makeSession({ id: "valid", documentKey: "101:/srv/app/demo.txt", state: "dirty", updatedAt: 10 });

    useExternalEditStore.setState({
      sessions: {
        clipboard,
        valid,
      },
      autoSavePhases: {
        [clipboard.documentKey]: "running",
        [valid.documentKey]: "pending",
      },
      pendingConflict: {
        status: "conflict_remote_changed",
        session: clipboard,
        conflict: {
          documentKey: clipboard.documentKey,
          primaryDraftSessionId: clipboard.id,
        },
      },
      compareResult: {
        documentKey: clipboard.documentKey,
        primaryDraftSessionId: clipboard.id,
        fileName: "clipboard.png",
        remotePath: clipboard.remotePath,
        localContent: "local",
        remoteContent: "remote",
        readOnly: true,
        session: clipboard,
      },
      mergeResult: {
        documentKey: clipboard.documentKey,
        primaryDraftSessionId: clipboard.id,
        fileName: "clipboard.png",
        remotePath: clipboard.remotePath,
        localContent: "local",
        remoteContent: "remote",
        finalContent: "local",
        remoteHash: "remote-hash",
        session: clipboard,
      },
      selectedError: clipboard,
    });

    useExternalEditStore.getState().applyEvent({
      type: "session_conflict",
      session: clipboard,
      saveResult: {
        status: "conflict_remote_changed",
        session: clipboard,
        conflict: {
          documentKey: clipboard.documentKey,
          primaryDraftSessionId: clipboard.id,
        },
      },
    });
    useExternalEditStore.getState().applyEvent({
      type: "session_auto_save",
      autoSave: {
        documentKey: clipboard.documentKey,
        phase: "running",
      },
    });

    const state = useExternalEditStore.getState();
    expect(state.sessions.clipboard).toBeUndefined();
    expect(state.sessions.valid).toBeDefined();
    expect(state.autoSavePhases[clipboard.documentKey]).toBeUndefined();
    expect(state.autoSavePhases[valid.documentKey]).toBe("pending");
    expect(state.pendingConflict).toBeNull();
    expect(state.compareResult).toBeNull();
    expect(state.mergeResult).toBeNull();
    expect(state.selectedError).toBeNull();
  });

  it("ignores clipboard residue returned from save, compare, merge, continue-edit recovery, and selected detail actions", async () => {
    const clipboard = makeClipboardResidueSession();
    const valid = makeSession({ id: "valid", documentKey: "101:/srv/app/demo.txt", state: "dirty", updatedAt: 10 });
    Object.assign(window.go!.app!.App!, {
      SaveExternalEditSession: vi.fn().mockResolvedValue({
        status: "conflict_remote_changed",
        session: clipboard,
        conflict: { documentKey: clipboard.documentKey, primaryDraftSessionId: clipboard.id },
      }),
      CompareExternalEditSession: vi.fn().mockResolvedValue({
        documentKey: clipboard.documentKey,
        primaryDraftSessionId: clipboard.id,
        fileName: "clipboard.png",
        remotePath: clipboard.remotePath,
        localContent: "local",
        remoteContent: "remote",
        readOnly: true,
        session: clipboard,
      }),
      PrepareExternalEditMerge: vi.fn().mockResolvedValue({
        documentKey: clipboard.documentKey,
        primaryDraftSessionId: clipboard.id,
        fileName: "clipboard.png",
        remotePath: clipboard.remotePath,
        localContent: "local",
        remoteContent: "remote",
        finalContent: "local",
        remoteHash: "remote-hash",
        session: clipboard,
      }),
      RecoverExternalEditSession: vi.fn().mockResolvedValue(clipboard),
    });
    useExternalEditStore.setState({
      sessions: { clipboard, valid },
      autoSavePhases: {
        [clipboard.documentKey]: "running",
        [valid.documentKey]: "pending",
      },
    });

    await useExternalEditStore.getState().saveSession(clipboard.id);
    await useExternalEditStore.getState().compareSession(clipboard.id);
    await useExternalEditStore.getState().prepareMerge(clipboard.id);
    await useExternalEditStore.getState().continuePendingSession(clipboard.id, "recovery");
    useExternalEditStore.getState().openErrorDetail(clipboard.id);

    const state = useExternalEditStore.getState();
    expect(state.sessions.clipboard).toBeUndefined();
    expect(state.sessions.valid).toBeDefined();
    expect(state.autoSavePhases[clipboard.documentKey]).toBeUndefined();
    expect(state.autoSavePhases[valid.documentKey]).toBe("pending");
    expect(state.pendingConflict).toBeNull();
    expect(state.compareResult).toBeNull();
    expect(state.mergeResult).toBeNull();
    expect(state.selectedError).toBeNull();
  });
});

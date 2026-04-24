/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { useSnippetStore } from "../stores/snippetStore";
import {
  ListSnippetCategories,
  ListSnippets,
  CreateSnippet,
  UpdateSnippet,
  DeleteSnippet,
  DuplicateSnippet,
  RecordSnippetUse,
  SetSnippetLastAssets,
  GetSnippetLastAssets,
} from "../../wailsjs/go/app/App";

describe("snippetStore", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSnippetStore.setState({
      categories: [],
      categoriesLoading: false,
      list: [],
      listLoading: false,
      filter: { categories: [], keyword: "" },
    });
    vi.mocked(ListSnippetCategories).mockResolvedValue([]);
    vi.mocked(ListSnippets).mockResolvedValue([]);
  });

  describe("loadCategories", () => {
    it("stores returned categories", async () => {
      vi.mocked(ListSnippetCategories).mockResolvedValue([
        { id: "shell", assetType: "ssh", label: "Shell", source: "builtin" } as any,
      ]);
      await useSnippetStore.getState().loadCategories();
      expect(ListSnippetCategories).toHaveBeenCalledTimes(1);
      expect(useSnippetStore.getState().categories).toHaveLength(1);
      expect(useSnippetStore.getState().categories[0].id).toBe("shell");
      expect(useSnippetStore.getState().categoriesLoading).toBe(false);
    });

    it("handles null response", async () => {
      vi.mocked(ListSnippetCategories).mockResolvedValue(null as any);
      await useSnippetStore.getState().loadCategories();
      expect(useSnippetStore.getState().categories).toEqual([]);
    });

    it("resets loading flag on error", async () => {
      vi.mocked(ListSnippetCategories).mockRejectedValue(new Error("fail"));
      await useSnippetStore
        .getState()
        .loadCategories()
        .catch(() => {});
      expect(useSnippetStore.getState().categoriesLoading).toBe(false);
    });
  });

  describe("loadList", () => {
    it("passes current filter to ListSnippets", async () => {
      useSnippetStore.setState({ filter: { categories: ["shell", "sql"], keyword: "foo" } });
      vi.mocked(ListSnippets).mockResolvedValue([]);
      await useSnippetStore.getState().loadList();
      expect(ListSnippets).toHaveBeenCalledTimes(1);
      const arg = vi.mocked(ListSnippets).mock.calls[0][0] as any;
      expect(arg.categories).toEqual(["shell", "sql"]);
      expect(arg.keyword).toBe("foo");
      expect(arg.limit).toBe(0);
      expect(arg.offset).toBe(0);
    });

    it("stores returned list", async () => {
      vi.mocked(ListSnippets).mockResolvedValue([
        { ID: 1, Name: "ls", Category: "shell", Content: "ls -al", Source: "user" } as any,
      ]);
      await useSnippetStore.getState().loadList();
      expect(useSnippetStore.getState().list).toHaveLength(1);
      expect(useSnippetStore.getState().listLoading).toBe(false);
    });

    it("handles null response as empty array", async () => {
      vi.mocked(ListSnippets).mockResolvedValue(null as any);
      await useSnippetStore.getState().loadList();
      expect(useSnippetStore.getState().list).toEqual([]);
    });

    it("resets loading flag on error", async () => {
      vi.mocked(ListSnippets).mockRejectedValue(new Error("fail"));
      await useSnippetStore
        .getState()
        .loadList()
        .catch(() => {});
      expect(useSnippetStore.getState().listLoading).toBe(false);
    });
  });

  describe("setFilter", () => {
    it("merges partial filter and triggers loadList", async () => {
      vi.mocked(ListSnippets).mockResolvedValue([]);
      useSnippetStore.getState().setFilter({ keyword: "abc" });
      expect(useSnippetStore.getState().filter).toEqual({ categories: [], keyword: "abc" });
      // setFilter triggers loadList asynchronously
      await vi.waitFor(() => expect(ListSnippets).toHaveBeenCalled());
    });

    it("merges categories without clobbering other fields", () => {
      useSnippetStore.setState({ filter: { categories: [], keyword: "x" } });
      useSnippetStore.getState().setFilter({ categories: ["shell"] });
      expect(useSnippetStore.getState().filter).toEqual({ categories: ["shell"], keyword: "x" });
    });

    it("short-circuits when patch doesn't change filter", async () => {
      vi.mocked(ListSnippets).mockResolvedValue([]);
      useSnippetStore.setState({ filter: { categories: [], keyword: "" } });
      useSnippetStore.getState().setFilter({ keyword: "" });
      // Give microtasks a chance to flush.
      await Promise.resolve();
      expect(ListSnippets).not.toHaveBeenCalled();
    });

    it("short-circuits on equal categories array (same ids, same order)", async () => {
      vi.mocked(ListSnippets).mockResolvedValue([]);
      useSnippetStore.setState({ filter: { categories: ["shell", "sql"], keyword: "" } });
      useSnippetStore.getState().setFilter({ categories: ["shell", "sql"] });
      await Promise.resolve();
      expect(ListSnippets).not.toHaveBeenCalled();
    });
  });

  describe("stale request guard", () => {
    it("discards stale loadList response when a newer request is in flight", async () => {
      let releaseFirst!: (v: any[]) => void;
      const firstPromise = new Promise<any[]>((r) => {
        releaseFirst = r;
      });
      vi.mocked(ListSnippets)
        .mockImplementationOnce(() => firstPromise as any)
        .mockResolvedValueOnce([{ ID: 2, Name: "B", Category: "shell", Source: "user" } as any]);

      const store = useSnippetStore.getState();
      const first = store.loadList();
      const second = store.loadList();
      await second; // second resolves first -> list = [B]
      // Now release the stale first response; it must be discarded.
      releaseFirst([{ ID: 1, Name: "A", Category: "shell", Source: "user" } as any]);
      await first;

      expect(useSnippetStore.getState().list.map((s) => s.ID)).toEqual([2]);
    });
  });

  describe("mutations reload list", () => {
    it("create triggers loadList", async () => {
      vi.mocked(CreateSnippet).mockResolvedValue({ ID: 1, Name: "a" } as any);
      vi.mocked(ListSnippets).mockResolvedValue([]);
      await useSnippetStore.getState().create({ name: "a", category: "shell", content: "ls" } as any);
      expect(CreateSnippet).toHaveBeenCalled();
      expect(ListSnippets).toHaveBeenCalled();
    });

    it("update triggers loadList", async () => {
      vi.mocked(UpdateSnippet).mockResolvedValue({ ID: 1, Name: "a" } as any);
      vi.mocked(ListSnippets).mockResolvedValue([]);
      await useSnippetStore.getState().update({ id: 1, name: "a", content: "ls" } as any);
      expect(UpdateSnippet).toHaveBeenCalled();
      expect(ListSnippets).toHaveBeenCalled();
    });

    it("remove triggers loadList", async () => {
      vi.mocked(DeleteSnippet).mockResolvedValue(undefined);
      vi.mocked(ListSnippets).mockResolvedValue([]);
      await useSnippetStore.getState().remove(7);
      expect(DeleteSnippet).toHaveBeenCalledWith(7);
      expect(ListSnippets).toHaveBeenCalled();
    });

    it("duplicate triggers loadList", async () => {
      vi.mocked(DuplicateSnippet).mockResolvedValue({ ID: 2, Name: "a (copy)" } as any);
      vi.mocked(ListSnippets).mockResolvedValue([]);
      await useSnippetStore.getState().duplicate(1);
      expect(DuplicateSnippet).toHaveBeenCalledWith(1);
      expect(ListSnippets).toHaveBeenCalled();
    });
  });

  describe("recordUse", () => {
    it("calls RecordSnippetUse and swallows rejection", async () => {
      vi.mocked(RecordSnippetUse).mockRejectedValue(new Error("boom"));
      // Must NOT throw to the caller.
      expect(() => useSnippetStore.getState().recordUse(42)).not.toThrow();
      // Give the microtask queue a chance to settle.
      await new Promise((r) => setTimeout(r, 0));
      expect(RecordSnippetUse).toHaveBeenCalledWith(42);
    });

    it("fires RecordSnippetUse with provided id on success", async () => {
      vi.mocked(RecordSnippetUse).mockResolvedValue(undefined);
      useSnippetStore.getState().recordUse(7);
      await new Promise((r) => setTimeout(r, 0));
      expect(RecordSnippetUse).toHaveBeenCalledWith(7);
    });
  });

  describe("setLastAssets / getLastAssets", () => {
    it("setLastAssets calls SetSnippetLastAssets with id and array", async () => {
      vi.mocked(SetSnippetLastAssets).mockResolvedValue(undefined);
      await useSnippetStore.getState().setLastAssets(5, [1, 2, 3]);
      expect(SetSnippetLastAssets).toHaveBeenCalledWith(5, [1, 2, 3]);
    });

    it("getLastAssets returns ids from GetSnippetLastAssets", async () => {
      vi.mocked(GetSnippetLastAssets).mockResolvedValue([10, 20] as any);
      const ids = await useSnippetStore.getState().getLastAssets(5);
      expect(GetSnippetLastAssets).toHaveBeenCalledWith(5);
      expect(ids).toEqual([10, 20]);
    });

    it("getLastAssets returns empty array when response is null", async () => {
      vi.mocked(GetSnippetLastAssets).mockResolvedValue(null as any);
      const ids = await useSnippetStore.getState().getLastAssets(5);
      expect(ids).toEqual([]);
    });
  });
});

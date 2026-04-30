/* eslint-disable @typescript-eslint/no-explicit-any */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { useAssetStore } from "../stores/assetStore";
import { useRecentAssetStore } from "../stores/recentAssetStore";
import {
  ListAssets,
  ListGroups,
  CreateAsset,
  UpdateAsset,
  DeleteAsset,
  GetAsset,
  CreateGroup,
  DeleteGroup,
} from "../../wailsjs/go/app/App";

vi.mocked(ListAssets).mockResolvedValue([]);
vi.mocked(ListGroups).mockResolvedValue([]);

describe("assetStore", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useAssetStore.setState({
      assets: [],
      groups: [],
      selectedAssetId: null,
      selectedGroupId: null,
      loading: false,
      initialized: false,
    });
    useRecentAssetStore.setState({ recentIds: [] });
  });

  describe("fetchAssets", () => {
    it("sets loading true during fetch, false after", async () => {
      vi.mocked(ListAssets).mockResolvedValue([{ ID: 1, Name: "Server1" }] as any);

      const promise = useAssetStore.getState().fetchAssets();
      expect(useAssetStore.getState().loading).toBe(true);

      await promise;
      expect(useAssetStore.getState().loading).toBe(false);
      expect(useAssetStore.getState().assets).toHaveLength(1);
    });

    it("passes assetType and groupId to backend", async () => {
      vi.mocked(ListAssets).mockResolvedValue([]);
      await useAssetStore.getState().fetchAssets("ssh", 5);
      expect(ListAssets).toHaveBeenCalledWith("ssh", 5);
    });

    it("defaults to empty string and 0", async () => {
      vi.mocked(ListAssets).mockResolvedValue([]);
      await useAssetStore.getState().fetchAssets();
      expect(ListAssets).toHaveBeenCalledWith("", 0);
    });

    it("handles null response as empty array", async () => {
      vi.mocked(ListAssets).mockResolvedValue(null as any);
      await useAssetStore.getState().fetchAssets();
      expect(useAssetStore.getState().assets).toEqual([]);
    });

    it("sets loading false even on error", async () => {
      vi.mocked(ListAssets).mockRejectedValue(new Error("fail"));
      await useAssetStore
        .getState()
        .fetchAssets()
        .catch(() => {});
      expect(useAssetStore.getState().loading).toBe(false);
    });
  });

  describe("fetchGroups", () => {
    it("fetches and stores groups", async () => {
      vi.mocked(ListGroups).mockResolvedValue([{ ID: 1, Name: "G1", ParentID: 0 }] as any);
      await useAssetStore.getState().fetchGroups();
      expect(useAssetStore.getState().groups).toHaveLength(1);
    });

    it("handles null response as empty array", async () => {
      vi.mocked(ListGroups).mockResolvedValue(null as any);
      await useAssetStore.getState().fetchGroups();
      expect(useAssetStore.getState().groups).toEqual([]);
    });
  });

  describe("CRUD operations", () => {
    it("createAsset calls backend and refreshes", async () => {
      vi.mocked(CreateAsset).mockResolvedValue(undefined as any);
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      await useAssetStore.getState().createAsset({ Name: "New" } as any);
      expect(CreateAsset).toHaveBeenCalledWith({ Name: "New" });
      expect(ListAssets).toHaveBeenCalled();
    });

    it("updateAsset calls backend and refreshes", async () => {
      vi.mocked(UpdateAsset).mockResolvedValue(undefined as any);
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      await useAssetStore.getState().updateAsset({ ID: 1, Name: "Updated" } as any);
      expect(UpdateAsset).toHaveBeenCalledWith({ ID: 1, Name: "Updated" });
    });

    it("deleteAsset calls backend, clears selection, and refreshes", async () => {
      vi.mocked(DeleteAsset).mockResolvedValue(undefined as any);
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      useAssetStore.setState({ selectedAssetId: 1 });
      await useAssetStore.getState().deleteAsset(1);
      expect(DeleteAsset).toHaveBeenCalledWith(1);
      expect(useAssetStore.getState().selectedAssetId).toBeNull();
    });

    it("deleteAsset removes the asset from recentAssetStore after backend succeeds", async () => {
      vi.mocked(DeleteAsset).mockResolvedValue(undefined as any);
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      // Pre-seed the recent store as if the asset was previously opened
      useRecentAssetStore.getState().touch(42);
      expect(useRecentAssetStore.getState().recentIds.includes(42)).toBe(true);

      await useAssetStore.getState().deleteAsset(42);

      expect(useRecentAssetStore.getState().recentIds.includes(42)).toBe(false);
    });

    it("does not remove from recentAssetStore when backend deleteAsset fails", async () => {
      vi.mocked(DeleteAsset).mockRejectedValue(new Error("backend error"));
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      useRecentAssetStore.getState().touch(99);

      await useAssetStore
        .getState()
        .deleteAsset(99)
        .catch(() => {});

      // Recent entry should remain because the backend call failed
      expect(useRecentAssetStore.getState().recentIds.includes(99)).toBe(true);
    });

    it("getAsset calls backend", async () => {
      vi.mocked(GetAsset).mockResolvedValue({ ID: 1, Name: "S1" } as any);
      const asset = await useAssetStore.getState().getAsset(1);
      expect(GetAsset).toHaveBeenCalledWith(1);
      expect(asset.Name).toBe("S1");
    });
  });

  describe("group CRUD", () => {
    it("createGroup calls backend and fetches groups", async () => {
      vi.mocked(CreateGroup).mockResolvedValue(undefined as any);
      vi.mocked(ListGroups).mockResolvedValue([]);

      await useAssetStore.getState().createGroup({ Name: "G1" } as any);
      expect(CreateGroup).toHaveBeenCalled();
      expect(ListGroups).toHaveBeenCalled();
    });

    it("deleteGroup calls backend with deleteAssets flag", async () => {
      vi.mocked(DeleteGroup).mockResolvedValue(undefined as any);
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      await useAssetStore.getState().deleteGroup(1, true);
      expect(DeleteGroup).toHaveBeenCalledWith(1, true);
    });
  });

  describe("getAssetPath", () => {
    it("returns just asset name when no group", () => {
      useAssetStore.setState({ groups: [] });
      const path = useAssetStore.getState().getAssetPath({ Name: "Server1", GroupID: 0 } as any);
      expect(path).toBe("Server1");
    });

    it("returns group / asset path", () => {
      useAssetStore.setState({
        groups: [{ ID: 1, Name: "Production", ParentID: 0 }] as any,
      });
      const path = useAssetStore.getState().getAssetPath({ Name: "Server1", GroupID: 1 } as any);
      expect(path).toBe("Production / Server1");
    });

    it("resolves nested group hierarchy", () => {
      useAssetStore.setState({
        groups: [
          { ID: 1, Name: "Infra", ParentID: 0 },
          { ID: 2, Name: "US-East", ParentID: 1 },
        ] as any,
      });
      const path = useAssetStore.getState().getAssetPath({ Name: "Web01", GroupID: 2 } as any);
      expect(path).toBe("Infra / US-East / Web01");
    });

    it("stops at missing parent group", () => {
      useAssetStore.setState({
        groups: [{ ID: 2, Name: "Orphan", ParentID: 999 }] as any,
      });
      const path = useAssetStore.getState().getAssetPath({ Name: "S1", GroupID: 2 } as any);
      expect(path).toBe("Orphan / S1");
    });

    it("stops when group parents form a cycle", () => {
      useAssetStore.setState({
        groups: [
          { ID: 1, Name: "A", ParentID: 2 },
          { ID: 2, Name: "B", ParentID: 1 },
        ] as any,
      });
      const path = useAssetStore.getState().getAssetPath({ Name: "S1", GroupID: 1 } as any);
      expect(path).toBe("B / A / S1");
    });
  });

  describe("selection", () => {
    it("selectAsset sets selectedAssetId", () => {
      useAssetStore.getState().selectAsset(42);
      expect(useAssetStore.getState().selectedAssetId).toBe(42);
    });

    it("selectGroup sets selectedGroupId", () => {
      useAssetStore.getState().selectGroup(10);
      expect(useAssetStore.getState().selectedGroupId).toBe(10);
    });

    it("selectAsset with null clears selection", () => {
      useAssetStore.getState().selectAsset(42);
      useAssetStore.getState().selectAsset(null);
      expect(useAssetStore.getState().selectedAssetId).toBeNull();
    });
  });

  describe("initialized", () => {
    it("starts as false", () => {
      expect(useAssetStore.getState().initialized).toBe(false);
    });

    it("becomes true after fetchAssets completes", async () => {
      vi.mocked(ListAssets).mockResolvedValue([]);
      await useAssetStore.getState().fetchAssets();
      expect(useAssetStore.getState().initialized).toBe(true);
    });

    it("becomes true even when fetchAssets returns null", async () => {
      vi.mocked(ListAssets).mockResolvedValue(null as any);
      await useAssetStore.getState().fetchAssets();
      expect(useAssetStore.getState().initialized).toBe(true);
    });

    it("stays false when fetchAssets throws", async () => {
      vi.mocked(ListAssets).mockRejectedValue(new Error("fail"));
      await useAssetStore
        .getState()
        .fetchAssets()
        .catch(() => {});
      expect(useAssetStore.getState().initialized).toBe(false);
    });
  });

  describe("refresh", () => {
    it("calls both fetchAssets and fetchGroups", async () => {
      vi.mocked(ListAssets).mockResolvedValue([]);
      vi.mocked(ListGroups).mockResolvedValue([]);

      await useAssetStore.getState().refresh();
      expect(ListAssets).toHaveBeenCalled();
      expect(ListGroups).toHaveBeenCalled();
    });
  });
});

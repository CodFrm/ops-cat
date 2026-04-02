import { create } from "zustand";
import { asset_entity, group_entity } from "../../wailsjs/go/models";
import {
  ListAssets,
  CreateAsset,
  UpdateAsset,
  DeleteAsset,
  GetAsset,
  ListGroups,
  CreateGroup,
  UpdateGroup,
  DeleteGroup,
} from "../../wailsjs/go/app/App";

interface AssetState {
  assets: asset_entity.Asset[];
  groups: group_entity.Group[];
  selectedAssetId: number | null;
  selectedGroupId: number | null;
  loading: boolean;
  initialized: boolean;

  fetchAssets: (assetType?: string, groupId?: number) => Promise<void>;
  fetchGroups: () => Promise<void>;
  createAsset: (asset: asset_entity.Asset) => Promise<void>;
  updateAsset: (asset: asset_entity.Asset) => Promise<void>;
  deleteAsset: (id: number) => Promise<void>;
  getAsset: (id: number) => Promise<asset_entity.Asset>;
  getAssetPath: (asset: asset_entity.Asset) => string;
  createGroup: (group: group_entity.Group) => Promise<void>;
  updateGroup: (group: group_entity.Group) => Promise<void>;
  deleteGroup: (id: number, deleteAssets: boolean) => Promise<void>;
  selectAsset: (id: number | null) => void;
  selectGroup: (id: number | null) => void;
  refresh: () => Promise<void>;
}

export const useAssetStore = create<AssetState>((set, get) => ({
  assets: [],
  groups: [],
  selectedAssetId: null,
  selectedGroupId: null,
  loading: false,
  initialized: false,

  fetchAssets: async (assetType = "", groupId = 0) => {
    set({ loading: true });
    try {
      const assets = await ListAssets(assetType, groupId);
      set({ assets: assets || [], initialized: true });
    } finally {
      set({ loading: false });
    }
  },

  fetchGroups: async () => {
    const groups = await ListGroups();
    set({ groups: groups || [] });
  },

  createAsset: async (asset) => {
    await CreateAsset(asset);
    await get().refresh();
  },

  updateAsset: async (asset) => {
    await UpdateAsset(asset);
    await get().refresh();
  },

  deleteAsset: async (id) => {
    await DeleteAsset(id);
    set({ selectedAssetId: null });
    await get().refresh();
  },

  getAsset: async (id) => {
    return await GetAsset(id);
  },

  getAssetPath: (asset) => {
    const { groups } = get();
    const parts: string[] = [asset.Name];
    let groupId = asset.GroupID;
    while (groupId > 0) {
      const group = groups.find((g) => g.ID === groupId);
      if (!group) break;
      parts.unshift(group.Name);
      groupId = group.ParentID;
    }
    return parts.join(" / ");
  },

  createGroup: async (group) => {
    await CreateGroup(group);
    await get().fetchGroups();
  },

  updateGroup: async (group) => {
    await UpdateGroup(group);
    await get().fetchGroups();
  },

  deleteGroup: async (id, deleteAssets) => {
    await DeleteGroup(id, deleteAssets);
    await get().refresh();
  },

  selectAsset: (id) => set({ selectedAssetId: id }),
  selectGroup: (id) => set({ selectedGroupId: id }),

  refresh: async () => {
    await Promise.all([get().fetchAssets(), get().fetchGroups()]);
  },
}));

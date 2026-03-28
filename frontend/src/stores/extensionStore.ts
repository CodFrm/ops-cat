import { create } from "zustand";
import { GetExtensions } from "../../wailsjs/go/app/App";

export interface ExtensionAssetType {
  type: string;
  name: string;
  name_en: string;
  configSchema: any;
}

export interface ExtensionPage {
  id: string;
  name: string;
  component: string;
}

export interface ExtensionInfo {
  name: string;
  displayName: string;
  version: string;
  icon: string;
  description: string;
  assetTypes: ExtensionAssetType[];
  pages: ExtensionPage[];
}

interface ExtensionState {
  extensions: ExtensionInfo[];
  loading: boolean;
  fetchExtensions: () => Promise<void>;
  getExtensionForAssetType: (type: string) => ExtensionInfo | undefined;
  isExtensionAssetType: (type: string) => boolean;
}

export const useExtensionStore = create<ExtensionState>((set, get) => ({
  extensions: [],
  loading: false,

  fetchExtensions: async () => {
    set({ loading: true });
    try {
      const exts = await GetExtensions();
      set({ extensions: (exts as unknown as ExtensionInfo[]) || [] });
    } catch (e) {
      console.error("Failed to fetch extensions:", e);
      set({ extensions: [] });
    } finally {
      set({ loading: false });
    }
  },

  getExtensionForAssetType: (type: string) => {
    return get().extensions.find((ext) =>
      ext.assetTypes?.some((at) => at.type === type)
    );
  },

  isExtensionAssetType: (type: string) => {
    const builtinTypes = ["ssh", "database", "redis"];
    if (builtinTypes.includes(type)) return false;
    return get().extensions.some((ext) =>
      ext.assetTypes?.some((at) => at.type === type)
    );
  },
}));

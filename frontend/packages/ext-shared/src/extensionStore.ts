import { create } from "zustand";
import type { ExtensionAPI, ExtensionInfo } from "./api";

interface ExtensionState {
  extensions: ExtensionInfo[];
  loading: boolean;
  fetchExtensions: () => Promise<void>;
  installExtension: (sourcePath: string) => Promise<void>;
  removeExtension: (name: string) => Promise<void>;
  getExtensionForAssetType: (type: string) => ExtensionInfo | undefined;
  isExtensionAssetType: (type: string) => boolean;
}

/**
 * Create a Zustand extension store bound to a specific ExtensionAPI implementation.
 * Call once at app startup with the appropriate API (Wails or HTTP).
 */
export function createExtensionStore(api: ExtensionAPI) {
  return create<ExtensionState>((set, get) => ({
    extensions: [],
    loading: false,

    fetchExtensions: async () => {
      set({ loading: true });
      try {
        const exts = await api.getExtensions();
        set({ extensions: exts || [] });
      } catch (e) {
        console.error("Failed to fetch extensions:", e);
        set({ extensions: [] });
      } finally {
        set({ loading: false });
      }
    },

    installExtension: async (sourcePath: string) => {
      await api.installExtension(sourcePath);
      await get().fetchExtensions();
    },

    removeExtension: async (name: string) => {
      await api.removeExtension(name);
      set((state) => ({
        extensions: state.extensions.filter((e) => e.name !== name),
      }));
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
}

// frontend/src/extension/store.ts
import { create } from "zustand";
import type { ExtManifest, LoadedExtension } from "./types";

interface ExtensionEntry {
  manifest: ExtManifest;
  loaded?: LoadedExtension;
}

interface ExtensionState {
  ready: boolean;
  extensions: Record<string, ExtensionEntry>;
  setReady: (ready: boolean) => void;
  register: (name: string, manifest: ExtManifest) => void;
  unregister: (name: string) => void;
  setLoaded: (name: string, loaded: LoadedExtension) => void;
  getExtensionForAssetType: (assetType: string) => { name: string; manifest: ExtManifest } | undefined;
  isExtensionAssetType: (assetType: string) => boolean;
}

export const useExtensionStore = create<ExtensionState>((set, get) => ({
  ready: false,
  extensions: {},

  setReady(ready) {
    set({ ready });
  },

  register(name, manifest) {
    set((s) => ({ extensions: { ...s.extensions, [name]: { manifest } } }));
  },

  unregister(name) {
    set((s) => {
      const { [name]: _, ...rest } = s.extensions;
      return { extensions: rest };
    });
  },

  setLoaded(name, loaded) {
    set((s) => {
      const entry = s.extensions[name];
      if (!entry) return s;
      return { extensions: { ...s.extensions, [name]: { ...entry, loaded } } };
    });
  },

  getExtensionForAssetType(assetType) {
    for (const [name, entry] of Object.entries(get().extensions)) {
      if (entry.manifest.assetTypes?.some((at) => at.type === assetType)) {
        return { name, manifest: entry.manifest };
      }
    }
    return undefined;
  },

  isExtensionAssetType(assetType) {
    return get().getExtensionForAssetType(assetType) !== undefined;
  },
}));

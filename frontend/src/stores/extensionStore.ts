import { create } from "zustand";
import { GetExtensions, InstallExtension, RemoveExtension } from "../../wailsjs/go/app/App";

export interface ExtensionAssetType {
  type: string;
  name: string;
  name_zh?: string;
  namePlaceholder?: string;
  testConnection?: boolean;
  configSchema: Record<string, unknown>;
}

export interface ExtensionPage {
  id: string;
  name: string;
  name_zh?: string;
  component: string;
}

export interface ExtensionInfo {
  name: string;
  displayName: string;
  displayName_zh?: string;
  version: string;
  icon: string;
  description: string;
  description_zh?: string;
  assetTypes: ExtensionAssetType[];
  pages: ExtensionPage[];
  policyType?: string;
  policyActions?: string[];
}

/**
 * Get localized text from extension manifest fields.
 * English is the default (no suffix), Chinese uses _zh suffix.
 */
export function extLocalized(text: string, textZh: string | undefined, lang: string): string {
  if ((lang === "zh" || lang === "zh-CN" || lang.startsWith("zh")) && textZh) return textZh;
  return text;
}

interface ExtensionState {
  extensions: ExtensionInfo[];
  loading: boolean;
  fetchExtensions: () => Promise<void>;
  installExtension: (sourcePath: string) => Promise<void>;
  removeExtension: (name: string) => Promise<void>;
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

  installExtension: async (sourcePath: string) => {
    await InstallExtension(sourcePath);
    await get().fetchExtensions();
  },

  removeExtension: async (name: string) => {
    await RemoveExtension(name);
    set((state) => ({
      extensions: state.extensions.filter((e) => e.name !== name),
    }));
  },

  getExtensionForAssetType: (type: string) => {
    return get().extensions.find((ext) => ext.assetTypes?.some((at) => at.type === type));
  },

  isExtensionAssetType: (type: string) => {
    const builtinTypes = ["ssh", "database", "redis"];
    if (builtinTypes.includes(type)) return false;
    return get().extensions.some((ext) => ext.assetTypes?.some((at) => at.type === type));
  },
}));

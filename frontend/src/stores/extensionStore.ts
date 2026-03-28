// Re-export shared types and utilities from @opskat/ext-shared.
// The store is created with WailsExtensionAPI in main.tsx.
export type {
  ExtensionInfo,
  ExtensionAssetType,
  ExtensionPage,
} from "@opskat/ext-shared";
export { extLocalized, createExtensionStore } from "@opskat/ext-shared";

import { createExtensionStore } from "@opskat/ext-shared";
import { WailsExtensionAPI } from "../lib/wailsExtensionAPI";

// Singleton store bound to Wails API (backward compatibility)
export const useExtensionStore = createExtensionStore(new WailsExtensionAPI());

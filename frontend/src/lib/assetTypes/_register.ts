import type { AssetTypeDefinition } from "./types";

export const registry = new Map<string, AssetTypeDefinition>();

export function registerAssetType(def: AssetTypeDefinition) {
  registry.set(def.type, def);
}

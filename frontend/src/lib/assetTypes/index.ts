import type { AssetTypeDefinition } from "./types";
import { registry } from "./_register";
export { registerAssetType } from "./_register";

export function getAssetType(type: string): AssetTypeDefinition | undefined {
  return registry.get(type);
}

export function isBuiltinType(type: string): boolean {
  return registry.has(type);
}

export function getBuiltinTypes(): AssetTypeDefinition[] {
  return [...registry.values()];
}

// Side-effect imports — register all built-in types
import "./ssh";
import "./database";
import "./redis";
import "./mongodb";
import "./k8s";

export type { AssetTypeDefinition, DetailInfoCardProps, PolicyDefinition, PolicyFieldDef } from "./types";

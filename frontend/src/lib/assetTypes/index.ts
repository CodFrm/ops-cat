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

export type HomeSection = "home" | "database" | "ssh" | "redis" | "mongodb";

export function normalizeAssetSection(type: string): "database" | "ssh" | "redis" | "mongodb" | undefined {
  const normalized = type.trim().toLowerCase();
  if (!normalized) return undefined;
  if (normalized === "mysql" || normalized === "postgresql") return "database";
  if (normalized === "mongo") return "mongodb";
  if (normalized === "database" || normalized === "ssh" || normalized === "redis" || normalized === "mongodb") {
    return normalized;
  }
  return undefined;
}

// Side-effect imports — register all built-in types
import "./ssh";
import "./database";
import "./redis";
import "./mongodb";

export type { AssetTypeDefinition, DetailInfoCardProps, PolicyDefinition, PolicyFieldDef } from "./types";

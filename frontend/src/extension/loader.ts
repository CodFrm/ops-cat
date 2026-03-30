// frontend/src/extension/loader.ts
import type { ComponentType } from "react";
import type { ExtManifest, LoadedExtension } from "./types";

const cache = new Map<string, LoadedExtension>();

export async function loadExtension(name: string, manifest: ExtManifest): Promise<LoadedExtension> {
  const cached = cache.get(name);
  if (cached) return cached;

  const frontend = manifest.frontend;
  if (!frontend) throw new Error(`Extension "${name}" has no frontend definition`);

  // Inject CSS
  if (frontend.styles) {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = `/extensions/${name}/${frontend.styles}`;
    document.head.appendChild(link);
  }

  // Load ESM module
  const mod = await import(/* @vite-ignore */ `/extensions/${name}/${frontend.entry}`);

  // Extract page components
  const components: Record<string, ComponentType<{ assetId?: number }>> = {};
  for (const page of frontend.pages) {
    if (mod[page.component]) {
      components[page.component] = mod[page.component];
    }
  }

  const loaded: LoadedExtension = { name, manifest, components };
  cache.set(name, loaded);
  return loaded;
}

export function clearExtensionCache(name?: string): void {
  if (name) cache.delete(name);
  else cache.clear();
}

// frontend/src/extension/loader.ts
import type { ComponentType } from "react";
import type { ExtManifest, LoadedExtension } from "./types";

const cache = new Map<string, LoadedExtension>();

const NAME_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;

function validateName(name: string): void {
  if (!NAME_RE.test(name)) {
    throw new Error(`Invalid extension name: ${JSON.stringify(name)} (must match ${NAME_RE})`);
  }
}

function validateEntryPath(entry: string, kind: string): void {
  if (!entry) return;
  if (entry.includes("..")) {
    throw new Error(`Extension ${kind} path contains traversal: ${JSON.stringify(entry)}`);
  }
  if (entry.startsWith("/") || entry.startsWith("\\")) {
    throw new Error(`Extension ${kind} path must be relative: ${JSON.stringify(entry)}`);
  }
  // Reject URL-like values
  if (/^[a-z]+:\/\//i.test(entry)) {
    throw new Error(`Extension ${kind} path must not be a URL: ${JSON.stringify(entry)}`);
  }
}

export async function loadExtension(name: string, manifest: ExtManifest): Promise<LoadedExtension> {
  validateName(name);

  const cached = cache.get(name);
  if (cached) return cached;

  const frontend = manifest.frontend;
  if (!frontend) throw new Error(`Extension "${name}" has no frontend definition`);

  validateEntryPath(frontend.entry, "entry");
  if (frontend.styles) {
    validateEntryPath(frontend.styles, "styles");
  }

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

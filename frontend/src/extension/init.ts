// frontend/src/extension/init.ts
import { ListInstalledExtensions } from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { useExtensionStore } from "./store";
import { injectExtensionAPI } from "./inject";
import { createExtensionAPI } from "./api";
import { clearExtensionCache } from "./loader";
import type { ExtManifest } from "./types";

export async function initExtensions(): Promise<void> {
  injectExtensionAPI(createExtensionAPI());
  await refreshExtensions();
  EventsOn("ext:reload", () => {
    clearExtensionCache();
    refreshExtensions();
  });
}

async function refreshExtensions(): Promise<void> {
  try {
    const extensions = await ListInstalledExtensions();
    const store = useExtensionStore.getState();

    const newNames = new Set((extensions || []).map((e: any) => e.name));
    for (const name of Object.keys(store.extensions)) {
      if (!newNames.has(name)) {
        store.unregister(name);
      }
    }

    for (const ext of extensions || []) {
      store.register(ext.name, ext.manifest as ExtManifest);
    }
  } catch (err) {
    console.error("Failed to load extensions:", err);
  }
}

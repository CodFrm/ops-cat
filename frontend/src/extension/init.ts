// frontend/src/extension/init.ts
import { ListInstalledExtensions } from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { useExtensionStore } from "./store";
import { injectExtensionAPI } from "./inject";
import { createExtensionAPI } from "./api";
import { clearExtensionCache } from "./loader";
import type { ExtManifest } from "./types";

let _bootstrapped = false;
let _subscribed = false;

/**
 * One-shot bootstrap: inject API, load extension list, subscribe to events.
 * If the backend hasn't finished init yet (returns empty), ready is NOT set —
 * we wait for the ext:ready event from the backend to refresh and set ready.
 * Safe to call multiple times — only the first call takes effect.
 */
export async function bootstrapExtensions(): Promise<void> {
  if (_bootstrapped) return;
  _bootstrapped = true;
  injectExtensionAPI(createExtensionAPI());
  subscribeExtensionReload(); // subscribe BEFORE async gap — no events lost
  subscribeExtensionReady();
  const loaded = await refreshExtensions();
  // 只有实际获取到扩展时才设置 ready，否则等 ext:ready 事件
  if (loaded) {
    useExtensionStore.getState().setReady(true);
  }
}

/**
 * Register ext:reload event listener. Returns cleanup function.
 * Safe to call multiple times — only the first call registers.
 */
export function subscribeExtensionReload(): () => void {
  if (_subscribed) return () => {};
  _subscribed = true;
  const cancel = EventsOn("ext:reload", () => {
    clearExtensionCache();
    refreshExtensions();
  });
  return cancel;
}

let _readySubscribed = false;

/**
 * Listen for ext:ready from backend (emitted after extension init completes).
 * On receive, refresh extensions and mark ready.
 */
function subscribeExtensionReady(): void {
  if (_readySubscribed) return;
  _readySubscribed = true;
  EventsOn("ext:ready", async () => {
    await refreshExtensions();
    useExtensionStore.getState().setReady(true);
  });
}

/** Compat wrapper — calls bootstrap + subscribe. */
export async function initExtensions(): Promise<void> {
  await bootstrapExtensions();
  subscribeExtensionReload();
}

/**
 * Refresh extension list from the backend.
 * Returns true if extensions were loaded, false if the list was empty.
 */
async function refreshExtensions(): Promise<boolean> {
  try {
    const extensions = await ListInstalledExtensions();
    const store = useExtensionStore.getState();

    const newNames = new Set((extensions || []).map((e: { name: string }) => e.name));
    for (const name of Object.keys(store.extensions)) {
      if (!newNames.has(name)) {
        store.unregister(name);
      }
    }

    for (const ext of extensions || []) {
      if (ext.enabled) {
        store.register(ext.name, ext.manifest as ExtManifest);
      }
    }

    return (extensions || []).length > 0;
  } catch (err) {
    console.error("Failed to load extensions:", err);
    return false;
  }
}

// Exports for testing only
export { refreshExtensions as _refreshExtensions };
export function _resetForTesting(): void {
  _bootstrapped = false;
  _subscribed = false;
  _readySubscribed = false;
}

// frontend/src/extension/index.ts
export { useExtensionStore } from "./store";
export { loadExtension, clearExtensionCache } from "./loader";
export { loadExtensionLocales } from "./i18n";
export { injectExtensionAPI } from "./inject";
export { createExtensionAPI } from "./api";
export { initExtensions, bootstrapExtensions, subscribeExtensionReload } from "./init";
export type { ExtManifest, ExtPage, ExtFrontend, LoadedExtension, ExtAPI, ExtEvent } from "./types";
export { ExtensionPage } from "./ExtensionPage";

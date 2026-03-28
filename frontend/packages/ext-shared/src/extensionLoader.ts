import React from "react";

/** ExtensionModule maps component names to React components */
export interface ExtensionModule {
  [componentName: string]: React.ComponentType<Record<string, unknown>>;
}

const loadedModules: Map<string, ExtensionModule> = new Map();
const loadingPromises: Map<string, Promise<ExtensionModule>> = new Map();

/**
 * Load an extension frontend module (IIFE format, registered to window.__OPSKAT_EXT_{name}__).
 * Each extension is loaded once; subsequent calls return the cached module.
 * @param extName Extension name
 * @param basePath Base URL path for extension assets (default: "/extensions")
 */
export function loadExtensionModule(
  extName: string,
  basePath = "/extensions"
): Promise<ExtensionModule> {
  const cached = loadedModules.get(extName);
  if (cached) return Promise.resolve(cached);

  const loading = loadingPromises.get(extName);
  if (loading) return loading;

  const promise = new Promise<ExtensionModule>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = `${basePath}/${extName}/frontend/index.js`;
    script.onload = () => {
      const globalName = `__OPSKAT_EXT_${extName}__`;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const mod = (window as any)[globalName] as ExtensionModule | undefined;
      if (mod) {
        loadedModules.set(extName, mod);
        loadingPromises.delete(extName);
        resolve(mod);
      } else {
        loadingPromises.delete(extName);
        reject(
          new Error(
            `Extension "${extName}" loaded but window.${globalName} not found`
          )
        );
      }
    };
    script.onerror = () => {
      loadingPromises.delete(extName);
      reject(new Error(`Failed to load extension "${extName}" frontend`));
    };
    document.head.appendChild(script);
  });

  loadingPromises.set(extName, promise);
  return promise;
}

/**
 * Create a React.lazy component that loads from an extension module.
 */
export function lazyExtensionComponent(
  extName: string,
  componentName: string,
  basePath?: string
): React.LazyExoticComponent<React.ComponentType<Record<string, unknown>>> {
  return React.lazy(() =>
    loadExtensionModule(extName, basePath).then((mod) => {
      const Component = mod[componentName];
      if (!Component) {
        throw new Error(
          `Component "${componentName}" not found in extension "${extName}"`
        );
      }
      return { default: Component };
    })
  );
}

import React from "react";

// ExtensionModule 从扩展 IIFE 脚本加载的模块
export interface ExtensionModule {
  [componentName: string]: React.ComponentType<Record<string, unknown>>;
}

// 已加载的扩展模块缓存
const loadedModules: Map<string, ExtensionModule> = new Map();
const loadingPromises: Map<string, Promise<ExtensionModule>> = new Map();

/**
 * 加载扩展前端模块（IIFE 格式，注册到 window.__OPSKAT_EXT_{name}__）
 * 同一扩展只加载一次，后续调用返回缓存。
 */
export function loadExtensionModule(extName: string): Promise<ExtensionModule> {
  const cached = loadedModules.get(extName);
  if (cached) return Promise.resolve(cached);

  const loading = loadingPromises.get(extName);
  if (loading) return loading;

  const promise = new Promise<ExtensionModule>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = `/extensions/${extName}/frontend/index.js`;
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
        reject(new Error(`Extension "${extName}" loaded but window.${globalName} not found`));
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
 * 创建 React.lazy 组件，从扩展模块中加载指定组件
 */
export function lazyExtensionComponent(
  extName: string,
  componentName: string
): React.LazyExoticComponent<React.ComponentType<Record<string, unknown>>> {
  return React.lazy(() =>
    loadExtensionModule(extName).then((mod) => {
      const Component = mod[componentName];
      if (!Component) {
        throw new Error(`Component "${componentName}" not found in extension "${extName}"`);
      }
      return { default: Component };
    })
  );
}

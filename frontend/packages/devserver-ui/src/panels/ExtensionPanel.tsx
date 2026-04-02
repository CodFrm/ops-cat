// frontend/packages/devserver-ui/src/panels/ExtensionPanel.tsx
import React, { useEffect, useState, useRef, useCallback } from "react";
import ReactDOM from "react-dom/client";
import * as ui from "@opskat/ui";
import type { ComponentType } from "react";
import { useLocale } from "../hooks/useLocale";
import { useLanguage } from "../hooks/useLanguage";

interface ExtPage {
  id: string;
  slot?: string;
  i18n: { name: string };
  component: string;
}

interface ExtManifest {
  name: string;
  frontend?: {
    entry: string;
    styles: string;
    pages: ExtPage[];
  };
}

declare global {
  interface Window {
    __OPSKAT_EXT__?: Record<string, unknown>;
  }
}

// i18n stub with mutable translations — updated when locale data loads.
const i18nTranslations: Record<string, string> = {};
const i18nStub = {
  t: (key: string) => i18nTranslations[key] ?? key,
  language: navigator.language || "zh-CN",
  changeLanguage: () => Promise.resolve(),
};

function injectDevServerAPI(): void {
  if (window.__OPSKAT_EXT__) return;

  window.__OPSKAT_EXT__ = {
    React,
    ReactDOM,
    i18n: i18nStub,
    ui,
    api: {
      async callTool(_extName: string, tool: string, args: unknown) {
        const res = await fetch(`/api/tool/${tool}`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(args),
        });
        if (!res.ok) throw new Error(await res.text());
        return res.json();
      },
      async executeAction(
        _extName: string,
        action: string,
        args: unknown,
        onEvent?: (e: { eventType: string; data: unknown }) => void
      ) {
        let ws: WebSocket | null = null;
        if (onEvent) {
          const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
          ws = new WebSocket(`${proto}//${window.location.host}/ws/events`);
          ws.onmessage = (msg) => {
            try {
              const data = JSON.parse(msg.data);
              if (data.type === "event") {
                onEvent({ eventType: data.eventType, data: data.data });
              }
            } catch {
              /* ignore */
            }
          };
          await new Promise<void>((resolve) => {
            ws!.onopen = () => resolve();
            setTimeout(resolve, 500);
          });
        }

        try {
          const res = await fetch(`/api/action/${action}`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(args),
          });
          if (!res.ok) throw new Error(await res.text());
          return res.json();
        } finally {
          ws?.close();
        }
      },
    },
  };
}

export function ExtensionPanel() {
  const [manifest, setManifest] = useState<ExtManifest | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [components, setComponents] = useState<Record<string, ComponentType<Record<string, unknown>>>>({});
  const [activePageId, setActivePageId] = useState<string | null>(null);
  const [moduleLoaded, setModuleLoaded] = useState(false);
  const [loadingModule, setLoadingModule] = useState(false);
  const injected = useRef(false);
  const { language } = useLanguage();
  const { t, translations } = useLocale(manifest?.name);

  // Sync locale data and language into the mutable i18n stub so extension frontend can use it.
  useEffect(() => {
    Object.keys(i18nTranslations).forEach((k) => delete i18nTranslations[k]);
    Object.assign(i18nTranslations, translations);
    i18nStub.language = language;
    window.dispatchEvent(new CustomEvent("opskat-language-change"));
  }, [translations, language]);

  // Step 1: Load manifest only (lightweight)
  useEffect(() => {
    (async () => {
      try {
        const res = await fetch("/api/manifest");
        if (!res.ok) throw new Error("Failed to fetch manifest");
        const m: ExtManifest = await res.json();
        setManifest(m);

        if (!m.frontend) {
          setError("Extension has no frontend definition");
          return;
        }

        // Default to first page but don't load module yet
        if (m.frontend.pages.length > 0) {
          setActivePageId(m.frontend.pages[0].id);
        }
      } catch (err) {
        setError(String(err));
      }
    })();
  }, []);

  // Step 2: Load module on demand when user clicks "Load"
  const loadModule = useCallback(async () => {
    if (!manifest?.frontend || moduleLoaded || loadingModule) return;
    setLoadingModule(true);
    setError(null);

    try {
      if (!injected.current) {
        injectDevServerAPI();
        injected.current = true;
      }

      const m = manifest;
      if (m.frontend!.styles) {
        const link = document.createElement("link");
        link.rel = "stylesheet";
        link.href = `/extensions/${m.name}/${m.frontend!.styles}`;
        document.head.appendChild(link);
      }

      const mod = await import(/* @vite-ignore */ `/extensions/${m.name}/${m.frontend!.entry}`);

      const loaded: Record<string, ComponentType<Record<string, unknown>>> = {};
      for (const page of m.frontend!.pages) {
        if (mod[page.component]) {
          loaded[page.component] = mod[page.component];
        }
      }
      setComponents(loaded);
      setModuleLoaded(true);
    } catch (err) {
      setError(String(err));
    } finally {
      setLoadingModule(false);
    }
  }, [manifest, moduleLoaded, loadingModule]);

  if (error) {
    return (
      <div className="p-4">
        <p className="text-red-500">{error}</p>
      </div>
    );
  }

  if (!manifest) {
    return <div className="p-4 text-muted-foreground">Loading extension manifest...</div>;
  }

  const pages = manifest.frontend?.pages ?? [];
  const activePage = pages.find((p) => p.id === activePageId);
  const Component = activePage && moduleLoaded ? components[activePage.component] : null;

  return (
    <div className="h-full flex flex-col">
      {/* Page tabs */}
      <div className="flex items-center gap-2 border-b pb-2 mb-4">
        {pages.map((page) => (
          <button
            key={page.id}
            onClick={() => setActivePageId(page.id)}
            className={`px-3 py-1 rounded text-sm ${
              activePageId === page.id ? "bg-primary text-primary-foreground" : "hover:bg-muted"
            }`}
          >
            {t(page.i18n?.name) || page.id}
            {page.slot && <span className="ml-1 text-xs opacity-60">({page.slot})</span>}
          </button>
        ))}

        {!moduleLoaded && (
          <button
            onClick={loadModule}
            disabled={loadingModule || !activePageId}
            className="ml-auto px-4 py-1 rounded text-sm bg-primary text-primary-foreground disabled:opacity-50"
          >
            {loadingModule ? "Loading..." : "Load Extension"}
          </button>
        )}
      </div>

      {/* Page content */}
      <div className="flex-1 overflow-auto">
        {!moduleLoaded && !loadingModule && (
          <div className="text-muted-foreground text-sm">
            Select a page and click &ldquo;Load Extension&rdquo; to render the extension frontend.
          </div>
        )}
        {moduleLoaded && !activePage && <div className="text-muted-foreground">No pages defined in manifest</div>}
        {activePage && moduleLoaded && !Component && (
          <div className="text-red-500">Component &ldquo;{activePage.component}&rdquo; not found in module exports</div>
        )}
        {activePage && Component && <Component assetId={0} />}
      </div>
    </div>
  );
}

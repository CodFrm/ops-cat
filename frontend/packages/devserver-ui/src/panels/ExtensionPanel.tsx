// frontend/packages/devserver-ui/src/panels/ExtensionPanel.tsx
import React, { useEffect, useState, useRef } from "react";
import type { ComponentType } from "react";

interface ExtManifest {
  name: string;
  frontend?: {
    entry: string;
    styles: string;
    pages: { id: string; slot?: string; component: string }[];
  };
}

function injectDevServerAPI(): void {
  if ((window as any).__OPSKAT_EXT__) return;

  (window as any).__OPSKAT_EXT__ = {
    React,
    ReactDOM: null,
    i18n: null,
    ui: {},
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
  const [Component, setComponent] = useState<ComponentType<any> | null>(null);
  const injected = useRef(false);

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

        if (!injected.current) {
          injectDevServerAPI();
          injected.current = true;
        }

        if (m.frontend.styles) {
          const link = document.createElement("link");
          link.rel = "stylesheet";
          link.href = `/extensions/${m.name}/${m.frontend.styles}`;
          document.head.appendChild(link);
        }

        const mod = await import(/* @vite-ignore */ `/extensions/${m.name}/${m.frontend.entry}`);

        const page = m.frontend.pages.find((p) => p.slot === "asset.connect") || m.frontend.pages[0];
        if (page && mod[page.component]) {
          setComponent(() => mod[page.component]);
        } else {
          setError(`Component "${page?.component}" not found in module exports`);
        }
      } catch (err) {
        setError(String(err));
      }
    })();
  }, []);

  if (error) {
    return (
      <div className="p-4">
        <p className="text-red-500">{error}</p>
      </div>
    );
  }

  if (!manifest) {
    return <div className="p-4 text-gray-500">Loading extension manifest...</div>;
  }

  if (!Component) {
    return <div className="p-4 text-gray-500">Loading extension frontend...</div>;
  }

  return (
    <div className="h-full">
      <Component assetId={0} />
    </div>
  );
}

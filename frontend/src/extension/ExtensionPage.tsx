// frontend/src/extension/ExtensionPage.tsx
import React, { useEffect, useState } from "react";
import { useExtensionStore } from "./store";
import { loadExtension } from "./loader";
import { loadExtensionLocales } from "./i18n";
import type { LoadedExtension } from "./types";

interface ExtensionPageProps {
  extensionName: string;
  pageId: string;
  assetId?: number;
}

class ExtensionErrorBoundary extends React.Component<
  { extensionName: string; children: React.ReactNode },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex items-center justify-center h-full">
          <div className="text-center">
            <p className="text-destructive font-medium">Extension &ldquo;{this.props.extensionName}&rdquo; crashed</p>
            <p className="text-sm text-muted-foreground mt-1">{this.state.error.message}</p>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

/** Grace period (ms) before showing "not registered" error. */
const NOT_REGISTERED_TIMEOUT = 5000;

interface ExtensionPageError {
  extensionName: string;
  pageId: string;
  kind: "not_registered" | "load";
  message: string;
}

export function ExtensionPage({ extensionName, pageId, assetId }: ExtensionPageProps) {
  const ready = useExtensionStore((s) => s.ready);
  const entry = useExtensionStore((s) => s.extensions[extensionName]);
  const setLoaded = useExtensionStore((s) => s.setLoaded);
  const [loadedLocal, setLoadedLocal] = useState<{ extensionName: string; loaded: LoadedExtension } | null>(null);
  const loaded = entry?.loaded ?? (loadedLocal?.extensionName === extensionName ? loadedLocal.loaded : null);
  const [error, setError] = useState<ExtensionPageError | null>(null);
  const currentError =
    error?.extensionName === extensionName && error.pageId === pageId && (error.kind !== "not_registered" || !entry)
      ? error.message
      : null;

  useEffect(() => {
    if (!ready) return;

    if (!entry) {
      // Extension not registered yet — wait for ext:reload to bring it in.
      // Only show error after a grace period.
      const timer = setTimeout(() => {
        setError({
          extensionName,
          pageId,
          kind: "not_registered",
          message: `Extension "${extensionName}" not registered`,
        });
      }, NOT_REGISTERED_TIMEOUT);
      return () => clearTimeout(timer);
    }

    if (entry.loaded) return;

    let cancelled = false;
    (async () => {
      try {
        await loadExtensionLocales(extensionName);
        const result = await loadExtension(extensionName, entry.manifest);
        if (!cancelled) {
          setLoaded(extensionName, result);
          setLoadedLocal({ extensionName, loaded: result });
        }
      } catch (err) {
        if (!cancelled) setError({ extensionName, pageId, kind: "load", message: String(err) });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [ready, entry, extensionName, pageId, setLoaded]);

  if (currentError) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-destructive">{currentError}</p>
      </div>
    );
  }

  if (!loaded) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-muted-foreground">Loading extension...</p>
      </div>
    );
  }

  const page = loaded.manifest.frontend?.pages.find((p) => p.id === pageId);
  if (!page) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-destructive">Page &ldquo;{pageId}&rdquo; not found in extension</p>
      </div>
    );
  }

  const Component = loaded.components[page.component];
  if (!Component) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-destructive">Component &ldquo;{page.component}&rdquo; not exported by extension</p>
      </div>
    );
  }

  return (
    <ExtensionErrorBoundary extensionName={extensionName}>
      <Component assetId={assetId} />
    </ExtensionErrorBoundary>
  );
}

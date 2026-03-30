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

export function ExtensionPage({ extensionName, pageId, assetId }: ExtensionPageProps) {
  const extensions = useExtensionStore((s) => s.extensions);
  const setLoaded = useExtensionStore((s) => s.setLoaded);
  const [loaded, setLoadedLocal] = useState<LoadedExtension | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const entry = extensions[extensionName];
    if (!entry) {
      setError(`Extension "${extensionName}" not registered`);
      return;
    }

    if (entry.loaded) {
      setLoadedLocal(entry.loaded);
      return;
    }

    let cancelled = false;
    (async () => {
      try {
        await loadExtensionLocales(extensionName);
        const result = await loadExtension(extensionName, entry.manifest);
        if (!cancelled) {
          setLoaded(extensionName, result);
          setLoadedLocal(result);
        }
      } catch (err) {
        if (!cancelled) setError(String(err));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [extensionName, pageId]);

  if (error) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-destructive">{error}</p>
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

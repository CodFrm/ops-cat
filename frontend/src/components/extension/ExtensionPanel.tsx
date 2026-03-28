import { Suspense, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import React from "react";
import { useExtensionStore } from "@/stores/extensionStore";
import { loadExtensionModule } from "@/lib/extensionLoader";

interface ExtensionPanelProps {
  assetType: string;
  assetId: number;
  pageId?: string;
}

export function ExtensionPanel({ assetType, assetId, pageId }: ExtensionPanelProps) {
  const { t } = useTranslation();
  const getExtensionForAssetType = useExtensionStore((s) => s.getExtensionForAssetType);
  const ext = getExtensionForAssetType(assetType);

  const [ExtComponent, setExtComponent] = useState<React.ComponentType<Record<string, unknown>> | null>(null);

  useEffect(() => {
    if (!ext) return;
    const page = pageId ? ext.pages?.find((p) => p.id === pageId) : ext.pages?.[0];
    if (!page) return;
    loadExtensionModule(ext.name)
      .then((mod) => {
        const Comp = mod[page.component];
        if (Comp) setExtComponent(() => Comp);
      })
      .catch(() => {
        setExtComponent(null);
      });
  }, [ext, pageId]);

  if (!ext || !ExtComponent) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">{t("extension.notFound")}</div>
    );
  }

  return (
    <Suspense
      fallback={
        <div className="flex items-center justify-center h-full text-muted-foreground">{t("extension.loading")}</div>
      }
    >
      <ExtComponent assetId={assetId} assetType={assetType} />
    </Suspense>
  );
}

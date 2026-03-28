import { Suspense, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useExtensionStore } from "@/stores/extensionStore";
import { lazyExtensionComponent } from "@/lib/extensionLoader";

interface ExtensionPanelProps {
  assetType: string;
  assetId: number;
  pageId?: string;
}

export function ExtensionPanel({ assetType, assetId, pageId }: ExtensionPanelProps) {
  const { t } = useTranslation();
  const getExtensionForAssetType = useExtensionStore((s) => s.getExtensionForAssetType);
  const ext = getExtensionForAssetType(assetType);

  const LazyComponent = useMemo(() => {
    if (!ext) return null;
    const page = pageId ? ext.pages?.find((p) => p.id === pageId) : ext.pages?.[0];
    if (!page) return null;
    return lazyExtensionComponent(ext.name, page.component);
  }, [ext, pageId]);

  if (!ext || !LazyComponent) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        {t("extension.notFound")}
      </div>
    );
  }

  return (
    <Suspense
      fallback={
        <div className="flex items-center justify-center h-full text-muted-foreground">
          {t("extension.loading")}
        </div>
      }
    >
      <LazyComponent assetId={assetId} assetType={assetType} />
    </Suspense>
  );
}

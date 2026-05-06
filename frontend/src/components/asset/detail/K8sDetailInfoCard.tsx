import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

interface K8sConfig {
  kubeconfig?: string;
  namespace?: string;
  context?: string;
}

export function K8sDetailInfoCard({ asset, sshTunnelName }: DetailInfoCardProps) {
  const { t } = useTranslation();

  let cfg: K8sConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;
  const tunnelName = sshTunnelName(asset.sshTunnelId);

  return (
    <div className="rounded-xl border bg-card p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">K8S</h3>
      <div className="grid grid-cols-2 gap-4 text-sm">
        <InfoItem
          label={t("asset.k8sKubeconfig")}
          value={cfg.kubeconfig ? t("asset.k8sKubeconfigProvided") : ""}
          mono
        />
        {cfg.namespace && <InfoItem label={t("asset.k8sNamespace")} value={cfg.namespace} mono />}
        {cfg.context && <InfoItem label={t("asset.k8sContext")} value={cfg.context} mono />}
      </div>
      {tunnelName && (
        <div className="mt-3 pt-3 border-t text-sm">
          <InfoItem label={t("asset.sshTunnel")} value={tunnelName} mono />
        </div>
      )}
    </div>
  );
}

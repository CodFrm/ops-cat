import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

interface RedisConfig {
  host: string;
  port: number;
  username?: string;
  password?: string;
  database?: number;
  tls?: boolean;
  ssh_asset_id?: number;
}

export function RedisDetailInfoCard({ asset, sshTunnelName }: DetailInfoCardProps) {
  const { t } = useTranslation();

  let cfg: RedisConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;

  return (
    <div className="rounded-xl border bg-card p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">Redis</h3>
      <div className="grid grid-cols-2 gap-4 text-sm">
        <InfoItem label={t("asset.host")} value={`${cfg.host}:${cfg.port}`} mono />
        {cfg.username && <InfoItem label={t("asset.username")} value={cfg.username} mono />}
        {cfg.password && <InfoItem label={t("asset.password")} value={"\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF"} />}
        <InfoItem label={t("asset.redisDatabase")} value={String(cfg.database || 0)} mono />
        {cfg.tls && <InfoItem label={t("asset.tls")} value={"\u2713"} />}
      </div>
      {sshTunnelName(cfg.ssh_asset_id) && (
        <div className="mt-3 pt-3 border-t text-sm">
          <InfoItem label={t("asset.sshTunnel")} value={sshTunnelName(cfg.ssh_asset_id)!} mono />
        </div>
      )}
    </div>
  );
}

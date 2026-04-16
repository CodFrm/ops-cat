import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

interface DatabaseConfig {
  driver: string;
  host: string;
  port: number;
  username: string;
  password?: string;
  database?: string;
  ssl_mode?: string;
  tls?: boolean;
  params?: string;
  read_only?: boolean;
  ssh_asset_id?: number;
}

export function DatabaseDetailInfoCard({ asset, sshTunnelName }: DetailInfoCardProps) {
  const { t } = useTranslation();

  let cfg: DatabaseConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;

  return (
    <div className="rounded-xl border bg-card p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
        {t("asset.typeDatabase")}
      </h3>
      <div className="grid grid-cols-2 gap-4 text-sm">
        <InfoItem label={t("asset.driver")} value={cfg.driver === "postgresql" ? "PostgreSQL" : "MySQL"} />
        <InfoItem label={t("asset.host")} value={`${cfg.host}:${cfg.port}`} mono />
        <InfoItem label={t("asset.username")} value={cfg.username} mono />
        {cfg.database && <InfoItem label={t("asset.database")} value={cfg.database} mono />}
        {cfg.password && <InfoItem label={t("asset.password")} value={"\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF"} />}
        {cfg.ssl_mode && cfg.ssl_mode !== "disable" && <InfoItem label={t("asset.sslMode")} value={cfg.ssl_mode} />}
        {cfg.tls && <InfoItem label="TLS" value={"\u2713"} />}
        {cfg.read_only && <InfoItem label={t("asset.readOnly")} value={"\u2713"} />}
        {cfg.params && <InfoItem label={t("asset.params")} value={cfg.params} mono />}
      </div>
      {sshTunnelName(cfg.ssh_asset_id) && (
        <div className="mt-3 pt-3 border-t text-sm">
          <InfoItem label={t("asset.sshTunnel")} value={sshTunnelName(cfg.ssh_asset_id)!} mono />
        </div>
      )}
    </div>
  );
}

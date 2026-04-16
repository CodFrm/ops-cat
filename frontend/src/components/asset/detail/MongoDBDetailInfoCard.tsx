import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

interface MongoDBConfig {
  connection_uri?: string;
  host?: string;
  port?: number;
  replica_set?: string;
  username?: string;
  password?: string;
  credential_id?: number;
  database?: string;
  auth_source?: string;
  tls?: boolean;
  ssh_asset_id?: number;
}

export function MongoDBDetailInfoCard({ asset, sshTunnelName }: DetailInfoCardProps) {
  const { t } = useTranslation();

  let cfg: MongoDBConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;

  return (
    <div className="rounded-xl border bg-card p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">MongoDB</h3>
      <div className="grid grid-cols-2 gap-4 text-sm">
        {cfg.connection_uri ? (
          <InfoItem label={t("asset.mongoUri")} value={cfg.connection_uri} mono />
        ) : (
          <InfoItem label={t("asset.host")} value={`${cfg.host}:${cfg.port}`} mono />
        )}
        {cfg.username && <InfoItem label={t("asset.username")} value={cfg.username} mono />}
        {cfg.password && <InfoItem label={t("asset.password")} value={"\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF"} />}
        {cfg.database && <InfoItem label={t("asset.mongoDefaultDatabase")} value={cfg.database} mono />}
        {cfg.auth_source && <InfoItem label={t("asset.mongoAuthSource")} value={cfg.auth_source} mono />}
        {cfg.replica_set && <InfoItem label={t("asset.mongoReplicaSet")} value={cfg.replica_set} mono />}
        {cfg.tls && <InfoItem label="TLS" value={"\u2713"} />}
      </div>
      {sshTunnelName(cfg.ssh_asset_id) && (
        <div className="mt-3 pt-3 border-t text-sm">
          <InfoItem label={t("asset.sshTunnel")} value={sshTunnelName(cfg.ssh_asset_id)!} mono />
        </div>
      )}
    </div>
  );
}

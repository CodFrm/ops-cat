import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { DetailGrid, DetailSection, InfoItem, TunnelInfo } from "./InfoItem";
import { ENABLED_VALUE, MASKED_SECRET, parseDetailConfig } from "./utils";

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

  const cfg = parseDetailConfig<MongoDBConfig>(asset.Config);
  if (!cfg) return null;
  const tunnelName = sshTunnelName(cfg.ssh_asset_id);

  return (
    <DetailSection title="MongoDB">
      <DetailGrid>
        {cfg.connection_uri ? (
          <InfoItem label={t("asset.mongoUri")} value={cfg.connection_uri} mono />
        ) : (
          <InfoItem label={t("asset.host")} value={`${cfg.host}:${cfg.port}`} mono />
        )}
        {cfg.username && <InfoItem label={t("asset.username")} value={cfg.username} mono />}
        {cfg.password && <InfoItem label={t("asset.password")} value={MASKED_SECRET} />}
        {cfg.database && <InfoItem label={t("asset.mongoDefaultDatabase")} value={cfg.database} mono />}
        {cfg.auth_source && <InfoItem label={t("asset.mongoAuthSource")} value={cfg.auth_source} mono />}
        {cfg.replica_set && <InfoItem label={t("asset.mongoReplicaSet")} value={cfg.replica_set} mono />}
        {cfg.tls && <InfoItem label="TLS" value={ENABLED_VALUE} />}
      </DetailGrid>
      {tunnelName && <TunnelInfo label={t("asset.sshTunnel")} name={tunnelName} />}
    </DetailSection>
  );
}

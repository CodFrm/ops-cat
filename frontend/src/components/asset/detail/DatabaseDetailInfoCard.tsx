import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { DetailGrid, DetailSection, InfoItem, TunnelInfo } from "./InfoItem";
import { ENABLED_VALUE, MASKED_SECRET, parseDetailConfig } from "./utils";

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

  const cfg = parseDetailConfig<DatabaseConfig>(asset.Config);
  if (!cfg) return null;
  const tunnelName = sshTunnelName(cfg.ssh_asset_id);

  return (
    <DetailSection title={t("asset.typeDatabase")}>
      <DetailGrid>
        <InfoItem label={t("asset.driver")} value={cfg.driver === "postgresql" ? "PostgreSQL" : "MySQL"} />
        <InfoItem label={t("asset.host")} value={`${cfg.host}:${cfg.port}`} mono />
        <InfoItem label={t("asset.username")} value={cfg.username} mono />
        {cfg.database && <InfoItem label={t("asset.database")} value={cfg.database} mono />}
        {cfg.password && <InfoItem label={t("asset.password")} value={MASKED_SECRET} />}
        {cfg.ssl_mode && cfg.ssl_mode !== "disable" && <InfoItem label={t("asset.sslMode")} value={cfg.ssl_mode} />}
        {cfg.tls && <InfoItem label="TLS" value={ENABLED_VALUE} />}
        {cfg.read_only && <InfoItem label={t("asset.readOnly")} value={ENABLED_VALUE} />}
        {cfg.params && <InfoItem label={t("asset.params")} value={cfg.params} mono />}
      </DetailGrid>
      {tunnelName && <TunnelInfo label={t("asset.sshTunnel")} name={tunnelName} />}
    </DetailSection>
  );
}

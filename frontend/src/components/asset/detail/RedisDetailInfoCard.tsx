import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { DetailGrid, DetailSection, InfoItem, TunnelInfo } from "./InfoItem";
import { ENABLED_VALUE, MASKED_SECRET, parseDetailConfig } from "./utils";

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

  const cfg = parseDetailConfig<RedisConfig>(asset.Config);
  if (!cfg) return null;
  const tunnelName = sshTunnelName(cfg.ssh_asset_id);

  return (
    <DetailSection title="Redis">
      <DetailGrid>
        <InfoItem label={t("asset.host")} value={`${cfg.host}:${cfg.port}`} mono />
        {cfg.username && <InfoItem label={t("asset.username")} value={cfg.username} mono />}
        {cfg.password && <InfoItem label={t("asset.password")} value={MASKED_SECRET} />}
        <InfoItem label={t("asset.redisDatabase")} value={String(cfg.database || 0)} mono />
        {cfg.tls && <InfoItem label={t("asset.tls")} value={ENABLED_VALUE} />}
      </DetailGrid>
      {tunnelName && <TunnelInfo label={t("asset.sshTunnel")} name={tunnelName} />}
    </DetailSection>
  );
}

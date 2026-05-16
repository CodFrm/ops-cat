import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { DetailGrid, DetailSection, InfoItem, TunnelInfo } from "./InfoItem";
import { ENABLED_VALUE, MASKED_SECRET, parseDetailConfig } from "./utils";

interface KafkaConfig {
  brokers?: string[];
  client_id?: string;
  sasl_mechanism?: string;
  username?: string;
  password?: string;
  credential_id?: number;
  tls?: boolean;
  ssh_asset_id?: number;
  request_timeout_seconds?: number;
  message_preview_bytes?: number;
  message_fetch_limit?: number;
}

export function KafkaDetailInfoCard({ asset, sshTunnelName }: DetailInfoCardProps) {
  const { t } = useTranslation();

  const cfg = parseDetailConfig<KafkaConfig>(asset.Config);
  if (!cfg) return null;

  const tunnelName = sshTunnelName(asset.sshTunnelId || cfg.ssh_asset_id);
  const sasl = cfg.sasl_mechanism || "none";

  return (
    <DetailSection title="Kafka">
      <DetailGrid>
        <InfoItem label={t("asset.kafkaBrokers")} value={(cfg.brokers || []).join(", ")} mono />
        <InfoItem label={t("asset.kafkaClientId")} value={cfg.client_id || "opskat"} mono />
        <InfoItem label={t("asset.kafkaSaslMechanism")} value={sasl.toUpperCase()} mono />
        {cfg.username && <InfoItem label={t("asset.username")} value={cfg.username} mono />}
        {(cfg.password || cfg.credential_id) && <InfoItem label={t("asset.password")} value={MASKED_SECRET} />}
        {cfg.tls && <InfoItem label={t("asset.tls")} value={ENABLED_VALUE} />}
        {cfg.request_timeout_seconds ? (
          <InfoItem label={t("asset.kafkaRequestTimeout")} value={String(cfg.request_timeout_seconds)} mono />
        ) : null}
        {cfg.message_fetch_limit ? (
          <InfoItem label={t("asset.kafkaMessageFetchLimit")} value={String(cfg.message_fetch_limit)} mono />
        ) : null}
        {cfg.message_preview_bytes ? (
          <InfoItem label={t("asset.kafkaMessagePreviewBytes")} value={String(cfg.message_preview_bytes)} mono />
        ) : null}
      </DetailGrid>
      {tunnelName && <TunnelInfo label={t("asset.sshTunnel")} name={tunnelName} />}
    </DetailSection>
  );
}

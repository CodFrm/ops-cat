import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

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

  let cfg: KafkaConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;

  const tunnelName = sshTunnelName(asset.sshTunnelId || cfg.ssh_asset_id);
  const sasl = cfg.sasl_mechanism || "none";

  return (
    <div className="rounded-xl border bg-card p-4">
      <h3 className="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Kafka</h3>
      <div className="grid grid-cols-2 gap-4 text-sm">
        <InfoItem label={t("asset.kafkaBrokers")} value={(cfg.brokers || []).join(", ")} mono />
        <InfoItem label={t("asset.kafkaClientId")} value={cfg.client_id || "opskat"} mono />
        <InfoItem label={t("asset.kafkaSaslMechanism")} value={sasl.toUpperCase()} mono />
        {cfg.username && <InfoItem label={t("asset.username")} value={cfg.username} mono />}
        {(cfg.password || cfg.credential_id) && <InfoItem label={t("asset.password")} value="******" />}
        {cfg.tls && <InfoItem label={t("asset.tls")} value="yes" />}
        {cfg.request_timeout_seconds ? (
          <InfoItem label={t("asset.kafkaRequestTimeout")} value={String(cfg.request_timeout_seconds)} mono />
        ) : null}
        {cfg.message_fetch_limit ? (
          <InfoItem label={t("asset.kafkaMessageFetchLimit")} value={String(cfg.message_fetch_limit)} mono />
        ) : null}
        {cfg.message_preview_bytes ? (
          <InfoItem label={t("asset.kafkaMessagePreviewBytes")} value={String(cfg.message_preview_bytes)} mono />
        ) : null}
      </div>
      {tunnelName && (
        <div className="mt-3 border-t pt-3 text-sm">
          <InfoItem label={t("asset.sshTunnel")} value={tunnelName} mono />
        </div>
      )}
    </div>
  );
}

import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

interface SerialConfig {
  port_path?: string;
  baud_rate?: number;
  data_bits?: number;
  stop_bits?: string;
  parity?: string;
  flow_control?: string;
}

export function SerialDetailInfoCard({ asset }: DetailInfoCardProps) {
  const { t } = useTranslation();

  let cfg: SerialConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;

  return (
    <div className="rounded-xl border bg-card p-4">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
        {t("asset.serialTitle")}
      </h3>
      <div className="grid grid-cols-2 gap-4 text-sm">
        {cfg.port_path && <InfoItem label={t("asset.serialPortPath")} value={cfg.port_path} mono />}
        {cfg.baud_rate && <InfoItem label={t("asset.serialBaudRate")} value={String(cfg.baud_rate)} mono />}
        {cfg.data_bits && <InfoItem label={t("asset.serialDataBits")} value={String(cfg.data_bits)} mono />}
        {cfg.stop_bits && <InfoItem label={t("asset.serialStopBits")} value={cfg.stop_bits} mono />}
        {cfg.parity && <InfoItem label={t("asset.serialParity")} value={cfg.parity} mono />}
        {cfg.flow_control && cfg.flow_control !== "none" && (
          <InfoItem label={t("asset.serialFlowControl")} value={cfg.flow_control} mono />
        )}
      </div>
    </div>
  );
}

import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { DetailGrid, DetailSection, InfoItem } from "./InfoItem";
import { parseDetailConfig } from "./utils";

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

  const cfg = parseDetailConfig<SerialConfig>(asset.Config);
  if (!cfg) return null;

  return (
    <DetailSection title={t("asset.serialTitle")}>
      <DetailGrid>
        {cfg.port_path && <InfoItem label={t("asset.serialPortPath")} value={cfg.port_path} mono />}
        {cfg.baud_rate && <InfoItem label={t("asset.serialBaudRate")} value={String(cfg.baud_rate)} mono />}
        {cfg.data_bits && <InfoItem label={t("asset.serialDataBits")} value={String(cfg.data_bits)} mono />}
        {cfg.stop_bits && <InfoItem label={t("asset.serialStopBits")} value={cfg.stop_bits} mono />}
        {cfg.parity && <InfoItem label={t("asset.serialParity")} value={cfg.parity} mono />}
        {cfg.flow_control && cfg.flow_control !== "none" && (
          <InfoItem label={t("asset.serialFlowControl")} value={cfg.flow_control} mono />
        )}
      </DetailGrid>
    </DetailSection>
  );
}

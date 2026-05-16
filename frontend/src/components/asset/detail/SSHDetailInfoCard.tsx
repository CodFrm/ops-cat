import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { DetailGrid, DetailSection, InfoItem } from "./InfoItem";
import { parseDetailConfig } from "./utils";

interface SSHConfig {
  host: string;
  port: number;
  username: string;
  auth_type: string;
  password?: string;
  credential_id?: number;
  private_keys?: string[];
  jump_host_id?: number;
  proxy?: {
    type: string;
    host: string;
    port: number;
    username?: string;
    password?: string;
  } | null;
}

export function SSHDetailInfoCard({ asset, sshTunnelName }: DetailInfoCardProps) {
  const { t } = useTranslation();

  const cfg = parseDetailConfig<SSHConfig>(asset.Config);
  if (!cfg) return null;

  const jumpHostName = sshTunnelName(cfg.jump_host_id);

  return (
    <>
      {/* SSH Connection Info */}
      <DetailSection title="SSH Connection">
        <DetailGrid>
          <InfoItem label={t("asset.host")} value={cfg.host} mono />
          <InfoItem label={t("asset.port")} value={String(cfg.port)} mono />
          <InfoItem label={t("asset.username")} value={cfg.username} mono />
          <InfoItem
            label={t("asset.authType")}
            value={
              cfg.auth_type === "password"
                ? t("asset.authPassword") + (cfg.password ? " \u25CF" : "")
                : cfg.auth_type === "key"
                  ? t("asset.authKey") +
                    (cfg.credential_id
                      ? ` (${t("asset.keySourceManaged")})`
                      : cfg.private_keys?.length
                        ? ` (${t("asset.keySourceFile")})`
                        : "")
                  : cfg.auth_type
            }
          />
        </DetailGrid>
      </DetailSection>

      {/* SSH Private Keys */}
      {cfg.private_keys && cfg.private_keys.length > 0 && (
        <DetailSection title={t("asset.privateKeys")}>
          <div className="flex flex-col gap-1">
            {cfg.private_keys.map((key, i) => (
              <p key={i} className="text-sm font-mono text-muted-foreground">
                {key}
              </p>
            ))}
          </div>
        </DetailSection>
      )}

      {/* SSH Jump Host */}
      {jumpHostName && (
        <DetailSection title={t("asset.jumpHost")}>
          <p className="text-sm font-mono">{jumpHostName}</p>
        </DetailSection>
      )}

      {/* SSH Proxy */}
      {cfg.proxy && (
        <DetailSection title={t("asset.proxy")}>
          <DetailGrid>
            <InfoItem label={t("asset.proxyType")} value={cfg.proxy.type.toUpperCase()} />
            <InfoItem label={t("asset.proxyHost")} value={`${cfg.proxy.host}:${cfg.proxy.port}`} mono />
            {cfg.proxy.username && <InfoItem label={t("asset.proxyUsername")} value={cfg.proxy.username} />}
          </DetailGrid>
        </DetailSection>
      )}
    </>
  );
}

import { useTranslation } from "react-i18next";
import type { DetailInfoCardProps } from "@/lib/assetTypes/types";
import { InfoItem } from "./InfoItem";

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

  let cfg: SSHConfig | null = null;
  try {
    cfg = JSON.parse(asset.Config || "{}");
  } catch {
    /* ignore */
  }
  if (!cfg) return null;

  const jumpHostName = sshTunnelName(cfg.jump_host_id);

  return (
    <>
      {/* SSH Connection Info */}
      <div className="rounded-xl border bg-card p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">SSH Connection</h3>
        <div className="grid grid-cols-2 gap-4 text-sm">
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
        </div>
      </div>

      {/* SSH Private Keys */}
      {cfg.private_keys && cfg.private_keys.length > 0 && (
        <div className="rounded-xl border bg-card p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            {t("asset.privateKeys")}
          </h3>
          <div className="space-y-1">
            {cfg.private_keys.map((key, i) => (
              <p key={i} className="text-sm font-mono text-muted-foreground">
                {key}
              </p>
            ))}
          </div>
        </div>
      )}

      {/* SSH Jump Host */}
      {jumpHostName && (
        <div className="rounded-xl border bg-card p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            {t("asset.jumpHost")}
          </h3>
          <p className="text-sm font-mono">{jumpHostName}</p>
        </div>
      )}

      {/* SSH Proxy */}
      {cfg.proxy && (
        <div className="rounded-xl border bg-card p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
            {t("asset.proxy")}
          </h3>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <InfoItem label={t("asset.proxyType")} value={cfg.proxy.type.toUpperCase()} />
            <InfoItem label={t("asset.proxyHost")} value={`${cfg.proxy.host}:${cfg.proxy.port}`} mono />
            {cfg.proxy.username && <InfoItem label={t("asset.proxyUsername")} value={cfg.proxy.username} />}
          </div>
        </div>
      )}
    </>
  );
}

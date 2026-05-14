import { useTranslation } from "react-i18next";
import { Input, Label, Switch } from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";
import { PasswordSourceField } from "@/components/asset/PasswordSourceField";
import { credential_entity } from "../../../wailsjs/go/models";

export interface RedisConfigSectionProps {
  host: string;
  setHost: (v: string) => void;
  port: number;
  setPort: (v: number) => void;
  username: string;
  setUsername: (v: string) => void;
  tls: boolean;
  setTls: (v: boolean) => void;
  tlsInsecure: boolean;
  setTlsInsecure: (v: boolean) => void;
  tlsServerName: string;
  setTlsServerName: (v: string) => void;
  tlsCAFile: string;
  setTlsCAFile: (v: string) => void;
  tlsCertFile: string;
  setTlsCertFile: (v: string) => void;
  tlsKeyFile: string;
  setTlsKeyFile: (v: string) => void;
  database: number;
  setDatabase: (v: number) => void;
  commandTimeoutSeconds: number;
  setCommandTimeoutSeconds: (v: number) => void;
  scanPageSize: number;
  setScanPageSize: (v: number) => void;
  keySeparator: string;
  setKeySeparator: (v: string) => void;
  sshTunnelId: number;
  setSshTunnelId: (v: number) => void;
  // Password fields
  password: string;
  setPassword: (v: string) => void;
  encryptedPassword: string;
  passwordSource: "inline" | "managed";
  setPasswordSource: (v: "inline" | "managed") => void;
  passwordCredentialId: number;
  setPasswordCredentialId: (v: number) => void;
  managedPasswords: credential_entity.Credential[];
  editAssetId?: number;
}

export function RedisConfigSection({
  host,
  setHost,
  port,
  setPort,
  username,
  setUsername,
  tls,
  setTls,
  tlsInsecure,
  setTlsInsecure,
  tlsServerName,
  setTlsServerName,
  tlsCAFile,
  setTlsCAFile,
  tlsCertFile,
  setTlsCertFile,
  tlsKeyFile,
  setTlsKeyFile,
  database,
  setDatabase,
  commandTimeoutSeconds,
  setCommandTimeoutSeconds,
  scanPageSize,
  setScanPageSize,
  keySeparator,
  setKeySeparator,
  sshTunnelId,
  setSshTunnelId,
  password,
  setPassword,
  encryptedPassword,
  passwordSource,
  setPasswordSource,
  passwordCredentialId,
  setPasswordCredentialId,
  managedPasswords,
  editAssetId,
}: RedisConfigSectionProps) {
  const { t } = useTranslation();

  return (
    <>
      {/* Connection & Auth (single visual block) */}
      <div className="grid gap-3 border rounded-lg p-3">
        {/* Host + Port (each labeled) */}
        <div className="grid grid-cols-[1fr_120px] gap-3">
          <div className="grid gap-2">
            <Label>{t("asset.host")}</Label>
            <Input value={host} onChange={(e) => setHost(e.target.value)} placeholder="example.com" />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.port")}</Label>
            <Input
              className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
              type="number"
              value={port || ""}
              placeholder="6379"
              onChange={(e) => setPort(Number(e.target.value))}
            />
          </div>
        </div>

        {/* Username */}
        <div className="grid gap-2">
          <Label>{t("asset.username")}</Label>
          <Input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder={t("asset.username") + " (" + t("asset.databasePlaceholder").split("\uFF08")[0] + ")"}
          />
        </div>

        {/* Password */}
        <PasswordSourceField
          source={passwordSource}
          onSourceChange={setPasswordSource}
          password={password}
          onPasswordChange={setPassword}
          credentialId={passwordCredentialId}
          onCredentialIdChange={setPasswordCredentialId}
          managedPasswords={managedPasswords}
          hasExistingPassword={!!encryptedPassword}
          editAssetId={editAssetId}
          onUsernameChange={setUsername}
        />
      </div>

      {/* TLS */}
      <div className="flex items-center justify-between">
        <Label>{t("asset.tls")}</Label>
        <Switch checked={tls} onCheckedChange={setTls} />
      </div>

      {tls && (
        <>
          <div className="flex items-center justify-between">
            <Label>{t("asset.redisTlsInsecure")}</Label>
            <Switch checked={tlsInsecure} onCheckedChange={setTlsInsecure} />
          </div>

          <div className="grid gap-2">
            <Label>{t("asset.redisTlsServerName")}</Label>
            <Input
              value={tlsServerName}
              onChange={(e) => setTlsServerName(e.target.value)}
              placeholder="redis.example.com"
            />
          </div>

          <div className="grid gap-2">
            <Label>{t("asset.redisTlsCAFile")}</Label>
            <Input value={tlsCAFile} onChange={(e) => setTlsCAFile(e.target.value)} placeholder="/path/to/ca.pem" />
          </div>

          <div className="grid gap-2">
            <Label>{t("asset.redisTlsCertFile")}</Label>
            <Input
              value={tlsCertFile}
              onChange={(e) => setTlsCertFile(e.target.value)}
              placeholder="/path/to/client.crt"
            />
          </div>

          <div className="grid gap-2">
            <Label>{t("asset.redisTlsKeyFile")}</Label>
            <Input
              value={tlsKeyFile}
              onChange={(e) => setTlsKeyFile(e.target.value)}
              placeholder="/path/to/client.key"
            />
          </div>
        </>
      )}

      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-2">
          <Label>{t("asset.redisDatabase")}</Label>
          <Input
            className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            min={0}
            value={database}
            onChange={(e) => setDatabase(Math.max(0, Number(e.target.value) || 0))}
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.redisCommandTimeout")}</Label>
          <Input
            className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            min={0}
            value={commandTimeoutSeconds}
            onChange={(e) => setCommandTimeoutSeconds(Math.max(0, Number(e.target.value) || 0))}
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="grid gap-2">
          <Label>{t("asset.redisScanPageSize")}</Label>
          <Input
            className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            min={0}
            value={scanPageSize}
            onChange={(e) => setScanPageSize(Math.max(0, Number(e.target.value) || 0))}
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.redisKeySeparator")}</Label>
          <Input value={keySeparator} onChange={(e) => setKeySeparator(e.target.value)} placeholder=":" />
        </div>
      </div>

      {/* SSH Tunnel */}
      <div className="grid gap-2">
        <Label>{t("asset.sshTunnel")}</Label>
        <AssetSelect
          value={sshTunnelId}
          onValueChange={setSshTunnelId}
          filterType="ssh"
          placeholder={t("asset.sshTunnelNone")}
        />
      </div>
    </>
  );
}

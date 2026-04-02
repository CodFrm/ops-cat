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
      {/* Host + Port */}
      <div className="grid gap-2">
        <Label>{t("asset.host")}</Label>
        <div className="flex gap-2">
          <Input className="flex-1" value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.1" />
          <Input
            className="w-[80px] shrink-0 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            value={port}
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
      />

      {/* TLS */}
      <div className="flex items-center justify-between">
        <Label>{t("asset.tls")}</Label>
        <Switch checked={tls} onCheckedChange={setTls} />
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

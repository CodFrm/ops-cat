import { useTranslation } from "react-i18next";
import { Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, Switch } from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";
import { PasswordSourceField } from "@/components/asset/PasswordSourceField";
import { credential_entity } from "../../../wailsjs/go/models";

export interface DatabaseConfigSectionProps {
  host: string;
  setHost: (v: string) => void;
  port: number;
  setPort: (v: number) => void;
  username: string;
  setUsername: (v: string) => void;
  driver: string;
  database: string;
  setDatabase: (v: string) => void;
  sslMode: string;
  setSslMode: (v: string) => void;
  tls: boolean;
  setTls: (v: boolean) => void;
  readOnly: boolean;
  setReadOnly: (v: boolean) => void;
  sshTunnelId: number;
  setSshTunnelId: (v: number) => void;
  params: string;
  setParams: (v: string) => void;
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

export function DatabaseConfigSection({
  host,
  setHost,
  port,
  setPort,
  username,
  setUsername,
  driver,
  database,
  setDatabase,
  sslMode,
  setSslMode,
  tls,
  setTls,
  readOnly,
  setReadOnly,
  sshTunnelId,
  setSshTunnelId,
  params,
  setParams,
  password,
  setPassword,
  encryptedPassword,
  passwordSource,
  setPasswordSource,
  passwordCredentialId,
  setPasswordCredentialId,
  managedPasswords,
  editAssetId,
}: DatabaseConfigSectionProps) {
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
              placeholder={driver === "postgresql" ? "5432" : "3306"}
              onChange={(e) => setPort(Number(e.target.value))}
            />
          </div>
        </div>

        {/* Username */}
        <div className="grid gap-2">
          <Label>{t("asset.username")}</Label>
          <Input value={username} onChange={(e) => setUsername(e.target.value)} />
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

      {/* Database name */}
      <div className="grid gap-2">
        <Label>{t("asset.database")}</Label>
        <Input
          value={database}
          onChange={(e) => setDatabase(e.target.value)}
          placeholder={t("asset.databasePlaceholder")}
        />
      </div>

      {/* SSL Mode (PostgreSQL only) */}
      {driver === "postgresql" && (
        <div className="grid gap-2">
          <Label>{t("asset.sslMode")}</Label>
          <Select value={sslMode} onValueChange={setSslMode}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="disable">disable</SelectItem>
              <SelectItem value="require">require</SelectItem>
              <SelectItem value="verify-ca">verify-ca</SelectItem>
              <SelectItem value="verify-full">verify-full</SelectItem>
            </SelectContent>
          </Select>
        </div>
      )}

      {/* TLS (MySQL only) */}
      {driver === "mysql" && (
        <div className="flex items-center justify-between">
          <Label>TLS</Label>
          <Switch checked={tls} onCheckedChange={setTls} />
        </div>
      )}

      {/* Params */}
      <div className="grid gap-2">
        <Label>{t("asset.params")}</Label>
        <Input value={params} onChange={(e) => setParams(e.target.value)} placeholder={t("asset.paramsPlaceholder")} />
      </div>

      {/* Read Only */}
      <div className="flex items-center justify-between">
        <Label>{t("asset.readOnly")}</Label>
        <Switch checked={readOnly} onCheckedChange={setReadOnly} />
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

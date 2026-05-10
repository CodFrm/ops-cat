import { useTranslation } from "react-i18next";
import { Input, Label, Switch, Tabs, TabsList, TabsTrigger, TabsContent } from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";
import { PasswordSourceField } from "@/components/asset/PasswordSourceField";
import { credential_entity } from "../../../wailsjs/go/models";

export interface MongoDBConfigSectionProps {
  connectionMode: "manual" | "uri";
  setConnectionMode: (v: "manual" | "uri") => void;
  host: string;
  setHost: (v: string) => void;
  port: number;
  setPort: (v: number) => void;
  username: string;
  setUsername: (v: string) => void;
  connectionURI: string;
  setConnectionURI: (v: string) => void;
  replicaSet: string;
  setReplicaSet: (v: string) => void;
  authSource: string;
  setAuthSource: (v: string) => void;
  database: string;
  setDatabase: (v: string) => void;
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

export function MongoDBConfigSection({
  connectionMode,
  setConnectionMode,
  host,
  setHost,
  port,
  setPort,
  username,
  setUsername,
  connectionURI,
  setConnectionURI,
  replicaSet,
  setReplicaSet,
  authSource,
  setAuthSource,
  database,
  setDatabase,
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
}: MongoDBConfigSectionProps) {
  const { t } = useTranslation();

  return (
    <>
      {/* Connection Mode Toggle */}
      <Tabs value={connectionMode} onValueChange={(v) => setConnectionMode(v as "manual" | "uri")}>
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger value="manual">Manual</TabsTrigger>
          <TabsTrigger value="uri">URI</TabsTrigger>
        </TabsList>

        <TabsContent value="manual" className="space-y-4 mt-4">
          {/* Host + Port */}
          <div className="grid gap-2">
            <Label>{t("asset.host")}</Label>
            <div className="flex gap-2">
              <Input
                className="flex-1"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="192.168.1.1"
              />
              <Input
                className="w-[80px] shrink-0 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
              />
            </div>
          </div>

          {/* Replica Set */}
          <div className="grid gap-2">
            <Label>{t("asset.mongoReplicaSet")}</Label>
            <Input
              value={replicaSet}
              onChange={(e) => setReplicaSet(e.target.value)}
              placeholder={t("asset.mongoReplicaSetPlaceholder")}
            />
          </div>

          {/* Auth Source */}
          <div className="grid gap-2">
            <Label>{t("asset.mongoAuthSource")}</Label>
            <Input
              value={authSource}
              onChange={(e) => setAuthSource(e.target.value)}
              placeholder={t("asset.mongoAuthSourcePlaceholder")}
            />
          </div>
        </TabsContent>

        <TabsContent value="uri" className="space-y-4 mt-4">
          {/* Connection URI */}
          <div className="grid gap-2">
            <Label>{t("asset.mongoUri")}</Label>
            <Input
              value={connectionURI}
              onChange={(e) => setConnectionURI(e.target.value)}
              placeholder={t("asset.mongoUriPlaceholder")}
            />
          </div>
        </TabsContent>
      </Tabs>

      {/* Common fields (shown for both modes) */}

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

      {/* Default Database */}
      <div className="grid gap-2">
        <Label>{t("asset.mongoDefaultDatabase")}</Label>
        <Input
          value={database}
          onChange={(e) => setDatabase(e.target.value)}
          placeholder={t("asset.mongoDefaultDatabasePlaceholder")}
        />
      </div>

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

import { useTranslation } from "react-i18next";
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  Textarea,
} from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";
import { PasswordSourceField } from "@/components/asset/PasswordSourceField";
import { credential_entity } from "../../../wailsjs/go/models";

export interface KafkaConfigSectionProps {
  brokersText: string;
  setBrokersText: (v: string) => void;
  clientId: string;
  setClientId: (v: string) => void;
  saslMechanism: string;
  setSaslMechanism: (v: string) => void;
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
  requestTimeoutSeconds: number;
  setRequestTimeoutSeconds: (v: number) => void;
  messagePreviewBytes: number;
  setMessagePreviewBytes: (v: number) => void;
  messageFetchLimit: number;
  setMessageFetchLimit: (v: number) => void;
  sshTunnelId: number;
  setSshTunnelId: (v: number) => void;
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

function normalizedNumber(value: string, fallback: number) {
  const next = Number(value);
  if (!Number.isFinite(next)) return fallback;
  return Math.max(0, Math.floor(next));
}

export function KafkaConfigSection({
  brokersText,
  setBrokersText,
  clientId,
  setClientId,
  saslMechanism,
  setSaslMechanism,
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
  requestTimeoutSeconds,
  setRequestTimeoutSeconds,
  messagePreviewBytes,
  setMessagePreviewBytes,
  messageFetchLimit,
  setMessageFetchLimit,
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
}: KafkaConfigSectionProps) {
  const { t } = useTranslation();
  const saslEnabled = saslMechanism !== "none";

  return (
    <>
      <div className="grid gap-2">
        <Label>{t("asset.kafkaBrokers")}</Label>
        <Textarea
          value={brokersText}
          onChange={(e) => setBrokersText(e.target.value)}
          rows={3}
          className="font-mono text-sm"
          placeholder="192.168.100.50:9092"
        />
      </div>

      <div className="grid gap-2">
        <Label>{t("asset.kafkaClientId")}</Label>
        <Input value={clientId} onChange={(e) => setClientId(e.target.value)} placeholder="opskat" />
      </div>

      <div className="grid gap-2">
        <Label>{t("asset.kafkaSaslMechanism")}</Label>
        <Select value={saslMechanism} onValueChange={setSaslMechanism}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">{t("asset.kafkaSaslNone")}</SelectItem>
            <SelectItem value="plain">PLAIN</SelectItem>
            <SelectItem value="scram-sha-256">SCRAM-SHA-256</SelectItem>
            <SelectItem value="scram-sha-512">SCRAM-SHA-512</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {saslEnabled && (
        <>
          <div className="grid gap-2">
            <Label>{t("asset.username")}</Label>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} />
          </div>
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
        </>
      )}

      <div className="flex items-center justify-between">
        <Label>{t("asset.tls")}</Label>
        <Switch checked={tls} onCheckedChange={setTls} />
      </div>

      {tls && (
        <>
          <div className="flex items-center justify-between">
            <Label>{t("asset.kafkaTlsInsecure")}</Label>
            <Switch checked={tlsInsecure} onCheckedChange={setTlsInsecure} />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.kafkaTlsServerName")}</Label>
            <Input
              value={tlsServerName}
              onChange={(e) => setTlsServerName(e.target.value)}
              placeholder="kafka.example.com"
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.kafkaTlsCAFile")}</Label>
            <Input value={tlsCAFile} onChange={(e) => setTlsCAFile(e.target.value)} placeholder="/path/to/ca.pem" />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.kafkaTlsCertFile")}</Label>
            <Input
              value={tlsCertFile}
              onChange={(e) => setTlsCertFile(e.target.value)}
              placeholder="/path/to/client.crt"
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.kafkaTlsKeyFile")}</Label>
            <Input
              value={tlsKeyFile}
              onChange={(e) => setTlsKeyFile(e.target.value)}
              placeholder="/path/to/client.key"
            />
          </div>
        </>
      )}

      <div className="grid grid-cols-3 gap-3">
        <div className="grid gap-2">
          <Label>{t("asset.kafkaRequestTimeout")}</Label>
          <Input
            className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            min={0}
            max={300}
            value={requestTimeoutSeconds}
            onChange={(e) => setRequestTimeoutSeconds(normalizedNumber(e.target.value, 30))}
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.kafkaMessagePreviewBytes")}</Label>
          <Input
            className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            min={0}
            value={messagePreviewBytes}
            onChange={(e) => setMessagePreviewBytes(normalizedNumber(e.target.value, 4096))}
          />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.kafkaMessageFetchLimit")}</Label>
          <Input
            className="[&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            min={0}
            max={1000}
            value={messageFetchLimit}
            onChange={(e) => setMessageFetchLimit(normalizedNumber(e.target.value, 50))}
          />
        </div>
      </div>

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

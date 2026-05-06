import { useTranslation } from "react-i18next";
import { Plus, Trash2 } from "lucide-react";
import {
  Button,
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

export type KafkaPasswordSource = "inline" | "managed";

export interface KafkaCompanionAuthForm {
  authType: string;
  username: string;
  password: string;
  encryptedPassword: string;
  passwordSource: KafkaPasswordSource;
  credentialId: number;
  tlsInsecure: boolean;
  tlsServerName: string;
  tlsCAFile: string;
  tlsCertFile: string;
  tlsKeyFile: string;
}

export interface KafkaSchemaRegistryForm extends KafkaCompanionAuthForm {
  enabled: boolean;
  url: string;
}

export interface KafkaConnectClusterForm extends KafkaCompanionAuthForm {
  id: string;
  name: string;
  url: string;
}

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
  schemaRegistry: KafkaSchemaRegistryForm;
  setSchemaRegistry: (patch: Partial<KafkaSchemaRegistryForm>) => void;
  connectEnabled: boolean;
  setConnectEnabled: (v: boolean) => void;
  connectClusters: KafkaConnectClusterForm[];
  setConnectClusters: (clusters: KafkaConnectClusterForm[]) => void;
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
  schemaRegistry,
  setSchemaRegistry,
  connectEnabled,
  setConnectEnabled,
  connectClusters,
  setConnectClusters,
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

      <div className="grid gap-3 rounded-md border p-3">
        <div className="flex items-center justify-between gap-3">
          <Label>{t("asset.kafkaSchemaRegistry")}</Label>
          <Switch checked={schemaRegistry.enabled} onCheckedChange={(enabled) => setSchemaRegistry({ enabled })} />
        </div>
        {schemaRegistry.enabled && (
          <>
            <div className="grid gap-2">
              <Label>{t("asset.kafkaSchemaRegistryURL")}</Label>
              <Input
                value={schemaRegistry.url}
                onChange={(e) => setSchemaRegistry({ url: e.target.value })}
                placeholder="http://schema-registry.example.com:8081"
              />
            </div>
            <KafkaCompanionAuthFields
              value={schemaRegistry}
              onChange={setSchemaRegistry}
              managedPasswords={managedPasswords}
            />
          </>
        )}
      </div>

      <div className="grid gap-3 rounded-md border p-3">
        <div className="flex items-center justify-between gap-3">
          <Label>{t("asset.kafkaConnect")}</Label>
          <Switch
            checked={connectEnabled}
            onCheckedChange={(enabled) => {
              setConnectEnabled(enabled);
              if (enabled && connectClusters.length === 0) {
                setConnectClusters([createConnectClusterForm()]);
              }
            }}
          />
        </div>
        {connectEnabled && (
          <div className="grid gap-3">
            {connectClusters.map((cluster, index) => (
              <KafkaConnectClusterEditor
                key={cluster.id}
                index={index}
                value={cluster}
                onChange={(patch) =>
                  setConnectClusters(
                    connectClusters.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item))
                  )
                }
                onRemove={() => setConnectClusters(connectClusters.filter((_, itemIndex) => itemIndex !== index))}
                managedPasswords={managedPasswords}
              />
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 w-fit gap-1.5"
              onClick={() => setConnectClusters([...connectClusters, createConnectClusterForm()])}
            >
              <Plus className="h-3.5 w-3.5" />
              {t("asset.kafkaConnectAddCluster")}
            </Button>
          </div>
        )}
      </div>
    </>
  );
}

function KafkaCompanionAuthFields({
  value,
  onChange,
  managedPasswords,
}: {
  value: KafkaCompanionAuthForm;
  onChange: (patch: Partial<KafkaCompanionAuthForm>) => void;
  managedPasswords: credential_entity.Credential[];
}) {
  const { t } = useTranslation();
  const authEnabled = value.authType !== "none";

  return (
    <div className="grid gap-3">
      <div className="grid gap-2">
        <Label>{t("asset.kafkaCompanionAuthType")}</Label>
        <Select
          value={value.authType}
          onValueChange={(authType) => onChange(kafkaCompanionAuthTypePatch(value, authType))}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none">{t("asset.kafkaSaslNone")}</SelectItem>
            <SelectItem value="basic">Basic</SelectItem>
            <SelectItem value="bearer">Bearer</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {authEnabled && (
        <>
          {value.authType !== "bearer" && (
            <div className="grid gap-2">
              <Label>{t("asset.username")}</Label>
              <Input value={value.username} onChange={(e) => onChange({ username: e.target.value })} />
            </div>
          )}
          <PasswordSourceField
            source={value.passwordSource}
            onSourceChange={(passwordSource) => onChange({ passwordSource })}
            password={value.password}
            onPasswordChange={(password) => onChange({ password })}
            credentialId={value.credentialId}
            onCredentialIdChange={(credentialId) => onChange({ credentialId })}
            managedPasswords={managedPasswords}
            hasExistingPassword={!!value.encryptedPassword}
            secretLabel={value.authType === "bearer" ? t("asset.kafkaBearerToken") : undefined}
            selectSecretLabel={value.authType === "bearer" ? t("asset.kafkaBearerToken") : undefined}
          />
        </>
      )}
      <div className="flex items-center justify-between">
        <Label>{t("asset.kafkaTlsInsecure")}</Label>
        <Switch checked={value.tlsInsecure} onCheckedChange={(tlsInsecure) => onChange({ tlsInsecure })} />
      </div>
      <div className="grid gap-2">
        <Label>{t("asset.kafkaTlsServerName")}</Label>
        <Input value={value.tlsServerName} onChange={(e) => onChange({ tlsServerName: e.target.value })} />
      </div>
      <div className="grid gap-2">
        <Label>{t("asset.kafkaTlsCAFile")}</Label>
        <Input value={value.tlsCAFile} onChange={(e) => onChange({ tlsCAFile: e.target.value })} />
      </div>
      <div className="grid gap-2 md:grid-cols-2">
        <div className="grid gap-2">
          <Label>{t("asset.kafkaTlsCertFile")}</Label>
          <Input value={value.tlsCertFile} onChange={(e) => onChange({ tlsCertFile: e.target.value })} />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.kafkaTlsKeyFile")}</Label>
          <Input value={value.tlsKeyFile} onChange={(e) => onChange({ tlsKeyFile: e.target.value })} />
        </div>
      </div>
    </div>
  );
}

function kafkaCompanionAuthTypePatch(value: KafkaCompanionAuthForm, authType: string): Partial<KafkaCompanionAuthForm> {
  const patch: Partial<KafkaCompanionAuthForm> = { authType };
  if (authType === "bearer") {
    if (value.username && !value.password && !value.encryptedPassword && !value.credentialId) {
      patch.password = value.username;
    }
    patch.username = "";
  }
  return patch;
}

function KafkaConnectClusterEditor({
  index,
  value,
  onChange,
  onRemove,
  managedPasswords,
}: {
  index: number;
  value: KafkaConnectClusterForm;
  onChange: (patch: Partial<KafkaConnectClusterForm>) => void;
  onRemove: () => void;
  managedPasswords: credential_entity.Credential[];
}) {
  const { t } = useTranslation();

  return (
    <div className="grid gap-3 rounded-md border bg-muted/20 p-3">
      <div className="flex items-center justify-between gap-2">
        <Label>{t("asset.kafkaConnectClusterNumber", { index: index + 1 })}</Label>
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7 text-destructive" onClick={onRemove}>
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="grid gap-2 md:grid-cols-2">
        <div className="grid gap-2">
          <Label>{t("asset.kafkaConnectClusterName")}</Label>
          <Input value={value.name} onChange={(e) => onChange({ name: e.target.value })} placeholder="primary" />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.kafkaConnectClusterURL")}</Label>
          <Input
            value={value.url}
            onChange={(e) => onChange({ url: e.target.value })}
            placeholder="http://connect.example.com:8083"
          />
        </div>
      </div>
      <KafkaCompanionAuthFields value={value} onChange={onChange} managedPasswords={managedPasswords} />
    </div>
  );
}

function createConnectClusterForm(): KafkaConnectClusterForm {
  return {
    id: `connect-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
    name: "",
    url: "",
    authType: "none",
    username: "",
    password: "",
    encryptedPassword: "",
    passwordSource: "inline",
    credentialId: 0,
    tlsInsecure: false,
    tlsServerName: "",
    tlsCAFile: "",
    tlsCertFile: "",
    tlsKeyFile: "",
  };
}

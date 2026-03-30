import { useState, useEffect } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Loader2, PlugZap } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Button,
  Input,
  Label,
  Textarea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import { IconPicker } from "@/components/asset/IconPicker";
import { GroupSelect } from "@/components/asset/GroupSelect";
import { useAssetStore } from "@/stores/assetStore";
import { asset_entity, credential_entity } from "../../../wailsjs/go/models";
import {
  EncryptPassword,
  GetAvailableAssetTypes,
  ListCredentialsByType,
  ListLocalSSHKeys,
  TestSSHConnection,
  TestDatabaseConnection,
  TestRedisConnection,
} from "../../../wailsjs/go/app/App";
import { app } from "../../../wailsjs/go/models";
import { SSHConfigSection } from "@/components/asset/SSHConfigSection";
import { DatabaseConfigSection } from "@/components/asset/DatabaseConfigSection";
import { RedisConfigSection } from "@/components/asset/RedisConfigSection";
import { useExtensionStore, ExtensionPage } from "@/extension";

interface AssetFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editAsset?: asset_entity.Asset | null;
  defaultGroupId?: number;
}

interface ProxyConfig {
  type: string;
  host: string;
  port: number;
  username?: string;
  password?: string;
}

interface SSHConfig {
  host: string;
  port: number;
  username: string;
  auth_type: string;
  password?: string;
  credential_id?: number;
  private_keys?: string[];
  jump_host_id?: number;
  proxy?: ProxyConfig | null;
}

interface DatabaseConfig {
  driver: string;
  host: string;
  port: number;
  username: string;
  password?: string;
  credential_id?: number;
  database?: string;
  ssl_mode?: string;
  tls?: boolean;
  params?: string;
  read_only?: boolean;
  ssh_asset_id?: number;
}

interface RedisConfig {
  host: string;
  port: number;
  username?: string;
  password?: string;
  credential_id?: number;
  tls?: boolean;
  ssh_asset_id?: number;
}

type AssetType = "ssh" | "database" | "redis" | (string & {});

const DEFAULT_PORTS: Record<string, number> = {
  ssh: 22,
  mysql: 3306,
  postgresql: 5432,
  redis: 6379,
};

const DEFAULT_ICONS: Record<string, string> = {
  ssh: "server",
  mysql: "mysql",
  postgresql: "postgresql",
  redis: "redis",
};

export function AssetForm({ open, onOpenChange, editAsset, defaultGroupId = 0 }: AssetFormProps) {
  const { t } = useTranslation();
  const { createAsset, updateAsset } = useAssetStore();

  // Asset type
  const [assetType, setAssetType] = useState<AssetType>("ssh");
  const [availableTypes, setAvailableTypes] = useState<
    { type: string; extensionName?: string; displayName: string; sshTunnel?: boolean }[]
  >([]);

  // Basic fields
  const [name, setName] = useState("");
  const [groupId, setGroupId] = useState(0);
  const [description, setDescription] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState(22);
  const [username, setUsername] = useState("root");
  const [authType, setAuthType] = useState("password");
  const [icon, setIcon] = useState("server");
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  // Connection type (SSH only)
  const [connectionType, setConnectionType] = useState<"direct" | "jumphost" | "proxy">("direct");

  // Auth fields
  const [password, setPassword] = useState("");
  const [encryptedPassword, setEncryptedPassword] = useState("");
  const [passwordSource, setPasswordSource] = useState<"inline" | "managed">("inline");
  const [passwordCredentialId, setPasswordCredentialId] = useState(0);
  const [managedPasswords, setManagedPasswords] = useState<credential_entity.Credential[]>([]);
  const [keySource, setKeySource] = useState<"managed" | "file">("managed");
  const [credentialId, setCredentialId] = useState(0);
  const [managedKeys, setManagedKeys] = useState<credential_entity.Credential[]>([]);

  // SSH fields - local key
  const [localKeys, setLocalKeys] = useState<app.LocalSSHKeyInfo[]>([]);
  const [selectedKeyPaths, setSelectedKeyPaths] = useState<string[]>([]);
  const [scanningKeys, setScanningKeys] = useState(false);
  const [sshTunnelId, setSshTunnelId] = useState(0);
  const [proxyType, setProxyType] = useState("socks5");
  const [proxyHost, setProxyHost] = useState("");
  const [proxyPort, setProxyPort] = useState(1080);
  const [proxyUsername, setProxyUsername] = useState("");
  const [proxyPassword, setProxyPassword] = useState("");
  const [encryptedProxyPassword, setEncryptedProxyPassword] = useState("");

  // Database fields
  const [driver, setDriver] = useState("mysql");
  const [database, setDatabase] = useState("");
  const [sslMode, setSslMode] = useState("disable");
  const [readOnly, setReadOnly] = useState(false);
  const [params, setParams] = useState("");

  // Redis fields
  const [tls, setTls] = useState(false);

  // Exclude self from jump host / SSH tunnel selection
  const jumpHostExcludeIds = editAsset?.ID ? [editAsset.ID] : undefined;

  // Load managed keys/passwords and scan local keys when dialog opens
  useEffect(() => {
    if (open) {
      ListCredentialsByType("ssh_key")
        .then((keys) => setManagedKeys(keys || []))
        .catch(() => setManagedKeys([]));
      ListCredentialsByType("password")
        .then((passwords) => setManagedPasswords(passwords || []))
        .catch(() => setManagedPasswords([]));
      setScanningKeys(true);
      ListLocalSSHKeys()
        .then((keys) => setLocalKeys(keys || []))
        .catch(() => setLocalKeys([]))
        .finally(() => setScanningKeys(false));
      GetAvailableAssetTypes()
        .then((types) => setAvailableTypes(types || []))
        .catch(() => setAvailableTypes([]));
    }
  }, [open]);

  useEffect(() => {
    if (open) {
      if (editAsset) {
        const editType = (editAsset.Type || "ssh") as AssetType;
        setAssetType(editType);
        setName(editAsset.Name);
        setGroupId(editAsset.GroupID);
        setIcon(editAsset.Icon || DEFAULT_ICONS[editType] || "server");
        setDescription(editAsset.Description);

        if (editType === "ssh") {
          loadSSHConfig(editAsset);
        } else if (editType === "database") {
          loadDatabaseConfig(editAsset);
        } else if (editType === "redis") {
          loadRedisConfig(editAsset);
        }
      } else {
        setAssetType("ssh");
        setName("");
        setGroupId(defaultGroupId);
        setIcon("server");
        setDescription("");
        resetSharedFields("ssh");
        resetSSHFields();
        resetDatabaseFields();
        resetRedisFields();
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, editAsset, defaultGroupId]);

  const loadSSHConfig = (asset: asset_entity.Asset) => {
    try {
      const cfg: SSHConfig = JSON.parse(asset.Config || "{}");
      setHost(cfg.host || "");
      setPort(cfg.port || 22);
      setUsername(cfg.username || "root");
      setAuthType(cfg.auth_type || "password");

      setEncryptedPassword(cfg.password || "");
      setPassword("");
      if (cfg.auth_type === "password" && cfg.credential_id) {
        setPasswordSource("managed");
        setPasswordCredentialId(cfg.credential_id);
      } else {
        setPasswordSource("inline");
        setPasswordCredentialId(0);
      }
      setKeySource(cfg.private_keys && cfg.private_keys.length > 0 ? "file" : "managed");
      setCredentialId(cfg.auth_type === "key" ? cfg.credential_id || 0 : 0);
      setSelectedKeyPaths(cfg.private_keys || []);

      // Unified SSH tunnel: prefer asset-level field, fall back to config
      const tunnelId = asset.sshTunnelId || cfg.jump_host_id || 0;
      setSshTunnelId(tunnelId);

      if (tunnelId) {
        setConnectionType("jumphost");
      } else if (cfg.proxy) {
        setConnectionType("proxy");
      } else {
        setConnectionType("direct");
      }

      if (cfg.proxy) {
        setProxyType(cfg.proxy.type || "socks5");
        setProxyHost(cfg.proxy.host || "");
        setProxyPort(cfg.proxy.port || 1080);
        setProxyUsername(cfg.proxy.username || "");
        setEncryptedProxyPassword(cfg.proxy.password || "");
        setProxyPassword("");
      } else {
        resetProxyFields();
      }
    } catch {
      resetSharedFields("ssh");
      resetSSHFields();
    }
  };

  const loadDatabaseConfig = (asset: asset_entity.Asset) => {
    try {
      const cfg: DatabaseConfig = JSON.parse(asset.Config || "{}");
      setHost(cfg.host || "");
      setPort(cfg.port || 3306);
      setUsername(cfg.username || "");
      setDriver(cfg.driver || "mysql");
      setDatabase(cfg.database || "");
      setSslMode(cfg.ssl_mode || "disable");
      setTls(cfg.tls || false);
      setReadOnly(cfg.read_only || false);
      setSshTunnelId(asset.sshTunnelId || cfg.ssh_asset_id || 0);
      setParams(cfg.params || "");

      if (cfg.credential_id) {
        setPasswordSource("managed");
        setPasswordCredentialId(cfg.credential_id);
        setEncryptedPassword("");
        setPassword("");
      } else {
        setPasswordSource("inline");
        setPasswordCredentialId(0);
        setEncryptedPassword(cfg.password || "");
        setPassword("");
      }
    } catch {
      resetSharedFields("database");
      resetDatabaseFields();
    }
  };

  const loadRedisConfig = (asset: asset_entity.Asset) => {
    try {
      const cfg: RedisConfig = JSON.parse(asset.Config || "{}");
      setHost(cfg.host || "");
      setPort(cfg.port || 6379);
      setUsername(cfg.username || "");
      setTls(cfg.tls || false);
      setSshTunnelId(asset.sshTunnelId || cfg.ssh_asset_id || 0);

      if (cfg.credential_id) {
        setPasswordSource("managed");
        setPasswordCredentialId(cfg.credential_id);
        setEncryptedPassword("");
        setPassword("");
      } else {
        setPasswordSource("inline");
        setPasswordCredentialId(0);
        setEncryptedPassword(cfg.password || "");
        setPassword("");
      }
    } catch {
      resetSharedFields("redis");
      resetRedisFields();
    }
  };

  // Reset shared connection fields with type-appropriate defaults
  const resetSharedFields = (type: AssetType, dbDriver = "mysql") => {
    setHost("");
    setPort(type === "database" ? DEFAULT_PORTS[dbDriver] || 3306 : DEFAULT_PORTS[type] || 22);
    setUsername(type === "ssh" ? "root" : "");
    setPassword("");
    setEncryptedPassword("");
    setPasswordSource("inline");
    setPasswordCredentialId(0);
  };

  const resetProxyFields = () => {
    setProxyType("socks5");
    setProxyHost("");
    setProxyPort(1080);
    setProxyUsername("");
    setProxyPassword("");
    setEncryptedProxyPassword("");
  };

  // SSH-exclusive fields only
  const resetSSHFields = () => {
    setAuthType("password");
    setKeySource("managed");
    setCredentialId(0);
    setSelectedKeyPaths([]);
    setConnectionType("direct");
    setSshTunnelId(0);
    resetProxyFields();
  };

  // Database-exclusive fields only
  const resetDatabaseFields = () => {
    setDriver("mysql");
    setDatabase("");
    setSslMode("disable");
    setTls(false);
    setReadOnly(false);
    setParams("");
  };

  // Redis-exclusive fields only
  const resetRedisFields = () => {
    setTls(false);
  };

  const handleTypeChange = (newType: AssetType) => {
    if (newType === assetType) return;
    setAssetType(newType);

    // Reset port/username/password to type-appropriate defaults (keep host)
    const defaultDriver = newType === "database" ? driver : undefined;
    setPort(newType === "database" ? DEFAULT_PORTS[defaultDriver || "mysql"] || 3306 : DEFAULT_PORTS[newType] || 22);
    setUsername(newType === "ssh" ? "root" : "");
    setPassword("");
    setEncryptedPassword("");
    setPasswordSource("inline");
    setPasswordCredentialId(0);
    setIcon(newType === "database" ? DEFAULT_ICONS[driver] || "mysql" : DEFAULT_ICONS[newType] || "server");
  };

  const handleDriverChange = (newDriver: string) => {
    setDriver(newDriver);
    setPort(DEFAULT_PORTS[newDriver] || 3306);
    setIcon(DEFAULT_ICONS[newDriver] || "mysql");
    if (newDriver !== "postgresql") {
      setSslMode("disable");
    }
  };

  const handleTestConnection = async () => {
    const sshConfig: SSHConfig = {
      host,
      port,
      username,
      auth_type: authType,
    };
    if (authType === "key") {
      if (keySource === "managed" && credentialId > 0) sshConfig.credential_id = credentialId;
      if (keySource === "file" && selectedKeyPaths.length > 0) sshConfig.private_keys = selectedKeyPaths;
    }
    if (!password && encryptedPassword) {
      sshConfig.password = encryptedPassword;
    }
    if (connectionType === "jumphost" && sshTunnelId > 0) sshConfig.jump_host_id = sshTunnelId;
    if (connectionType === "proxy" && proxyHost) {
      sshConfig.proxy = {
        type: proxyType,
        host: proxyHost,
        port: proxyPort,
        username: proxyUsername || undefined,
        password: proxyPassword || undefined,
      };
    }
    setTesting(true);
    try {
      await TestSSHConnection(JSON.stringify(sshConfig), password);
      toast.success(t("asset.testConnectionSuccess"));
    } catch (e) {
      toast.error(`${t("asset.testConnectionFailed")}: ${String(e)}`);
    } finally {
      setTesting(false);
    }
  };

  const handleTestDatabaseConnection = async () => {
    const cfg: DatabaseConfig = { driver, host, port, username };
    if (database) cfg.database = database;
    if (driver === "postgresql" && sslMode !== "disable") cfg.ssl_mode = sslMode;
    if (driver === "mysql" && tls) cfg.tls = true;
    if (readOnly) cfg.read_only = true;
    if (sshTunnelId > 0) cfg.ssh_asset_id = sshTunnelId;
    if (params) cfg.params = params;
    if (!password && encryptedPassword) cfg.password = encryptedPassword;
    setTesting(true);
    try {
      await TestDatabaseConnection(JSON.stringify(cfg), password);
      toast.success(t("asset.testConnectionSuccess"));
    } catch (e) {
      toast.error(`${t("asset.testConnectionFailed")}: ${String(e)}`);
    } finally {
      setTesting(false);
    }
  };

  const handleTestRedisConnection = async () => {
    const cfg: RedisConfig = { host, port };
    if (username) cfg.username = username;
    if (tls) cfg.tls = true;
    if (sshTunnelId > 0) cfg.ssh_asset_id = sshTunnelId;
    if (!password && encryptedPassword) cfg.password = encryptedPassword;
    setTesting(true);
    try {
      await TestRedisConnection(JSON.stringify(cfg), password);
      toast.success(t("asset.testConnectionSuccess"));
    } catch (e) {
      toast.error(`${t("asset.testConnectionFailed")}: ${String(e)}`);
    } finally {
      setTesting(false);
    }
  };

  const encryptPasswordValue = async (): Promise<string | undefined> => {
    if (password) {
      try {
        return await EncryptPassword(password);
      } catch {
        toast.error("Failed to encrypt password");
        return undefined;
      }
    }
    if (encryptedPassword) return encryptedPassword;
    return "";
  };

  const encryptProxyPassword = async (): Promise<string | undefined> => {
    if (proxyPassword) {
      try {
        return await EncryptPassword(proxyPassword);
      } catch {
        toast.error("Failed to encrypt proxy password");
        return undefined;
      }
    }
    if (encryptedProxyPassword) return encryptedProxyPassword;
    return undefined;
  };

  const handleSubmit = async () => {
    let config: string;

    if (assetType === "ssh") {
      const sshConfig: SSHConfig = {
        host,
        port,
        username,
        auth_type: authType,
      };

      if (authType === "password") {
        if (passwordSource === "managed" && passwordCredentialId > 0) {
          sshConfig.credential_id = passwordCredentialId;
        } else {
          const encrypted = await encryptPasswordValue();
          if (encrypted === undefined) return;
          if (encrypted) sshConfig.password = encrypted;
        }
      }

      if (authType === "key") {
        if (keySource === "managed" && credentialId > 0) sshConfig.credential_id = credentialId;
        if (keySource === "file" && selectedKeyPaths.length > 0) sshConfig.private_keys = selectedKeyPaths;
      }

      if (connectionType === "proxy" && proxyHost) {
        const encProxy = await encryptProxyPassword();
        sshConfig.proxy = {
          type: proxyType,
          host: proxyHost,
          port: proxyPort,
          username: proxyUsername || undefined,
          password: encProxy || undefined,
        };
      }
      config = JSON.stringify(sshConfig);
    } else if (assetType === "database") {
      const dbConfig: DatabaseConfig = {
        driver,
        host,
        port,
        username,
      };
      if (passwordSource === "managed" && passwordCredentialId > 0) {
        dbConfig.credential_id = passwordCredentialId;
      } else {
        const encrypted = await encryptPasswordValue();
        if (encrypted === undefined) return;
        if (encrypted) dbConfig.password = encrypted;
      }
      if (database) dbConfig.database = database;
      if (driver === "postgresql" && sslMode !== "disable") dbConfig.ssl_mode = sslMode;
      if (driver === "mysql" && tls) dbConfig.tls = true;
      if (readOnly) dbConfig.read_only = true;
      if (params) dbConfig.params = params;
      config = JSON.stringify(dbConfig);
    } else if (assetType === "redis") {
      const redisConfig: RedisConfig = {
        host,
        port,
      };
      if (username) redisConfig.username = username;
      if (passwordSource === "managed" && passwordCredentialId > 0) {
        redisConfig.credential_id = passwordCredentialId;
      } else {
        const encrypted = await encryptPasswordValue();
        if (encrypted === undefined) return;
        if (encrypted) redisConfig.password = encrypted;
      }
      if (tls) redisConfig.tls = true;
      config = JSON.stringify(redisConfig);
    } else {
      // Extension type: keep existing config (set by extension's ConfigForm)
      config = editAsset?.Config || "{}";
    }

    const asset = new asset_entity.Asset({
      ...(editAsset || {}),
      Name: name,
      Type: assetType,
      GroupID: groupId,
      Icon: icon,
      Description: description,
      Config: config,
      sshTunnelId: sshTunnelId > 0 ? sshTunnelId : 0,
    });

    setSaving(true);
    try {
      if (editAsset?.ID) {
        asset.ID = editAsset.ID;
        await updateAsset(asset);
      } else {
        await createAsset(asset);
      }
      onOpenChange(false);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  const typeLabel =
    assetType === "ssh"
      ? t("asset.typeSSH")
      : assetType === "database"
        ? t("asset.typeDatabase")
        : assetType === "redis"
          ? t("asset.typeRedis")
          : availableTypes.find((at) => at.type === assetType)?.displayName || assetType;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>
            {editAsset ? t("action.edit") : t("action.add")} {typeLabel}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          {/* Asset Type */}
          {!editAsset && (
            <div className="grid gap-2">
              <Label>{t("asset.type")}</Label>
              <Select value={assetType} onValueChange={(v) => handleTypeChange(v as AssetType)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ssh">{t("asset.typeSSH")}</SelectItem>
                  <SelectItem value="database">{t("asset.typeDatabase")}</SelectItem>
                  <SelectItem value="redis">{t("asset.typeRedis")}</SelectItem>
                  {availableTypes
                    .filter((at) => !!at.extensionName)
                    .map((at) => (
                      <SelectItem key={at.type} value={at.type}>
                        {at.displayName}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Name */}
          <div className="grid gap-2">
            <Label>{t("asset.name")}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={assetType === "ssh" ? "web-01" : assetType === "database" ? "prod-db" : "cache-01"}
            />
          </div>

          {/* Icon */}
          <div className="grid gap-2">
            <Label>{t("asset.icon")}</Label>
            <IconPicker value={icon} onChange={setIcon} type="asset" />
          </div>

          {/* Database Driver (database only, before host) */}
          {assetType === "database" && (
            <div className="grid gap-2">
              <Label>{t("asset.driver")}</Label>
              <Select value={driver} onValueChange={handleDriverChange}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="mysql">{t("asset.driverMySQL")}</SelectItem>
                  <SelectItem value="postgresql">{t("asset.driverPostgreSQL")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Type-specific config sections */}
          {assetType === "ssh" && (
            <SSHConfigSection
              host={host}
              setHost={setHost}
              port={port}
              setPort={setPort}
              username={username}
              setUsername={setUsername}
              authType={authType}
              setAuthType={setAuthType}
              connectionType={connectionType}
              setConnectionType={setConnectionType}
              password={password}
              setPassword={setPassword}
              encryptedPassword={encryptedPassword}
              passwordSource={passwordSource}
              setPasswordSource={setPasswordSource}
              passwordCredentialId={passwordCredentialId}
              setPasswordCredentialId={setPasswordCredentialId}
              managedPasswords={managedPasswords}
              keySource={keySource}
              setKeySource={setKeySource}
              credentialId={credentialId}
              setCredentialId={setCredentialId}
              managedKeys={managedKeys}
              localKeys={localKeys}
              setLocalKeys={setLocalKeys}
              selectedKeyPaths={selectedKeyPaths}
              setSelectedKeyPaths={setSelectedKeyPaths}
              scanningKeys={scanningKeys}
              sshTunnelId={sshTunnelId}
              setSshTunnelId={setSshTunnelId}
              jumpHostExcludeIds={jumpHostExcludeIds}
              proxyType={proxyType}
              setProxyType={setProxyType}
              proxyHost={proxyHost}
              setProxyHost={setProxyHost}
              proxyPort={proxyPort}
              setProxyPort={setProxyPort}
              proxyUsername={proxyUsername}
              setProxyUsername={setProxyUsername}
              proxyPassword={proxyPassword}
              setProxyPassword={setProxyPassword}
              encryptedProxyPassword={encryptedProxyPassword}
              editAssetId={editAsset?.ID}
            />
          )}

          {assetType === "database" && (
            <DatabaseConfigSection
              host={host}
              setHost={setHost}
              port={port}
              setPort={setPort}
              username={username}
              setUsername={setUsername}
              driver={driver}
              database={database}
              setDatabase={setDatabase}
              sslMode={sslMode}
              setSslMode={setSslMode}
              tls={tls}
              setTls={setTls}
              readOnly={readOnly}
              setReadOnly={setReadOnly}
              sshTunnelId={sshTunnelId}
              setSshTunnelId={setSshTunnelId}
              params={params}
              setParams={setParams}
              password={password}
              setPassword={setPassword}
              encryptedPassword={encryptedPassword}
              passwordSource={passwordSource}
              setPasswordSource={setPasswordSource}
              passwordCredentialId={passwordCredentialId}
              setPasswordCredentialId={setPasswordCredentialId}
              managedPasswords={managedPasswords}
              editAssetId={editAsset?.ID}
            />
          )}

          {assetType === "redis" && (
            <RedisConfigSection
              host={host}
              setHost={setHost}
              port={port}
              setPort={setPort}
              username={username}
              setUsername={setUsername}
              tls={tls}
              setTls={setTls}
              sshTunnelId={sshTunnelId}
              setSshTunnelId={setSshTunnelId}
              password={password}
              setPassword={setPassword}
              encryptedPassword={encryptedPassword}
              passwordSource={passwordSource}
              setPasswordSource={setPasswordSource}
              passwordCredentialId={passwordCredentialId}
              setPasswordCredentialId={setPasswordCredentialId}
              managedPasswords={managedPasswords}
              editAssetId={editAsset?.ID}
            />
          )}

          {/* Extension type config */}
          {assetType !== "ssh" &&
            assetType !== "database" &&
            assetType !== "redis" &&
            (() => {
              const extInfo = useExtensionStore.getState().getExtensionForAssetType(assetType);
              if (!extInfo) return null;
              const configPage = extInfo.manifest.frontend?.pages.find((p) => p.component === "ConfigForm");
              if (!configPage) return null;
              return <ExtensionPage extensionName={extInfo.name} pageId={configPage.id} assetId={editAsset?.ID} />;
            })()}

          {/* Test Connection */}
          {(assetType === "ssh" || assetType === "database" || assetType === "redis") && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={
                assetType === "ssh"
                  ? handleTestConnection
                  : assetType === "database"
                    ? handleTestDatabaseConnection
                    : handleTestRedisConnection
              }
              disabled={testing || !host}
              className="gap-1 w-fit"
            >
              {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <PlugZap className="h-3.5 w-3.5" />}
              {testing ? t("asset.testing") : t("asset.testConnection")}
            </Button>
          )}

          {/* Group - Tree Selector */}
          <div className="grid gap-2">
            <Label>{t("asset.group")}</Label>
            <GroupSelect value={groupId} onValueChange={setGroupId} />
          </div>

          {/* Description */}
          <div className="grid gap-2">
            <Label>{t("asset.description")}</Label>
            <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={saving || !name || !host}>
            {t("action.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

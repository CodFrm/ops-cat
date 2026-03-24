import { useState, useEffect } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Trash2, Eye, EyeOff, FolderOpen, Loader2, PlugZap } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { IconPicker } from "@/components/asset/IconPicker";
import { AssetSelect } from "@/components/asset/AssetSelect";
import { GroupSelect } from "@/components/asset/GroupSelect";
import { useAssetStore } from "@/stores/assetStore";
import { asset_entity, credential_entity } from "../../../wailsjs/go/models";
import {
  EncryptPassword,
  ListCredentialsByType,
  ListLocalSSHKeys,
  SelectSSHKeyFile,
  TestSSHConnection,
  TestDatabaseConnection,
  TestRedisConnection,
} from "../../../wailsjs/go/main/App";
import { main } from "../../../wailsjs/go/models";

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
  database?: string;
  ssl_mode?: string;
  params?: string;
  read_only?: boolean;
  ssh_asset_id?: number;
}

interface RedisConfig {
  host: string;
  port: number;
  username?: string;
  password?: string;
  tls?: boolean;
  ssh_asset_id?: number;
}

type AssetType = "ssh" | "database" | "redis";

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

export function AssetForm({
  open,
  onOpenChange,
  editAsset,
  defaultGroupId = 0,
}: AssetFormProps) {
  const { t } = useTranslation();
  const { createAsset, updateAsset } = useAssetStore();

  // Asset type
  const [assetType, setAssetType] = useState<AssetType>("ssh");

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
  const [showPassword, setShowPassword] = useState(false);
  const [encryptedPassword, setEncryptedPassword] = useState("");
  const [keySource, setKeySource] = useState<"managed" | "file">("managed");
  const [credentialId, setCredentialId] = useState(0);
  const [managedKeys, setManagedKeys] = useState<credential_entity.Credential[]>([]);

  // SSH fields - local key
  const [localKeys, setLocalKeys] = useState<main.LocalSSHKeyInfo[]>([]);
  const [selectedKeyPaths, setSelectedKeyPaths] = useState<string[]>([]);
  const [scanningKeys, setScanningKeys] = useState(false);
  const [jumpHostId, setJumpHostId] = useState(0);
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
  const [dbSshAssetId, setDbSshAssetId] = useState(0);
  const [params, setParams] = useState("");

  // Redis fields
  const [tls, setTls] = useState(false);
  const [redisSshAssetId, setRedisSshAssetId] = useState(0);

  // Exclude self from jump host / SSH tunnel selection
  const jumpHostExcludeIds = editAsset?.ID ? [editAsset.ID] : undefined;

  // Load managed keys and scan local keys when dialog opens (for SSH type)
  useEffect(() => {
    if (open) {
      ListCredentialsByType("ssh_key")
        .then((keys) => setManagedKeys(keys || []))
        .catch(() => setManagedKeys([]));
      setScanningKeys(true);
      ListLocalSSHKeys()
        .then((keys) => setLocalKeys(keys || []))
        .catch(() => setLocalKeys([]))
        .finally(() => setScanningKeys(false));
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
        resetSSHFields();
        resetDatabaseFields();
        resetRedisFields();
      }
    }
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
      // 向后兼容：如果有 private_keys 字段则是文件模式，否则是托管模式
      setKeySource(cfg.private_keys && cfg.private_keys.length > 0 ? "file" : "managed");
      setCredentialId(cfg.credential_id || 0);
      setSelectedKeyPaths(cfg.private_keys || []);
      setJumpHostId(cfg.jump_host_id || 0);

      if (cfg.jump_host_id) {
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
      setReadOnly(cfg.read_only || false);
      setDbSshAssetId(cfg.ssh_asset_id || 0);
      setParams(cfg.params || "");

      setEncryptedPassword(cfg.password || "");
      setPassword("");
    } catch {
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
      setRedisSshAssetId(cfg.ssh_asset_id || 0);

      setEncryptedPassword(cfg.password || "");
      setPassword("");
    } catch {
      resetRedisFields();
    }
  };

  const resetProxyFields = () => {
    setProxyType("socks5");
    setProxyHost("");
    setProxyPort(1080);
    setProxyUsername("");
    setProxyPassword("");
    setEncryptedProxyPassword("");
  };

  const resetSSHFields = () => {
    setHost("");
    setPort(22);
    setUsername("root");
    setAuthType("password");
    setPassword("");
    setShowPassword(false);
    setEncryptedPassword("");
    setKeySource("managed");
    setCredentialId(0);
    setSelectedKeyPaths([]);
    setConnectionType("direct");
    setJumpHostId(0);
    resetProxyFields();
  };

  const resetDatabaseFields = () => {
    setHost("");
    setPort(3306);
    setUsername("");
    setPassword("");
    setShowPassword(false);
    setEncryptedPassword("");
    setDriver("mysql");
    setDatabase("");
    setSslMode("disable");
    setReadOnly(false);
    setDbSshAssetId(0);
    setParams("");
  };

  const resetRedisFields = () => {
    setHost("");
    setPort(6379);
    setUsername("");
    setPassword("");
    setShowPassword(false);
    setEncryptedPassword("");
    setTls(false);
    setRedisSshAssetId(0);
  };

  const handleTypeChange = (newType: AssetType) => {
    if (newType === assetType) return;
    setAssetType(newType);
    setPassword("");
    setShowPassword(false);
    setEncryptedPassword("");

    if (newType === "ssh") {
      setPort(22);
      setUsername("root");
      setIcon(DEFAULT_ICONS.ssh);
    } else if (newType === "database") {
      setPort(DEFAULT_PORTS[driver] || 3306);
      setUsername("");
      setIcon(DEFAULT_ICONS[driver] || "mysql");
    } else if (newType === "redis") {
      setPort(6379);
      setUsername("");
      setIcon(DEFAULT_ICONS.redis);
    }
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
    // 没有输入新密码时，传入已保存的加密密码供后端解密
    if (!password && encryptedPassword) {
      sshConfig.password = encryptedPassword;
    }
    if (connectionType === "jumphost" && jumpHostId > 0) sshConfig.jump_host_id = jumpHostId;
    if (connectionType === "proxy" && proxyHost) {
      sshConfig.proxy = { type: proxyType, host: proxyHost, port: proxyPort, username: proxyUsername || undefined, password: proxyPassword || undefined };
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
    if (readOnly) cfg.read_only = true;
    if (dbSshAssetId > 0) cfg.ssh_asset_id = dbSshAssetId;
    if (params) cfg.params = params;
    // 没有输入新密码时，传入已保存的加密密码供后端解密
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
    if (redisSshAssetId > 0) cfg.ssh_asset_id = redisSshAssetId;
    // 没有输入新密码时，传入已保存的加密密码供后端解密
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

  const encryptPassword = async (): Promise<string | undefined> => {
    if (password) {
      try {
        return await EncryptPassword(password);
      } catch {
        toast.error("Failed to encrypt password");
        return undefined;
      }
    }
    // Keep existing encrypted password if user didn't change it
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
    let config = "";

    if (assetType === "ssh") {
      const sshConfig: SSHConfig = {
        host,
        port,
        username,
        auth_type: authType,
      };

      if (authType === "password" && password) {
        const encrypted = await encryptPassword();
        if (encrypted === undefined) return;
        sshConfig.password = encrypted;
      }

      if (authType === "key") {
        if (keySource === "managed" && credentialId > 0) sshConfig.credential_id = credentialId;
        if (keySource === "file" && selectedKeyPaths.length > 0) sshConfig.private_keys = selectedKeyPaths;
      }

      if (connectionType === "jumphost" && jumpHostId > 0) sshConfig.jump_host_id = jumpHostId;
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
      const encrypted = await encryptPassword();
      if (encrypted === undefined) return;
      const dbConfig: DatabaseConfig = {
        driver,
        host,
        port,
        username,
      };
      if (encrypted) dbConfig.password = encrypted;
      if (database) dbConfig.database = database;
      if (driver === "postgresql" && sslMode !== "disable") dbConfig.ssl_mode = sslMode;
      if (readOnly) dbConfig.read_only = true;
      if (dbSshAssetId > 0) dbConfig.ssh_asset_id = dbSshAssetId;
      if (params) dbConfig.params = params;
      config = JSON.stringify(dbConfig);
    } else if (assetType === "redis") {
      const encrypted = await encryptPassword();
      if (encrypted === undefined) return;
      const redisConfig: RedisConfig = {
        host,
        port,
      };
      if (username) redisConfig.username = username;
      if (encrypted) redisConfig.password = encrypted;
      if (tls) redisConfig.tls = true;
      if (redisSshAssetId > 0) redisConfig.ssh_asset_id = redisSshAssetId;
      config = JSON.stringify(redisConfig);
    }

    const asset = new asset_entity.Asset({
      ...(editAsset || {}),
      Name: name,
      Type: assetType,
      GroupID: groupId,
      Icon: icon,
      Description: description,
      Config: config,
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

  const typeLabel = assetType === "ssh" ? t("asset.typeSSH") : assetType === "database" ? t("asset.typeDatabase") : t("asset.typeRedis");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto">
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

          {/* SSH: Connection Type + Host + Port */}
          {assetType === "ssh" && (
            <div className="grid gap-2">
              <Label>{t("asset.host")}</Label>
              <div className="flex gap-2">
                <Select
                  value={connectionType}
                  onValueChange={(v) => setConnectionType(v as "direct" | "jumphost" | "proxy")}
                >
                  <SelectTrigger className="w-[100px] shrink-0">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="direct">{t("asset.connectionDirect")}</SelectItem>
                    <SelectItem value="jumphost">{t("asset.connectionJumpHost")}</SelectItem>
                    <SelectItem value="proxy">{t("asset.connectionProxy")}</SelectItem>
                  </SelectContent>
                </Select>
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
          )}

          {/* Database / Redis: Host + Port */}
          {(assetType === "database" || assetType === "redis") && (
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
          )}

          {/* SSH: Jump Host selector */}
          {assetType === "ssh" && connectionType === "jumphost" && (
            <div className="grid gap-2">
              <Label>{t("asset.selectJumpHost")}</Label>
              <AssetSelect
                value={jumpHostId}
                onValueChange={setJumpHostId}
                filterType="ssh"
                excludeIds={jumpHostExcludeIds}
                placeholder={t("asset.jumpHostNone")}
              />
            </div>
          )}

          {/* SSH: Proxy config */}
          {assetType === "ssh" && connectionType === "proxy" && (
            <div className="grid gap-3 border rounded-lg p-3">
              <div className="grid grid-cols-3 gap-2">
                <div className="grid gap-1">
                  <Label className="text-xs">{t("asset.proxyType")}</Label>
                  <Select value={proxyType} onValueChange={setProxyType}>
                    <SelectTrigger className="h-8 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="socks5">SOCKS5</SelectItem>
                      <SelectItem value="socks4">SOCKS4</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-1">
                  <Label className="text-xs">{t("asset.proxyHost")}</Label>
                  <Input
                    className="h-8 text-xs"
                    value={proxyHost}
                    onChange={(e) => setProxyHost(e.target.value)}
                    placeholder="127.0.0.1"
                  />
                </div>
                <div className="grid gap-1">
                  <Label className="text-xs">{t("asset.proxyPort")}</Label>
                  <Input
                    className="h-8 text-xs [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                    type="number"
                    value={proxyPort}
                    onChange={(e) => setProxyPort(Number(e.target.value))}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="grid gap-1">
                  <Label className="text-xs">{t("asset.proxyUsername")}</Label>
                  <Input
                    className="h-8 text-xs"
                    value={proxyUsername}
                    onChange={(e) => setProxyUsername(e.target.value)}
                  />
                </div>
                <div className="grid gap-1">
                  <Label className="text-xs">{t("asset.proxyPassword")}</Label>
                  <Input
                    className="h-8 text-xs"
                    type="password"
                    value={proxyPassword}
                    onChange={(e) => setProxyPassword(e.target.value)}
                    placeholder={encryptedProxyPassword ? t("asset.passwordUnchanged") : ""}
                  />
                </div>
              </div>
            </div>
          )}

          {/* SSH: Username + AuthType */}
          {assetType === "ssh" && (
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label>{t("asset.username")}</Label>
                <Input
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label>{t("asset.authType")}</Label>
                <Select value={authType} onValueChange={setAuthType}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="password">
                      {t("asset.authPassword")}
                    </SelectItem>
                    <SelectItem value="key">{t("asset.authKey")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          {/* Database / Redis: Username */}
          {(assetType === "database" || assetType === "redis") && (
            <div className="grid gap-2">
              <Label>{t("asset.username")}</Label>
              <Input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={assetType === "redis" ? t("asset.username") + " (" + t("asset.databasePlaceholder").split("（")[0] + ")" : ""}
              />
            </div>
          )}

          {/* SSH: Password (when auth_type=password) */}
          {assetType === "ssh" && authType === "password" && (
            <div className="grid gap-2">
              <Label>{t("asset.password")}</Label>
              <div className="relative">
                <Input
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={encryptedPassword ? t("asset.passwordUnchanged") : t("asset.passwordPlaceholder")}
                  className="pr-9"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
                  onClick={() => setShowPassword(!showPassword)}
                >
                  {showPassword ? (
                    <EyeOff className="h-3.5 w-3.5" />
                  ) : (
                    <Eye className="h-3.5 w-3.5" />
                  )}
                </Button>
              </div>
            </div>
          )}

          {/* Database / Redis: Password */}
          {(assetType === "database" || assetType === "redis") && (
            <div className="grid gap-2">
              <Label>{t("asset.password")}</Label>
              <div className="relative">
                <Input
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={encryptedPassword ? t("asset.passwordUnchanged") : ""}
                  className="pr-9"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
                  onClick={() => setShowPassword(!showPassword)}
                >
                  {showPassword ? (
                    <EyeOff className="h-3.5 w-3.5" />
                  ) : (
                    <Eye className="h-3.5 w-3.5" />
                  )}
                </Button>
              </div>
            </div>
          )}

          {/* SSH: Key config */}
          {assetType === "ssh" && authType === "key" && (
            <div className="grid gap-3 border rounded-lg p-3">
              <div className="grid gap-2">
                <Label>{t("asset.keySource")}</Label>
                <Select
                  value={keySource}
                  onValueChange={(v) => setKeySource(v as "managed" | "file")}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="managed">
                      {t("asset.keySourceManaged")}
                    </SelectItem>
                    <SelectItem value="file">
                      {t("asset.keySourceFile")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {keySource === "managed" && (
                <div className="grid gap-2">
                  <Label>{t("asset.selectKey")}</Label>
                  {managedKeys.length > 0 ? (
                    <Select
                      value={String(credentialId)}
                      onValueChange={(v) => setCredentialId(Number(v))}
                    >
                      <SelectTrigger>
                        <SelectValue
                          placeholder={t("asset.selectKeyPlaceholder")}
                        />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="0">
                          {t("asset.selectKeyPlaceholder")}
                        </SelectItem>
                        {managedKeys.map((k) => (
                          <SelectItem key={k.id} value={String(k.id)}>
                            {k.name} ({(k.keyType || "").toUpperCase()})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <p className="text-xs text-muted-foreground">
                      {t("asset.noManagedKeys")}
                    </p>
                  )}
                </div>
              )}

              {keySource === "file" && (
                <div className="grid gap-2">
                  <Label>{t("asset.discoveredKeys")}</Label>
                  {scanningKeys ? (
                    <div className="flex items-center gap-2 text-xs text-muted-foreground py-1">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      {t("asset.scanningKeys")}
                    </div>
                  ) : localKeys.length > 0 ? (
                    <div className="grid gap-1.5">
                      {localKeys.map((k) => {
                        const selected = selectedKeyPaths.includes(k.path);
                        return (
                          <label
                            key={k.path}
                            className="flex items-center gap-2 text-xs cursor-pointer hover:bg-accent rounded px-2 py-1.5"
                          >
                            <input
                              type="checkbox"
                              checked={selected}
                              onChange={() => {
                                if (selected) {
                                  setSelectedKeyPaths(selectedKeyPaths.filter((p) => p !== k.path));
                                } else {
                                  setSelectedKeyPaths([...selectedKeyPaths, k.path]);
                                }
                              }}
                              className="rounded"
                            />
                            <span className="font-medium truncate">{k.path.split("/").pop()}</span>
                            <span className="text-muted-foreground">({k.keyType})</span>
                            <span className="text-muted-foreground truncate ml-auto" title={k.fingerprint}>
                              {k.fingerprint.substring(0, 20)}...
                            </span>
                          </label>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="text-xs text-muted-foreground">{t("asset.noLocalKeys")}</p>
                  )}

                  {selectedKeyPaths
                    .filter((p) => !localKeys.some((k) => k.path === p))
                    .map((path) => (
                      <div key={path} className="flex items-center gap-2 text-xs px-2 py-1.5 bg-accent rounded">
                        <span className="truncate flex-1">{path}</span>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-5 w-5 shrink-0"
                          onClick={() => setSelectedKeyPaths(selectedKeyPaths.filter((p2) => p2 !== path))}
                        >
                          <Trash2 className="h-3 w-3" />
                        </Button>
                      </div>
                    ))}

                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="w-full mt-1"
                    onClick={async () => {
                      try {
                        const info = await SelectSSHKeyFile();
                        if (info && !selectedKeyPaths.includes(info.path)) {
                          setSelectedKeyPaths([...selectedKeyPaths, info.path]);
                          if (!localKeys.some((k) => k.path === info.path)) {
                            setLocalKeys([...localKeys, info]);
                          }
                        }
                      } catch (e) {
                        toast.error(String(e));
                      }
                    }}
                  >
                    <FolderOpen className="h-3.5 w-3.5 mr-1.5" />
                    {t("asset.browseKeyFile")}
                  </Button>
                </div>
              )}
            </div>
          )}

          {/* Database: extra fields */}
          {assetType === "database" && (
            <>
              <div className="grid gap-2">
                <Label>{t("asset.database")}</Label>
                <Input
                  value={database}
                  onChange={(e) => setDatabase(e.target.value)}
                  placeholder={t("asset.databasePlaceholder")}
                />
              </div>

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

              <div className="grid gap-2">
                <Label>{t("asset.params")}</Label>
                <Input
                  value={params}
                  onChange={(e) => setParams(e.target.value)}
                  placeholder={t("asset.paramsPlaceholder")}
                />
              </div>

              <div className="flex items-center justify-between">
                <Label>{t("asset.readOnly")}</Label>
                <Switch checked={readOnly} onCheckedChange={setReadOnly} />
              </div>

              <div className="grid gap-2">
                <Label>{t("asset.sshTunnel")}</Label>
                <AssetSelect
                  value={dbSshAssetId}
                  onValueChange={setDbSshAssetId}
                  filterType="ssh"
                  placeholder={t("asset.sshTunnelNone")}
                />
              </div>
            </>
          )}

          {/* Redis: extra fields */}
          {assetType === "redis" && (
            <>
              <div className="flex items-center justify-between">
                <Label>{t("asset.tls")}</Label>
                <Switch checked={tls} onCheckedChange={setTls} />
              </div>

              <div className="grid gap-2">
                <Label>{t("asset.sshTunnel")}</Label>
                <AssetSelect
                  value={redisSshAssetId}
                  onValueChange={setRedisSshAssetId}
                  filterType="ssh"
                  placeholder={t("asset.sshTunnelNone")}
                />
              </div>
            </>
          )}

          {/* Test Connection */}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={
              assetType === "ssh" ? handleTestConnection
                : assetType === "database" ? handleTestDatabaseConnection
                : handleTestRedisConnection
            }
            disabled={testing || !host}
            className="gap-1 w-fit"
          >
            {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <PlugZap className="h-3.5 w-3.5" />}
            {testing ? t("asset.testing") : t("asset.testConnection")}
          </Button>

          {/* Group - Tree Selector */}
          <div className="grid gap-2">
            <Label>{t("asset.group")}</Label>
            <GroupSelect
              value={groupId}
              onValueChange={setGroupId}
            />
          </div>

          {/* Description */}
          <div className="grid gap-2">
            <Label>{t("asset.description")}</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
            />
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

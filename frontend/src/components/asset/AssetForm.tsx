import { useState, useEffect } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Plus, Trash2, ChevronDown, ChevronRight, Eye, EyeOff, FolderOpen, Loader2, Folder, Server } from "lucide-react";
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
import { TreeSelect, type TreeNode } from "@/components/ui/tree-select";
import { IconPicker } from "@/components/asset/IconPicker";
import { useAssetStore } from "@/stores/assetStore";
import { asset_entity, ssh_key_entity } from "../../../wailsjs/go/models";
import {
  SaveCredential,
  LoadCredential,
  ListSSHKeys,
  ListLocalSSHKeys,
  SelectSSHKeyFile,
} from "../../../wailsjs/go/main/App";
import { main } from "../../../wailsjs/go/models";

interface AssetFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editAsset?: asset_entity.Asset | null;
  defaultGroupId?: number;
}

interface ForwardedPort {
  type: string;
  local_host: string;
  local_port: number;
  remote_host: string;
  remote_port: number;
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
  key_id?: number;
  key_source?: string;
  private_keys?: string[];
  jump_host_id?: number;
  forwarded_ports?: ForwardedPort[];
  proxy?: ProxyConfig | null;
}

export function AssetForm({
  open,
  onOpenChange,
  editAsset,
  defaultGroupId = 0,
}: AssetFormProps) {
  const { t } = useTranslation();
  const { createAsset, updateAsset, groups, assets } = useAssetStore();

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

  // Connection type
  const [connectionType, setConnectionType] = useState<"direct" | "jumphost" | "proxy">("direct");

  // Auth fields
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [encryptedPassword, setEncryptedPassword] = useState("");
  const [keySource, setKeySource] = useState<"managed" | "file">("managed");
  const [keyId, setKeyId] = useState(0);
  const [managedKeys, setManagedKeys] = useState<ssh_key_entity.SSHKey[]>([]);

  // SSH fields - local key
  const [localKeys, setLocalKeys] = useState<main.LocalSSHKeyInfo[]>([]);
  const [selectedKeyPaths, setSelectedKeyPaths] = useState<string[]>([]);
  const [scanningKeys, setScanningKeys] = useState(false);
  const [jumpHostId, setJumpHostId] = useState(0);
  const [forwardedPorts, setForwardedPorts] = useState<ForwardedPort[]>([]);
  const [proxyType, setProxyType] = useState("socks5");
  const [proxyHost, setProxyHost] = useState("");
  const [proxyPort, setProxyPort] = useState(1080);
  const [proxyUsername, setProxyUsername] = useState("");
  const [proxyPassword, setProxyPassword] = useState("");

  // Collapsible sections
  const [showAdvanced, setShowAdvanced] = useState(false);

  // SSH assets for jump host selection (exclude self)
  const sshAssets = assets.filter(
    (a) => a.Type === "ssh" && a.ID !== editAsset?.ID
  );

  const folderIcon = <Folder className="h-3.5 w-3.5 text-muted-foreground" />;
  const serverIcon = <Server className="h-3.5 w-3.5 text-muted-foreground" />;

  // Build group tree for TreeSelect
  const buildGroupTree = (): TreeNode[] => {
    const buildChildren = (parentId: number): TreeNode[] => {
      return groups
        .filter((g) => (g.ParentID || 0) === parentId)
        .map((g) => ({
          id: g.ID,
          label: g.Name,
          icon: folderIcon,
          children: buildChildren(g.ID),
        }));
    };
    return buildChildren(0);
  };
  const groupTree = buildGroupTree();

  // Build jump host tree: groups (non-selectable) containing SSH assets (selectable)
  const buildJumpHostTree = (): TreeNode[] => {
    const nodes: TreeNode[] = [];
    // Grouped assets
    const buildGroupWithAssets = (parentId: number): TreeNode[] => {
      return groups
        .filter((g) => (g.ParentID || 0) === parentId)
        .map((g) => {
          const childGroups = buildGroupWithAssets(g.ID);
          const childAssets: TreeNode[] = sshAssets
            .filter((a) => a.GroupID === g.ID)
            .map((a) => ({ id: a.ID, label: a.Name, icon: serverIcon }));
          return {
            id: -g.ID, // negative to avoid collision with asset IDs
            label: g.Name,
            icon: folderIcon,
            selectable: false,
            children: [...childGroups, ...childAssets],
          };
        })
        .filter((g) => g.children && g.children.length > 0);
    };
    nodes.push(...buildGroupWithAssets(0));
    // Ungrouped assets
    const ungrouped = sshAssets.filter((a) => !a.GroupID || a.GroupID === 0);
    for (const a of ungrouped) {
      nodes.push({ id: a.ID, label: a.Name, icon: serverIcon });
    }
    return nodes;
  };
  const jumpHostTree = buildJumpHostTree();

  // Load managed keys and scan local keys when dialog opens
  useEffect(() => {
    if (open) {
      ListSSHKeys()
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
        setName(editAsset.Name);
        setGroupId(editAsset.GroupID);
        setIcon(editAsset.Icon || "server");
        setDescription(editAsset.Description);
        try {
          const cfg: SSHConfig = JSON.parse(editAsset.Config || "{}");
          setHost(cfg.host || "");
          setPort(cfg.port || 22);
          setUsername(cfg.username || "root");
          setAuthType(cfg.auth_type || "password");

          // Auth fields
          setEncryptedPassword(cfg.password || "");
          if (cfg.password) {
            LoadCredential(cfg.password)
              .then((p) => setPassword(p))
              .catch(() => setPassword(""));
          } else {
            setPassword("");
          }
          setKeySource((cfg.key_source === "file" || cfg.key_source === "local") ? "file" : "managed");
          setKeyId(cfg.key_id || 0);

          setSelectedKeyPaths(cfg.private_keys || []);
          setJumpHostId(cfg.jump_host_id || 0);
          setForwardedPorts(cfg.forwarded_ports || []);

          // Derive connection type
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
            setProxyPassword(cfg.proxy.password || "");
          } else {
            setProxyType("socks5");
            setProxyHost("");
            setProxyPort(1080);
            setProxyUsername("");
            setProxyPassword("");
          }

          // Show advanced if forwarded ports exist
          setShowAdvanced(
            !!(cfg.forwarded_ports && cfg.forwarded_ports.length > 0)
          );

        } catch {
          resetSSHFields();
        }
      } else {
        setName("");
        setGroupId(defaultGroupId);
        setIcon("server");
        setDescription("");
        resetSSHFields();
      }
    }
  }, [open, editAsset, defaultGroupId]);

  const resetSSHFields = () => {
    setHost("");
    setPort(22);
    setUsername("root");
    setAuthType("password");
    setPassword("");
    setShowPassword(false);
    setEncryptedPassword("");
    setKeySource("managed");
    setKeyId(0);
    setSelectedKeyPaths([]);
    setConnectionType("direct");
    setJumpHostId(0);
    setForwardedPorts([]);
    setProxyType("socks5");
    setProxyHost("");
    setProxyPort(1080);
    setProxyUsername("");
    setProxyPassword("");
    setShowAdvanced(false);
  };

  const handleSubmit = async () => {
    const sshConfig: SSHConfig = {
      host,
      port,
      username,
      auth_type: authType,
    };

    // Save password (encrypt it)
    if (authType === "password" && password) {
      try {
        const encrypted = await SaveCredential(password);
        sshConfig.password = encrypted;
      } catch {
        toast.error("Failed to encrypt password");
        return;
      }
    } else if (authType === "password" && encryptedPassword && !password) {
      // Keep existing encrypted password if user didn't change it
    }

    // Save key config
    if (authType === "key") {
      sshConfig.key_source = keySource;
      if (keySource === "managed" && keyId > 0) {
        sshConfig.key_id = keyId;
      }
      if (keySource === "file" && selectedKeyPaths.length > 0) {
        sshConfig.private_keys = selectedKeyPaths;
      }
    }

    // Connection type specific fields
    if (connectionType === "jumphost" && jumpHostId > 0) {
      sshConfig.jump_host_id = jumpHostId;
    }
    if (connectionType === "proxy" && proxyHost) {
      sshConfig.proxy = {
        type: proxyType,
        host: proxyHost,
        port: proxyPort,
        username: proxyUsername || undefined,
        password: proxyPassword || undefined,
      };
    }

    if (forwardedPorts.length > 0) sshConfig.forwarded_ports = forwardedPorts;

    const config = JSON.stringify(sshConfig);

    const asset = new asset_entity.Asset({
      ...(editAsset || {}),
      Name: name,
      Type: "ssh",
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

  const addForwardedPort = () => {
    setForwardedPorts([
      ...forwardedPorts,
      { type: "local", local_host: "127.0.0.1", local_port: 0, remote_host: "127.0.0.1", remote_port: 0 },
    ]);
  };

  const removeForwardedPort = (index: number) => {
    setForwardedPorts(forwardedPorts.filter((_, i) => i !== index));
  };

  const updateForwardedPort = (index: number, field: keyof ForwardedPort, value: string | number) => {
    const updated = [...forwardedPorts];
    updated[index] = { ...updated[index], [field]: value };
    setForwardedPorts(updated);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {editAsset ? t("action.edit") : t("action.add")} SSH{" "}
            {t("asset.title")}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          {/* Name */}
          <div className="grid gap-2">
            <Label>{t("asset.name")}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="web-01"
            />
          </div>

          {/* Icon */}
          <div className="grid gap-2">
            <Label>{t("asset.icon")}</Label>
            <IconPicker value={icon} onChange={setIcon} type="asset" />
          </div>

          {/* Connection Type + Host + Port */}
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
                className="w-[80px] shrink-0"
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
              />
            </div>
          </div>

          {/* Jump Host selector (when connectionType is jumphost) */}
          {connectionType === "jumphost" && (
            <div className="grid gap-2">
              <Label>{t("asset.selectJumpHost")}</Label>
              <TreeSelect
                value={jumpHostId}
                onValueChange={setJumpHostId}
                nodes={jumpHostTree}
                placeholder={t("asset.jumpHostNone")}
                placeholderIcon={serverIcon}
              />
            </div>
          )}

          {/* Proxy config (when connectionType is proxy) */}
          {connectionType === "proxy" && (
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
                    className="h-8 text-xs"
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
                  />
                </div>
              </div>
            </div>
          )}

          {/* Username + AuthType */}
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

          {/* Auth-type specific fields */}
          {authType === "password" && (
            <div className="grid gap-2">
              <Label>{t("asset.password")}</Label>
              <div className="relative">
                <Input
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t("asset.passwordPlaceholder")}
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

          {authType === "key" && (
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
                      value={String(keyId)}
                      onValueChange={(v) => setKeyId(Number(v))}
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
                            {k.name} ({k.keyType.toUpperCase()})
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

                  {/* 已选的非发现密钥（手动浏览添加的） */}
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
                          onClick={() => setSelectedKeyPaths(selectedKeyPaths.filter((p) => p !== path))}
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

          {/* Group - Tree Selector */}
          <div className="grid gap-2">
            <Label>{t("asset.group")}</Label>
            <TreeSelect
              value={groupId}
              onValueChange={setGroupId}
              nodes={groupTree}
              placeholder={t("asset.ungrouped")}
              placeholderIcon={folderIcon}
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

          {/* Advanced section - only forwarded ports */}
          <button
            type="button"
            className="flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
            onClick={() => setShowAdvanced(!showAdvanced)}
          >
            {showAdvanced ? (
              <ChevronDown className="h-3.5 w-3.5" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" />
            )}
            {t("asset.forwardedPorts")}
          </button>

          {showAdvanced && (
            <div className="grid gap-4 border rounded-lg p-3">
              {/* Forwarded Ports */}
              <div className="grid gap-2">
                <div className="flex items-center justify-between">
                  <Label>{t("asset.forwardedPorts")}</Label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-6 gap-1 text-xs"
                    onClick={addForwardedPort}
                  >
                    <Plus className="h-3 w-3" />
                    {t("asset.addForwardedPort")}
                  </Button>
                </div>
                {forwardedPorts.map((fp, i) => (
                  <div key={i} className="flex items-center gap-1.5">
                    <Select
                      value={fp.type}
                      onValueChange={(v) => updateForwardedPort(i, "type", v)}
                    >
                      <SelectTrigger className="h-7 w-20 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="local">Local</SelectItem>
                        <SelectItem value="remote">Remote</SelectItem>
                        <SelectItem value="dynamic">Dynamic</SelectItem>
                      </SelectContent>
                    </Select>
                    <Input
                      className="h-7 text-xs w-20"
                      value={fp.local_host}
                      onChange={(e) => updateForwardedPort(i, "local_host", e.target.value)}
                      placeholder="127.0.0.1"
                    />
                    <Input
                      className="h-7 text-xs w-14"
                      type="number"
                      value={fp.local_port || ""}
                      onChange={(e) => updateForwardedPort(i, "local_port", Number(e.target.value))}
                      placeholder="Port"
                    />
                    <span className="text-xs text-muted-foreground">→</span>
                    <Input
                      className="h-7 text-xs w-20"
                      value={fp.remote_host}
                      onChange={(e) => updateForwardedPort(i, "remote_host", e.target.value)}
                      placeholder="127.0.0.1"
                    />
                    <Input
                      className="h-7 text-xs w-14"
                      type="number"
                      value={fp.remote_port || ""}
                      onChange={(e) => updateForwardedPort(i, "remote_port", Number(e.target.value))}
                      placeholder="Port"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-6 w-6 shrink-0"
                      onClick={() => removeForwardedPort(i)}
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}
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

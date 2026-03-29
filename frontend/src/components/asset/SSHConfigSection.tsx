import { Trash2, FolderOpen, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import { AssetSelect } from "@/components/asset/AssetSelect";
import { PasswordSourceField } from "@/components/asset/PasswordSourceField";
import { SelectSSHKeyFile } from "../../../wailsjs/go/app/App";
import { credential_entity } from "../../../wailsjs/go/models";
import { app } from "../../../wailsjs/go/models";

export interface SSHConfigSectionProps {
  host: string;
  setHost: (v: string) => void;
  port: number;
  setPort: (v: number) => void;
  username: string;
  setUsername: (v: string) => void;
  authType: string;
  setAuthType: (v: string) => void;
  connectionType: "direct" | "jumphost" | "proxy";
  setConnectionType: (v: "direct" | "jumphost" | "proxy") => void;
  // Password fields
  password: string;
  setPassword: (v: string) => void;
  encryptedPassword: string;
  passwordSource: "inline" | "managed";
  setPasswordSource: (v: "inline" | "managed") => void;
  passwordCredentialId: number;
  setPasswordCredentialId: (v: number) => void;
  managedPasswords: credential_entity.Credential[];
  // Key fields
  keySource: "managed" | "file";
  setKeySource: (v: "managed" | "file") => void;
  credentialId: number;
  setCredentialId: (v: number) => void;
  managedKeys: credential_entity.Credential[];
  localKeys: app.LocalSSHKeyInfo[];
  setLocalKeys: (v: app.LocalSSHKeyInfo[]) => void;
  selectedKeyPaths: string[];
  setSelectedKeyPaths: (v: string[]) => void;
  scanningKeys: boolean;
  // SSH tunnel (jump host)
  sshTunnelId: number;
  setSshTunnelId: (v: number) => void;
  jumpHostExcludeIds?: number[];
  // Proxy
  proxyType: string;
  setProxyType: (v: string) => void;
  proxyHost: string;
  setProxyHost: (v: string) => void;
  proxyPort: number;
  setProxyPort: (v: number) => void;
  proxyUsername: string;
  setProxyUsername: (v: string) => void;
  proxyPassword: string;
  setProxyPassword: (v: string) => void;
  encryptedProxyPassword: string;
  editAssetId?: number;
}

export function SSHConfigSection({
  host,
  setHost,
  port,
  setPort,
  username,
  setUsername,
  authType,
  setAuthType,
  connectionType,
  setConnectionType,
  password,
  setPassword,
  encryptedPassword,
  passwordSource,
  setPasswordSource,
  passwordCredentialId,
  setPasswordCredentialId,
  managedPasswords,
  keySource,
  setKeySource,
  credentialId,
  setCredentialId,
  managedKeys,
  localKeys,
  setLocalKeys,
  selectedKeyPaths,
  setSelectedKeyPaths,
  scanningKeys,
  sshTunnelId,
  setSshTunnelId,
  jumpHostExcludeIds,
  proxyType,
  setProxyType,
  proxyHost,
  setProxyHost,
  proxyPort,
  setProxyPort,
  proxyUsername,
  setProxyUsername,
  proxyPassword,
  setProxyPassword,
  encryptedProxyPassword,
  editAssetId,
}: SSHConfigSectionProps) {
  const { t } = useTranslation();

  return (
    <>
      {/* SSH: Connection Type + Host + Port */}
      <div className="grid gap-2">
        <Label>{t("asset.host")}</Label>
        <div className="flex gap-2">
          <Select value={connectionType} onValueChange={(v) => setConnectionType(v as "direct" | "jumphost" | "proxy")}>
            <SelectTrigger className="w-[100px] shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="direct">{t("asset.connectionDirect")}</SelectItem>
              <SelectItem value="jumphost">{t("asset.connectionJumpHost")}</SelectItem>
              <SelectItem value="proxy">{t("asset.connectionProxy")}</SelectItem>
            </SelectContent>
          </Select>
          <Input className="flex-1" value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.1" />
          <Input
            className="w-[80px] shrink-0 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            type="number"
            value={port}
            onChange={(e) => setPort(Number(e.target.value))}
          />
        </div>
      </div>

      {/* SSH: Jump Host selector */}
      {connectionType === "jumphost" && (
        <div className="grid gap-2">
          <Label>{t("asset.selectJumpHost")}</Label>
          <AssetSelect
            value={sshTunnelId}
            onValueChange={setSshTunnelId}
            filterType="ssh"
            excludeIds={jumpHostExcludeIds}
            placeholder={t("asset.jumpHostNone")}
          />
        </div>
      )}

      {/* SSH: Proxy config */}
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
              <Input className="h-8 text-xs" value={proxyUsername} onChange={(e) => setProxyUsername(e.target.value)} />
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
      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-2">
          <Label>{t("asset.username")}</Label>
          <Input value={username} onChange={(e) => setUsername(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label>{t("asset.authType")}</Label>
          <Select value={authType} onValueChange={setAuthType}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="password">{t("asset.authPassword")}</SelectItem>
              <SelectItem value="key">{t("asset.authKey")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* SSH: Password (when auth_type=password) */}
      {authType === "password" && (
        <PasswordSourceField
          source={passwordSource}
          onSourceChange={setPasswordSource}
          password={password}
          onPasswordChange={setPassword}
          credentialId={passwordCredentialId}
          onCredentialIdChange={setPasswordCredentialId}
          managedPasswords={managedPasswords}
          placeholder={t("asset.passwordPlaceholder")}
          hasExistingPassword={!!encryptedPassword}
          editAssetId={editAssetId}
        />
      )}

      {/* SSH: Key config */}
      {authType === "key" && (
        <div className="grid gap-3 border rounded-lg p-3">
          <div className="grid gap-2">
            <Label>{t("asset.keySource")}</Label>
            <Select value={keySource} onValueChange={(v) => setKeySource(v as "managed" | "file")}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="managed">{t("asset.keySourceManaged")}</SelectItem>
                <SelectItem value="file">{t("asset.keySourceFile")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {keySource === "managed" && (
            <div className="grid gap-2">
              <Label>{t("asset.selectKey")}</Label>
              {managedKeys.length > 0 ? (
                <Select value={String(credentialId)} onValueChange={(v) => setCredentialId(Number(v))}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("asset.selectKeyPlaceholder")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="0">{t("asset.selectKeyPlaceholder")}</SelectItem>
                    {managedKeys.map((k) => (
                      <SelectItem key={k.id} value={String(k.id)}>
                        {k.name} ({(k.keyType || "").toUpperCase()})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <p className="text-xs text-muted-foreground">{t("asset.noManagedKeys")}</p>
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
    </>
  );
}

import { useState, useEffect, useCallback } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Plus, Trash2, Copy, Key, FileKey, Download, Pencil, Lock, KeyRound, Eye, EyeOff, Shuffle } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Button,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  Input,
  Label,
  Textarea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import {
  ListCredentials,
  GenerateSSHKey,
  ImportSSHKeyFile,
  ImportSSHKeyPEM,
  UpdateCredential,
  DeleteCredential,
  GetCredentialPublicKey,
  GetCredentialUsage,
  CreatePasswordCredential,
  UpdateCredentialPassword,
  UpdateCredentialPassphrase,
} from "../../../wailsjs/go/app/App";
import { credential_entity } from "../../../wailsjs/go/models";

function generatePassword(length = 20): string {
  const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*";
  const values = crypto.getRandomValues(new Uint8Array(length));
  return Array.from(values, (v) => charset[v % charset.length]).join("");
}

export function CredentialManager() {
  const { t } = useTranslation();
  const [credentials, setCredentials] = useState<credential_entity.Credential[]>([]);
  const [loading, setLoading] = useState(false);
  const [generateOpen, setGenerateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [createPasswordOpen, setCreatePasswordOpen] = useState(false);
  const [editingCred, setEditingCred] = useState<credential_entity.Credential | null>(null);
  const [deleteCred, setDeleteCred] = useState<credential_entity.Credential | null>(null);
  const [deleteUsage, setDeleteUsage] = useState<string[]>([]);
  const [changePasswordCred, setChangePasswordCred] = useState<credential_entity.Credential | null>(null);
  const [changePassphraseCred, setChangePassphraseCred] = useState<credential_entity.Credential | null>(null);

  const fetchCredentials = useCallback(async () => {
    setLoading(true);
    try {
      const result = await ListCredentials();
      setCredentials(result || []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchCredentials();
  }, [fetchCredentials]);

  const handleDeleteClick = async (cred: credential_entity.Credential) => {
    try {
      const usage = await GetCredentialUsage(cred.id);
      setDeleteUsage(usage || []);
    } catch {
      setDeleteUsage([]);
    }
    setDeleteCred(cred);
  };

  const handleDeleteConfirm = async () => {
    if (!deleteCred) return;
    try {
      await DeleteCredential(deleteCred.id);
      toast.success(deleteCred.type === "ssh_key" ? t("sshKey.deleteSuccess") : t("credential.deleteSuccess"));
      setDeleteCred(null);
      fetchCredentials();
    } catch (e) {
      toast.error(String(e));
    }
  };

  const handleCopyPublicKey = async (id: number) => {
    try {
      const pubKey = await GetCredentialPublicKey(id);
      await navigator.clipboard.writeText(pubKey);
      toast.success(t("sshKey.copied"));
    } catch (e) {
      toast.error(String(e));
    }
  };

  const keyTypeLabel = (keyType: string, keySize: number) => {
    switch (keyType) {
      case "rsa":
        return `RSA${keySize ? ` ${keySize}` : ""}`;
      case "ed25519":
        return "ED25519";
      case "ecdsa":
        return `ECDSA${keySize ? ` P-${keySize}` : ""}`;
      default:
        return keyType;
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setImportOpen(true)}>
            <Download className="h-3.5 w-3.5" />
            {t("sshKey.import")}
          </Button>
          <Button variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setGenerateOpen(true)}>
            <KeyRound className="h-3.5 w-3.5" />
            {t("sshKey.generate")}
          </Button>
          <Button size="sm" className="h-8 gap-1.5" onClick={() => setCreatePasswordOpen(true)}>
            <Plus className="h-3.5 w-3.5" />
            {t("credential.createPassword")}
          </Button>
        </div>
      </div>

      {credentials.length === 0 && !loading ? (
        <div className="text-center py-8 text-sm text-muted-foreground">
          <Key className="h-8 w-8 mx-auto mb-2 opacity-30" />
          {t("sshKey.empty")}
        </div>
      ) : (
        <div className="space-y-2">
          {credentials.map((cred) => (
            <div key={cred.id} className="flex items-center justify-between p-3 rounded-lg border bg-card group">
              <div className="flex items-center gap-3 min-w-0">
                {cred.type === "ssh_key" ? (
                  <FileKey className="h-4 w-4 text-muted-foreground shrink-0" />
                ) : (
                  <Lock className="h-4 w-4 text-muted-foreground shrink-0" />
                )}
                <div className="min-w-0">
                  <div className="text-sm font-medium truncate">
                    {cred.name}
                    {cred.type === "ssh_key" && cred.comment && cred.comment !== cred.name && (
                      <span className="ml-2 text-xs text-muted-foreground font-normal">({cred.comment})</span>
                    )}
                  </div>
                  <div className="text-xs text-muted-foreground flex gap-2">
                    {cred.type === "ssh_key" ? (
                      <>
                        <span>{keyTypeLabel(cred.keyType || "", cred.keySize || 0)}</span>
                        <span className="font-mono truncate max-w-48">{cred.fingerprint}</span>
                      </>
                    ) : (
                      <>
                        {cred.username && <span>{cred.username}</span>}
                        {cred.description && <span className="truncate max-w-48">{cred.description}</span>}
                        {!cred.username && !cred.description && <span>{t("credential.passwords")}</span>}
                      </>
                    )}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                {cred.type === "ssh_key" && (
                  <>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => handleCopyPublicKey(cred.id)}
                        >
                          <Copy className="h-3.5 w-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t("sshKey.copyPublicKey")}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => setChangePassphraseCred(cred)}
                        >
                          <KeyRound className="h-3.5 w-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t("sshKey.changePassphrase")}</TooltipContent>
                    </Tooltip>
                  </>
                )}
                {cred.type === "password" && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() => setChangePasswordCred(cred)}
                      >
                        <KeyRound className="h-3.5 w-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t("credential.changePassword")}</TooltipContent>
                  </Tooltip>
                )}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setEditingCred(cred)}>
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t("action.edit")}</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-muted-foreground hover:text-destructive"
                      onClick={() => handleDeleteClick(cred)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t("action.delete")}</TooltipContent>
                </Tooltip>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Dialogs */}
      <GenerateKeyDialog open={generateOpen} onOpenChange={setGenerateOpen} onSuccess={fetchCredentials} />
      <ImportKeyDialog open={importOpen} onOpenChange={setImportOpen} onSuccess={fetchCredentials} />
      <CreatePasswordDialog
        open={createPasswordOpen}
        onOpenChange={setCreatePasswordOpen}
        onSuccess={fetchCredentials}
      />
      <EditCredentialDialog
        open={!!editingCred}
        onOpenChange={(open) => !open && setEditingCred(null)}
        credential={editingCred}
        onSuccess={fetchCredentials}
      />
      <ChangePasswordDialog
        open={!!changePasswordCred}
        onOpenChange={(open) => !open && setChangePasswordCred(null)}
        credential={changePasswordCred}
        onSuccess={fetchCredentials}
      />
      <ChangePassphraseDialog
        open={!!changePassphraseCred}
        onOpenChange={(open) => !open && setChangePassphraseCred(null)}
        credential={changePassphraseCred}
        onSuccess={fetchCredentials}
      />

      {/* Delete confirmation */}
      <Dialog open={!!deleteCred} onOpenChange={(open) => !open && setDeleteCred(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("credential.deleteConfirmTitle")}</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            {deleteUsage.length > 0
              ? t("credential.deleteConfirmUsage", {
                  name: deleteCred?.name,
                  assets: deleteUsage.join(", "),
                })
              : t("credential.deleteConfirm", { name: deleteCred?.name })}
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteCred(null)}>
              {t("action.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDeleteConfirm}>
              {t("action.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function GenerateKeyDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [comment, setComment] = useState("");
  const [username, setUsername] = useState("");
  const [keyType, setKeyType] = useState("ed25519");
  const [keySize, setKeySize] = useState(4096);
  const [passphrase, setPassphrase] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      setName("");
      setComment("");
      setUsername("");
      setKeyType("ed25519");
      setKeySize(4096);
      setPassphrase("");
    }
  }, [open]);

  const handleGenerate = async () => {
    setSaving(true);
    try {
      await GenerateSSHKey(name, comment, keyType, keySize, passphrase, username);
      toast.success(t("sshKey.generateSuccess"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  const sizeOptions = () => {
    switch (keyType) {
      case "rsa":
        return [
          { value: 2048, label: "2048 bits" },
          { value: 4096, label: "4096 bits" },
        ];
      case "ecdsa":
        return [
          { value: 256, label: "P-256" },
          { value: 384, label: "P-384" },
          { value: 521, label: "P-521" },
        ];
      default:
        return [];
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("sshKey.generateTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{t("sshKey.name")}</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("sshKey.namePlaceholder")} />
          </div>
          <div className="grid gap-2">
            <Label>{t("sshKey.comment")}</Label>
            <Input
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder={t("sshKey.commentPlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("credential.username")}</Label>
            <Input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t("credential.usernamePlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("sshKey.type")}</Label>
            <Select value={keyType} onValueChange={setKeyType}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ed25519">ED25519</SelectItem>
                <SelectItem value="rsa">RSA</SelectItem>
                <SelectItem value="ecdsa">ECDSA</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {sizeOptions().length > 0 && (
            <div className="grid gap-2">
              <Label>{t("sshKey.size")}</Label>
              <Select value={String(keySize)} onValueChange={(v) => setKeySize(Number(v))}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {sizeOptions().map((opt) => (
                    <SelectItem key={opt.value} value={String(opt.value)}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          <div className="grid gap-2">
            <Label>{t("sshKey.passphrase")}</Label>
            <Input
              type="password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              placeholder={t("sshKey.passphrasePlaceholderOptional")}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleGenerate} disabled={saving || !name}>
            {saving ? t("sshKey.generating") : t("sshKey.generate")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ImportKeyDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [comment, setComment] = useState("");
  const [username, setUsername] = useState("");
  const [pemContent, setPemContent] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [mode, setMode] = useState<"file" | "pem">("file");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      setName("");
      setComment("");
      setUsername("");
      setPemContent("");
      setPassphrase("");
      setMode("file");
    }
  }, [open]);

  const handleImportFile = async () => {
    setSaving(true);
    try {
      const result = await ImportSSHKeyFile(name, comment, passphrase, username);
      if (result) {
        toast.success(t("sshKey.importSuccess"));
        onOpenChange(false);
        onSuccess();
      }
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  const handleImportPEM = async () => {
    setSaving(true);
    try {
      await ImportSSHKeyPEM(name, comment, pemContent, passphrase, username);
      toast.success(t("sshKey.importSuccess"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("sshKey.importTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{t("sshKey.name")}</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("sshKey.namePlaceholder")} />
          </div>
          <div className="grid gap-2">
            <Label>{t("sshKey.comment")}</Label>
            <Input
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder={t("sshKey.commentPlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("credential.username")}</Label>
            <Input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t("credential.usernamePlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("sshKey.passphrase")}</Label>
            <Input
              type="password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              placeholder={t("sshKey.passphrasePlaceholder")}
            />
          </div>
          <div className="flex gap-2">
            <Button variant={mode === "file" ? "default" : "outline"} size="sm" onClick={() => setMode("file")}>
              {t("sshKey.importFile")}
            </Button>
            <Button variant={mode === "pem" ? "default" : "outline"} size="sm" onClick={() => setMode("pem")}>
              {t("sshKey.importPEM")}
            </Button>
          </div>
          {mode === "pem" && (
            <div className="grid gap-2">
              <Textarea
                value={pemContent}
                onChange={(e) => setPemContent(e.target.value)}
                placeholder={t("sshKey.pemPlaceholder")}
                rows={6}
                className="font-mono text-xs"
              />
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          {mode === "file" ? (
            <Button onClick={handleImportFile} disabled={saving || !name}>
              {saving ? t("sshKey.importing") : t("sshKey.importFile")}
            </Button>
          ) : (
            <Button onClick={handleImportPEM} disabled={saving || !name || !pemContent}>
              {saving ? t("sshKey.importing") : t("sshKey.import")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CreatePasswordDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (open) {
      setName("");
      setUsername("");
      setPassword("");
      setDescription("");
      setVisible(false);
    }
  }, [open]);

  const handleCreate = async () => {
    setSaving(true);
    try {
      await CreatePasswordCredential(name, username, password, description);
      toast.success(t("credential.createSuccess"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("credential.createPasswordTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{t("credential.name")}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("credential.namePlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("credential.username")}</Label>
            <Input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t("credential.usernamePlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("credential.password")}</Label>
            <div className="relative">
              <Input
                type={visible ? "text" : "password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t("credential.passwordPlaceholder")}
                className="pr-18"
              />
              <div className="absolute right-1 top-1/2 -translate-y-1/2 flex">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  onClick={() => setVisible(!visible)}
                >
                  {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  title={t("credential.randomPassword")}
                  onClick={() => {
                    setPassword(generatePassword());
                    setVisible(true);
                  }}
                >
                  <Shuffle className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </div>
          <div className="grid gap-2">
            <Label>{t("credential.description")}</Label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t("credential.descriptionPlaceholder")}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleCreate} disabled={saving || !name || !password}>
            {saving ? t("action.saving") : t("action.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditCredentialDialog({
  open,
  onOpenChange,
  credential,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  credential: credential_entity.Credential | null;
  onSuccess: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [comment, setComment] = useState("");
  const [description, setDescription] = useState("");
  const [username, setUsername] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open && credential) {
      setName(credential.name);
      setComment(credential.comment || "");
      setDescription(credential.description || "");
      setUsername(credential.username || "");
    }
  }, [open, credential]);

  const handleSave = async () => {
    if (!credential) return;
    setSaving(true);
    try {
      await UpdateCredential(credential.id, name, comment, description, username);
      toast.success(credential.type === "ssh_key" ? t("sshKey.updateSuccess") : t("credential.updateSuccess"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  const isSSHKey = credential?.type === "ssh_key";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{isSSHKey ? t("sshKey.editTitle") : t("credential.editPasswordTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{isSSHKey ? t("sshKey.name") : t("credential.name")}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={isSSHKey ? t("sshKey.namePlaceholder") : t("credential.namePlaceholder")}
            />
          </div>
          {isSSHKey ? (
            <>
              <div className="grid gap-2">
                <Label>{t("sshKey.comment")}</Label>
                <Input
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder={t("sshKey.commentPlaceholder")}
                />
              </div>
              <div className="grid gap-2">
                <Label>{t("credential.username")}</Label>
                <Input
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={t("credential.usernamePlaceholder")}
                />
              </div>
            </>
          ) : (
            <>
              <div className="grid gap-2">
                <Label>{t("credential.username")}</Label>
                <Input
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={t("credential.usernamePlaceholder")}
                />
              </div>
              <div className="grid gap-2">
                <Label>{t("credential.description")}</Label>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={t("credential.descriptionPlaceholder")}
                />
              </div>
            </>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving || !name}>
            {saving ? t("action.saving") : t("action.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ChangePasswordDialog({
  open,
  onOpenChange,
  credential,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  credential: credential_entity.Credential | null;
  onSuccess: () => void;
}) {
  const { t } = useTranslation();
  const [password, setPassword] = useState("");
  const [saving, setSaving] = useState(false);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (open) {
      setPassword("");
      setVisible(false);
    }
  }, [open]);

  const handleSave = async () => {
    if (!credential) return;
    setSaving(true);
    try {
      await UpdateCredentialPassword(credential.id, password);
      toast.success(t("credential.passwordChanged"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("credential.changePasswordTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{t("credential.newPassword")}</Label>
            <div className="relative">
              <Input
                type={visible ? "text" : "password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t("credential.newPasswordPlaceholder")}
                className="pr-18"
              />
              <div className="absolute right-1 top-1/2 -translate-y-1/2 flex">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  onClick={() => setVisible(!visible)}
                >
                  {visible ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7"
                  title={t("credential.randomPassword")}
                  onClick={() => {
                    setPassword(generatePassword());
                    setVisible(true);
                  }}
                >
                  <Shuffle className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving || !password}>
            {saving ? t("action.saving") : t("action.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ChangePassphraseDialog({
  open,
  onOpenChange,
  credential,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  credential: credential_entity.Credential | null;
  onSuccess: () => void;
}) {
  const { t } = useTranslation();
  const [oldPassphrase, setOldPassphrase] = useState("");
  const [newPassphrase, setNewPassphrase] = useState("");
  const [confirmPassphrase, setConfirmPassphrase] = useState("");
  const [saving, setSaving] = useState(false);
  const [visibleOld, setVisibleOld] = useState(false);
  const [visibleNew, setVisibleNew] = useState(false);
  const [visibleConfirm, setVisibleConfirm] = useState(false);

  useEffect(() => {
    if (open) {
      setOldPassphrase("");
      setNewPassphrase("");
      setConfirmPassphrase("");
      setVisibleOld(false);
      setVisibleNew(false);
      setVisibleConfirm(false);
    }
  }, [open]);

  const handleSave = async () => {
    if (!credential) return;
    if (newPassphrase !== confirmPassphrase) {
      toast.error(t("sshKey.passphraseMismatch"));
      return;
    }
    setSaving(true);
    try {
      await UpdateCredentialPassphrase(credential.id, oldPassphrase, newPassphrase);
      toast.success(t("sshKey.passphraseChanged"));
      onOpenChange(false);
      onSuccess();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("sshKey.changePassphraseTitle")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{t("sshKey.oldPassphrase")}</Label>
            <p className="text-xs text-muted-foreground">{t("sshKey.oldPassphraseHint")}</p>
            <div className="relative">
              <Input
                type={visibleOld ? "text" : "password"}
                value={oldPassphrase}
                onChange={(e) => setOldPassphrase(e.target.value)}
                placeholder={t("sshKey.passphrasePlaceholder")}
                className="pr-9"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
                onClick={() => setVisibleOld(!visibleOld)}
              >
                {visibleOld ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
              </Button>
            </div>
          </div>
          <div className="grid gap-2">
            <Label>{t("sshKey.newPassphrase")}</Label>
            <div className="relative">
              <Input
                type={visibleNew ? "text" : "password"}
                value={newPassphrase}
                onChange={(e) => setNewPassphrase(e.target.value)}
                placeholder={t("sshKey.passphrasePlaceholder")}
                className="pr-9"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
                onClick={() => setVisibleNew(!visibleNew)}
              >
                {visibleNew ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
              </Button>
            </div>
          </div>
          <div className="grid gap-2">
            <Label>{t("sshKey.confirmPassphrase")}</Label>
            <div className="relative">
              <Input
                type={visibleConfirm ? "text" : "password"}
                value={confirmPassphrase}
                onChange={(e) => setConfirmPassphrase(e.target.value)}
                placeholder={t("sshKey.passphrasePlaceholder")}
                className="pr-9"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
                onClick={() => setVisibleConfirm(!visibleConfirm)}
              >
                {visibleConfirm ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
              </Button>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? t("action.saving") : t("action.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

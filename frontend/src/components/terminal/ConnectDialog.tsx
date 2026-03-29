import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Button,
  Input,
  Label,
} from "@opskat/ui";

interface ConnectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetName: string;
  authType: string;
  onConnect: (password: string, updatePassword: boolean) => Promise<void>;
  authRetry?: boolean;
}

export function ConnectDialog({ open, onOpenChange, assetName, authType, onConnect, authRetry }: ConnectDialogProps) {
  const { t } = useTranslation();
  const [password, setPassword] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [updatePassword, setUpdatePassword] = useState(true);
  const [error, setError] = useState("");

  const handleConnect = async () => {
    setConnecting(true);
    setError("");
    try {
      await onConnect(password, updatePassword);
      onOpenChange(false);
      setPassword("");
      setError("");
    } catch (e) {
      setError(String(e).replace(/AUTH_FAILED:/, ""));
    } finally {
      setConnecting(false);
    }
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      setPassword("");
      setError("");
    }
    onOpenChange(v);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>
            {t("ssh.connect")} - {assetName}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          {authRetry && <p className="text-sm text-destructive">{t("ssh.authFailed")}</p>}
          {(authType === "password" || authRetry) && (
            <div className="grid gap-2">
              <Label>{t("ssh.password")}</Label>
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && !connecting && handleConnect()}
                autoFocus
              />
            </div>
          )}
          {authRetry && (
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={updatePassword}
                onChange={(e) => setUpdatePassword(e.target.checked)}
                className="rounded border-input"
              />
              <span className="text-sm">{t("ssh.updatePassword")}</span>
            </label>
          )}
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleConnect} disabled={connecting}>
            {connecting ? t("ssh.connecting") : t("ssh.connect")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

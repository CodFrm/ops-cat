import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
} from "@opskat/ui";
import { formatMode, modeToOctal, modeToPerms, parseOctal, permsToMode, type PermBits, type Perms } from "./utils";
import { type PermissionTarget } from "./types";

interface PermissionDialogProps {
  target: PermissionTarget | null;
  submitting: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: (mode: number, recursive: boolean) => void;
}

const ROLES: Array<{ key: keyof Perms; labelKey: string }> = [
  { key: "u", labelKey: "sftp.permOwner" },
  { key: "g", labelKey: "sftp.permGroup" },
  { key: "o", labelKey: "sftp.permOther" },
];

const BITS: Array<{ key: keyof PermBits; labelKey: string }> = [
  { key: "r", labelKey: "sftp.permRead" },
  { key: "w", labelKey: "sftp.permWrite" },
  { key: "x", labelKey: "sftp.permExec" },
];

export function PermissionDialog({ target, submitting, error, onClose, onConfirm }: PermissionDialogProps) {
  const { t } = useTranslation();
  const [octal, setOctal] = useState("");
  const [perms, setPerms] = useState<Perms>(() => modeToPerms(0));
  const [recursive, setRecursive] = useState(false);
  const [octalError, setOctalError] = useState<string | null>(null);

  useEffect(() => {
    if (!target) return;
    setOctal(modeToOctal(target.mode));
    setPerms(modeToPerms(target.mode));
    setRecursive(false);
    setOctalError(null);
  }, [target]);

  const applyOctal = (raw: string) => {
    setOctal(raw);
    const parsed = parseOctal(raw);
    if (parsed === null) {
      setOctalError(t("sftp.errOctal"));
      return;
    }
    setOctalError(null);
    setPerms(modeToPerms(parsed));
  };

  const toggleBit = (role: keyof Perms, bit: keyof PermBits) => {
    const next: Perms = {
      ...perms,
      [role]: { ...perms[role], [bit]: !perms[role][bit] },
    };
    setPerms(next);
    const m = permsToMode(next);
    setOctal(modeToOctal(m));
    setOctalError(null);
  };

  const handleSubmit = () => {
    const parsed = parseOctal(octal);
    if (parsed === null) {
      setOctalError(t("sftp.errOctal"));
      return;
    }
    onConfirm(parsed, recursive);
  };

  const preview = target ? formatMode(permsToMode(perms), target.isDir) : "";

  return (
    <Dialog
      open={!!target}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-base">{t("sftp.permissionsOf", { name: target?.name ?? "" })}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="flex items-center gap-3">
            <Label htmlFor="perm-octal" className="text-xs shrink-0">
              {t("sftp.permOctal")}
            </Label>
            <Input
              id="perm-octal"
              value={octal}
              onChange={(e) => applyOctal(e.target.value)}
              disabled={submitting}
              className="h-7 w-24 font-mono text-sm"
              maxLength={4}
              autoFocus
            />
            <span className="text-xs text-muted-foreground font-mono">{preview}</span>
          </div>

          <div className="grid grid-cols-[auto_repeat(3,minmax(0,1fr))] gap-x-3 gap-y-2 items-center">
            <span />
            {BITS.map((b) => (
              <span key={b.key} className="text-xs text-muted-foreground text-center">
                {t(b.labelKey)}
              </span>
            ))}
            {ROLES.map((role) => (
              <RoleRow
                key={role.key}
                roleLabel={t(role.labelKey)}
                bits={perms[role.key]}
                disabled={submitting}
                onToggle={(bit) => toggleBit(role.key, bit)}
              />
            ))}
          </div>

          {target?.isDir && (
            <label className="flex items-center gap-2 text-xs cursor-pointer select-none">
              <Checkbox checked={recursive} onCheckedChange={(v) => setRecursive(v === true)} disabled={submitting} />
              {t("sftp.permRecursive")}
            </label>
          )}

          {(error || octalError) && <span className="text-destructive text-xs break-all">{error || octalError}</span>}
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose} disabled={submitting}>
            {t("action.cancel")}
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={submitting || !!octalError}>
            {t("action.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface RoleRowProps {
  roleLabel: string;
  bits: PermBits;
  disabled: boolean;
  onToggle: (bit: keyof PermBits) => void;
}

function RoleRow({ roleLabel, bits, disabled, onToggle }: RoleRowProps) {
  return (
    <>
      <span className="text-xs">{roleLabel}</span>
      {BITS.map((b) => (
        <div key={b.key} className="flex items-center justify-center">
          <Checkbox checked={bits[b.key]} onCheckedChange={() => onToggle(b.key)} disabled={disabled} />
        </div>
      ))}
    </>
  );
}

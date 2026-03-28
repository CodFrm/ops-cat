import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Eye, EyeOff, Loader2 } from "lucide-react";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { extLocalized } from "@/stores/extensionStore";
import { GetExtensionAssetPassword } from "../../../wailsjs/go/app/App";

interface JSONSchemaProperty {
  type?: string;
  format?: string;
  title?: string;
  title_zh?: string;
  description?: string;
  description_zh?: string;
  enum?: string[];
}

interface JSONSchema {
  properties?: Record<string, JSONSchemaProperty>;
  required?: string[];
}

interface ExtensionConfigFormProps {
  schema: JSONSchema;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  /** Asset ID for decrypting existing password fields on reveal */
  editAssetId?: number;
  /** Whether this is editing an existing asset (password fields may have encrypted values) */
  hasExistingPasswords?: boolean;
}

/**
 * Get the list of password-format field keys from a JSON schema.
 */
export function getPasswordFields(schema: JSONSchema | undefined): string[] {
  if (!schema?.properties) return [];
  return Object.entries(schema.properties)
    .filter(([, prop]) => prop.format === "password")
    .map(([key]) => key);
}

function PasswordFieldInput({
  fieldKey,
  placeholder,
  value,
  onChange,
  editAssetId,
  hasExisting,
}: {
  fieldKey: string;
  placeholder: string;
  value: string;
  onChange: (val: string) => void;
  editAssetId?: number;
  hasExisting: boolean;
}) {
  const { t } = useTranslation();
  const [showPassword, setShowPassword] = useState(false);
  const [decrypting, setDecrypting] = useState(false);
  const [decryptedOnce, setDecryptedOnce] = useState(false);

  const handleToggle = useCallback(async () => {
    if (!showPassword && hasExisting && !value && editAssetId && !decryptedOnce) {
      setDecrypting(true);
      try {
        const plaintext = await GetExtensionAssetPassword(editAssetId, fieldKey);
        if (plaintext) {
          onChange(plaintext);
          setDecryptedOnce(true);
        }
      } catch {
        // decrypt failed — still toggle visibility
      } finally {
        setDecrypting(false);
      }
    }
    setShowPassword(!showPassword);
  }, [showPassword, hasExisting, value, editAssetId, decryptedOnce, fieldKey, onChange]);

  return (
    <div className="relative">
      <Input
        type={showPassword ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={hasExisting && !decryptedOnce ? t("asset.passwordUnchanged") : placeholder}
        className="pr-9"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
        onClick={handleToggle}
        disabled={decrypting}
      >
        {decrypting ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : showPassword ? (
          <EyeOff className="h-3.5 w-3.5" />
        ) : (
          <Eye className="h-3.5 w-3.5" />
        )}
      </Button>
    </div>
  );
}

export function ExtensionConfigForm({
  schema,
  value,
  onChange,
  editAssetId,
  hasExistingPasswords,
}: ExtensionConfigFormProps) {
  const { i18n } = useTranslation();
  if (!schema?.properties) return null;
  const properties = schema.properties;
  const required = new Set(schema.required || []);
  const lang = i18n.language;

  const handleChange = (key: string, val: unknown) => {
    onChange({ ...value, [key]: val });
  };

  return (
    <div className="grid gap-3">
      {Object.entries(properties).map(([key, prop]) => (
        <div key={key} className="grid gap-1.5">
          <Label>
            {extLocalized(prop.title || key, prop.title_zh, lang)}
            {required.has(key) && <span className="text-destructive ml-1">*</span>}
          </Label>
          {prop.type === "boolean" ? (
            <input type="checkbox" checked={!!value[key]} onChange={(e) => handleChange(key, e.target.checked)} />
          ) : prop.enum ? (
            <select
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm"
              value={(value[key] as string) ?? ""}
              onChange={(e) => handleChange(key, e.target.value)}
            >
              <option value="">--</option>
              {prop.enum.map((v: string) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          ) : prop.format === "password" ? (
            <PasswordFieldInput
              fieldKey={key}
              value={(value[key] as string) ?? ""}
              onChange={(val) => handleChange(key, val)}
              placeholder={extLocalized(prop.description || "", prop.description_zh, lang)}
              editAssetId={editAssetId}
              hasExisting={!!hasExistingPasswords}
            />
          ) : (
            <Input
              type={prop.type === "number" || prop.type === "integer" ? "number" : "text"}
              value={(value[key] as string) ?? ""}
              onChange={(e) =>
                handleChange(
                  key,
                  prop.type === "number" || prop.type === "integer" ? Number(e.target.value) : e.target.value
                )
              }
              placeholder={extLocalized(prop.description || "", prop.description_zh, lang)}
            />
          )}
          {prop.description && (
            <p className="text-xs text-muted-foreground">{extLocalized(prop.description, prop.description_zh, lang)}</p>
          )}
        </div>
      ))}
    </div>
  );
}

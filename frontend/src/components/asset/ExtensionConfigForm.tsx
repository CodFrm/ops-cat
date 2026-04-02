import { useState, useCallback } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { Loader2, PlugZap } from "lucide-react";
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
} from "@opskat/ui";
import { CallExtensionAction } from "../../../wailsjs/go/app/App";

interface JSONSchemaProperty {
  type?: string;
  format?: string;
  enum?: string[];
  title?: string;
  description?: string;
  placeholder?: string;
}

interface JSONSchema {
  type?: string;
  properties?: Record<string, JSONSchemaProperty>;
  required?: string[];
  propertyOrder?: string[];
}

interface ExtensionConfigFormProps {
  extensionName: string;
  configSchema: JSONSchema;
  value: Record<string, unknown>;
  onChange: (config: Record<string, unknown>) => void;
  hasBackend?: boolean;
}

export function ExtensionConfigForm({
  extensionName,
  configSchema,
  value,
  onChange,
  hasBackend,
}: ExtensionConfigFormProps) {
  const { t: tCommon } = useTranslation();
  const [testing, setTesting] = useState(false);

  const properties = configSchema.properties ?? {};
  const required = new Set(configSchema.required ?? []);
  const order = configSchema.propertyOrder;
  const fields = order
    ? order.filter((k) => k in properties).map((k) => [k, properties[k]] as const)
    : Object.entries(properties);

  const updateField = useCallback(
    (key: string, fieldValue: unknown) => {
      onChange({ ...value, [key]: fieldValue });
    },
    [value, onChange]
  );

  const handleTestConnection = useCallback(async () => {
    setTesting(true);
    try {
      await CallExtensionAction(extensionName, "test_connection", JSON.stringify(value));
      toast.success(tCommon("asset.testConnectionSuccess"));
    } catch (e) {
      toast.error(`${tCommon("asset.testConnectionFailed")}: ${String(e)}`);
    } finally {
      setTesting(false);
    }
  }, [extensionName, value, tCommon]);

  const renderField = useCallback(
    (key: string, prop: JSONSchemaProperty) => {
      // Config schema values are already translated by the backend
      const label = prop.title || key;
      const description = prop.description || "";
      const placeholder = prop.placeholder || "";
      const isRequired = required.has(key);

      // Enum → Select
      if (prop.enum && prop.enum.length > 0) {
        return (
          <div key={key} className="grid gap-2">
            <Label>
              {label}
              {isRequired && <span className="text-destructive ml-0.5">*</span>}
            </Label>
            <Select value={String(value[key] ?? "")} onValueChange={(v) => updateField(key, v)}>
              <SelectTrigger>
                <SelectValue placeholder={placeholder} />
              </SelectTrigger>
              <SelectContent>
                {prop.enum.map((opt) => (
                  <SelectItem key={opt} value={opt}>
                    {opt}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {description && <p className="text-xs text-muted-foreground">{description}</p>}
          </div>
        );
      }

      // Boolean → Switch
      if (prop.type === "boolean") {
        return (
          <div key={key} className="flex items-center justify-between">
            <div>
              <Label>{label}</Label>
              {description && <p className="text-xs text-muted-foreground">{description}</p>}
            </div>
            <Switch checked={!!value[key]} onCheckedChange={(v) => updateField(key, v)} />
          </div>
        );
      }

      // String (password or normal)
      return (
        <div key={key} className="grid gap-2">
          <Label>
            {label}
            {isRequired && <span className="text-destructive ml-0.5">*</span>}
          </Label>
          <Input
            type={prop.format === "password" ? "password" : "text"}
            value={String(value[key] ?? "")}
            onChange={(e) => updateField(key, e.target.value)}
            placeholder={placeholder || (prop.format === "password" ? "••••••••" : undefined)}
          />
          {description && <p className="text-xs text-muted-foreground">{description}</p>}
        </div>
      );
    },
    [value, required, updateField]
  );

  return (
    <>
      {fields.map(([key, prop]) => renderField(key, prop))}

      {/* Test Connection */}
      {hasBackend && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleTestConnection}
          disabled={testing}
          className="gap-1 w-fit"
        >
          {testing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <PlugZap className="h-3.5 w-3.5" />}
          {testing ? tCommon("asset.testing") : tCommon("asset.testConnection")}
        </Button>
      )}
    </>
  );
}

import { useState, useEffect, useCallback } from "react";
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
import { useLocale } from "../hooks/useLocale";

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
}

interface Manifest {
  name: string;
  assetTypes?: { type: string; configSchema?: JSONSchema }[];
}

export function ConfigPanel() {
  const [manifest, setManifest] = useState<Manifest | null>(null);
  const [config, setConfig] = useState<Record<string, unknown>>({});
  const [saved, setSaved] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; message?: string } | null>(null);
  const { t } = useLocale(manifest?.name);

  useEffect(() => {
    Promise.all([fetch("/api/manifest").then((r) => r.json()), fetch("/api/config").then((r) => r.json())])
      .then(([m, c]) => {
        setManifest(m);
        setConfig(c || {});
      })
      .catch(() => {});
  }, []);

  const schema = manifest?.assetTypes?.[0]?.configSchema;
  const properties = schema?.properties ?? {};
  const required = new Set(schema?.required ?? []);

  const regularFields: [string, JSONSchemaProperty][] = [];
  const credentialFields: [string, JSONSchemaProperty][] = [];

  for (const [key, prop] of Object.entries(properties)) {
    if (prop.format === "password") {
      credentialFields.push([key, prop]);
    } else {
      regularFields.push([key, prop]);
    }
  }

  const updateField = useCallback((key: string, value: unknown) => {
    setConfig((prev) => ({ ...prev, [key]: value }));
    setSaved(false);
    setTestResult(null);
  }, []);

  const saveConfig = async () => {
    await fetch("/api/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config),
    });
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const handleTestConnection = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await fetch("/api/action/test_connection", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });
      if (!res.ok) {
        setTestResult({ ok: false, message: await res.text() });
      } else {
        setTestResult({ ok: true });
      }
    } catch (e) {
      setTestResult({ ok: false, message: String(e) });
    } finally {
      setTesting(false);
    }
  };

  const resolveI18n = useCallback(
    (key: string | undefined): string => {
      if (!key) return "";
      const resolved = t(key);
      return resolved === key ? "" : resolved;
    },
    [t]
  );

  const renderField = (key: string, prop: JSONSchemaProperty) => {
    const label = resolveI18n(prop.title) || key;
    const description = resolveI18n(prop.description);
    const placeholder = resolveI18n(prop.placeholder);
    const isRequired = required.has(key);

    if (prop.enum && prop.enum.length > 0) {
      return (
        <div key={key} className="space-y-1">
          <Label>
            {label}
            {isRequired && <span className="text-red-500 ml-0.5">*</span>}
          </Label>
          <Select value={String(config[key] ?? "")} onValueChange={(v) => updateField(key, v)}>
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

    if (prop.type === "boolean") {
      return (
        <div key={key} className="flex items-center justify-between">
          <div>
            <Label>{label}</Label>
            {description && <p className="text-xs text-muted-foreground">{description}</p>}
          </div>
          <Switch checked={!!config[key]} onCheckedChange={(v) => updateField(key, v)} />
        </div>
      );
    }

    return (
      <div key={key} className="space-y-1">
        <Label>
          {label}
          {isRequired && <span className="text-red-500 ml-0.5">*</span>}
        </Label>
        <Input
          type={prop.format === "password" ? "password" : "text"}
          value={String(config[key] ?? "")}
          onChange={(e) => updateField(key, e.target.value)}
          placeholder={placeholder || (prop.format === "password" ? "••••••••" : undefined)}
        />
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
      </div>
    );
  };

  if (!manifest) {
    return <div className="text-muted-foreground">Loading manifest...</div>;
  }

  if (!schema) {
    return (
      <div className="space-y-4 max-w-lg">
        <h2 className="text-lg font-semibold">Configuration</h2>
        <p className="text-muted-foreground">No configSchema defined in manifest.</p>
      </div>
    );
  }

  return (
    <div className="space-y-4 max-w-lg">
      <h2 className="text-lg font-semibold">Configuration</h2>

      {regularFields.map(([key, prop]) => renderField(key, prop))}

      {credentialFields.length > 0 && (
        <>
          <div className="pt-2">
            <Label className="text-sm font-medium">Credentials</Label>
          </div>
          {credentialFields.map(([key, prop]) => renderField(key, prop))}
        </>
      )}

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={saveConfig}>{saved ? "Saved!" : "Save Config"}</Button>
        <Button variant="outline" onClick={handleTestConnection} disabled={testing}>
          {testing ? "Testing..." : "Test Connection"}
        </Button>
        {testResult && (
          <span className={`text-sm ${testResult.ok ? "text-green-600" : "text-red-500"}`}>
            {testResult.ok ? "Connected!" : testResult.message || "Failed"}
          </span>
        )}
      </div>
    </div>
  );
}

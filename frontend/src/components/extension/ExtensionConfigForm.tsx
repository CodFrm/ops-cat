import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";

interface JSONSchemaProperty {
  type?: string;
  title?: string;
  description?: string;
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
}

export function ExtensionConfigForm({ schema, value, onChange }: ExtensionConfigFormProps) {
  if (!schema?.properties) return null;
  const properties = schema.properties;
  const required = new Set(schema.required || []);

  const handleChange = (key: string, val: unknown) => {
    onChange({ ...value, [key]: val });
  };

  return (
    <div className="grid gap-3">
      {Object.entries(properties).map(([key, prop]) => (
        <div key={key} className="grid gap-1.5">
          <Label>
            {prop.title || key}
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
              placeholder={prop.description}
            />
          )}
          {prop.description && <p className="text-xs text-muted-foreground">{prop.description}</p>}
        </div>
      ))}
    </div>
  );
}

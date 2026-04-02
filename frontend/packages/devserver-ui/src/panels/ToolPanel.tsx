import { useState, useEffect } from "react";
import { useLocale } from "../hooks/useLocale";

interface ToolDefUI {
  name: string;
  i18n: { description: string };
  parameters?: Record<string, unknown>;
}

interface Manifest {
  name: string;
  tools: ToolDefUI[];
}

function generateDefaults(schema: Record<string, unknown> | undefined): unknown {
  if (!schema || schema.type !== "object") return {};
  const props = schema.properties as Record<string, Record<string, unknown>> | undefined;
  if (!props) return {};
  const result: Record<string, unknown> = {};
  for (const [key, prop] of Object.entries(props)) {
    if (prop.default !== undefined) {
      result[key] = prop.default;
    } else {
      switch (prop.type) {
        case "string":
          result[key] = "";
          break;
        case "number":
        case "integer":
          result[key] = 0;
          break;
        case "boolean":
          result[key] = false;
          break;
        case "array":
          result[key] = [];
          break;
        case "object":
          result[key] = generateDefaults(prop as Record<string, unknown>);
          break;
        default:
          result[key] = null;
      }
    }
  }
  return result;
}

export function ToolPanel() {
  const [manifest, setManifest] = useState<Manifest | null>(null);
  const { t } = useLocale(manifest?.name);
  const [selectedTool, setSelectedTool] = useState("");
  const [args, setArgs] = useState("{}");
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    fetch("/api/manifest")
      .then((r) => r.json())
      .then(setManifest);
  }, []);

  const handleToolChange = (toolName: string) => {
    setSelectedTool(toolName);
    const tool = manifest?.tools?.find((item) => item.name === toolName);
    if (tool?.parameters) {
      setArgs(JSON.stringify(generateDefaults(tool.parameters), null, 2));
    } else {
      setArgs("{}");
    }
  };

  const execute = async () => {
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const resp = await fetch(`/api/tool/${selectedTool}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: args,
      });
      const data = await resp.text();
      if (!resp.ok) {
        setError(data);
      } else {
        setResult(JSON.stringify(JSON.parse(data), null, 2));
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-4 max-w-2xl">
      <h2 className="text-lg font-semibold">Tool Debugger</h2>

      <div>
        <label className="block text-sm font-medium mb-1">Tool</label>
        <select
          value={selectedTool}
          onChange={(e) => handleToolChange(e.target.value)}
          className="w-full border rounded px-3 py-2 bg-background"
        >
          <option value="">Select a tool...</option>
          {manifest?.tools?.map((tool) => (
            <option key={tool.name} value={tool.name}>
              {tool.name} — {t(tool.i18n?.description)}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">Arguments (JSON)</label>
        <textarea
          value={args}
          onChange={(e) => setArgs(e.target.value)}
          className="w-full border rounded px-3 py-2 font-mono text-sm bg-background h-32"
        />
      </div>

      <button
        onClick={execute}
        disabled={!selectedTool || loading}
        className="px-4 py-2 bg-primary text-primary-foreground rounded disabled:opacity-50"
      >
        {loading ? "Executing..." : "Execute"}
      </button>

      {result && (
        <div>
          <label className="block text-sm font-medium mb-1">Result</label>
          <pre className="border rounded p-3 bg-muted text-sm font-mono overflow-auto max-h-64">{result}</pre>
        </div>
      )}

      {error && (
        <div className="border border-destructive rounded p-3 bg-destructive/10 text-destructive text-sm">{error}</div>
      )}
    </div>
  );
}

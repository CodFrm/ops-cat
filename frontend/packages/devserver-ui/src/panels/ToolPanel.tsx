import { useState, useEffect } from "react";

interface Manifest {
  name: string;
  tools: { name: string; i18n: { description: string } }[];
}

export function ToolPanel() {
  const [manifest, setManifest] = useState<Manifest | null>(null);
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
          onChange={(e) => setSelectedTool(e.target.value)}
          className="w-full border rounded px-3 py-2 bg-background"
        >
          <option value="">Select a tool...</option>
          {manifest?.tools?.map((t) => (
            <option key={t.name} value={t.name}>
              {t.name} — {t.i18n?.description}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-sm font-medium mb-1">
          Arguments (JSON)
        </label>
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
          <pre className="border rounded p-3 bg-muted text-sm font-mono overflow-auto max-h-64">
            {result}
          </pre>
        </div>
      )}

      {error && (
        <div className="border border-destructive rounded p-3 bg-destructive/10 text-destructive text-sm">
          {error}
        </div>
      )}
    </div>
  );
}

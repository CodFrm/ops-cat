import { useState, useEffect } from "react";

export function ConfigPanel() {
  const [config, setConfig] = useState("{}");
  const [credential, setCredential] = useState('""');
  const [configSaved, setConfigSaved] = useState(false);
  const [credSaved, setCredSaved] = useState(false);

  useEffect(() => {
    fetch("/api/config")
      .then((r) => r.text())
      .then((t) => setConfig(JSON.stringify(JSON.parse(t), null, 2)))
      .catch(() => {});
  }, []);

  const saveConfig = async () => {
    await fetch("/api/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: config,
    });
    setConfigSaved(true);
    setTimeout(() => setConfigSaved(false), 2000);
  };

  const saveCredential = async () => {
    await fetch("/api/credential", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: credential,
    });
    setCredSaved(true);
    setTimeout(() => setCredSaved(false), 2000);
  };

  return (
    <div className="space-y-6 max-w-2xl">
      <h2 className="text-lg font-semibold">Mock Configuration</h2>

      <div className="space-y-2">
        <label className="block text-sm font-medium">
          Asset Config (JSON)
        </label>
        <textarea
          value={config}
          onChange={(e) => setConfig(e.target.value)}
          className="w-full border rounded px-3 py-2 font-mono text-sm bg-background h-48"
        />
        <button
          onClick={saveConfig}
          className="px-4 py-2 bg-primary text-primary-foreground rounded"
        >
          {configSaved ? "Saved!" : "Save Config"}
        </button>
      </div>

      <div className="space-y-2">
        <label className="block text-sm font-medium">
          Credential (JSON string)
        </label>
        <textarea
          value={credential}
          onChange={(e) => setCredential(e.target.value)}
          className="w-full border rounded px-3 py-2 font-mono text-sm bg-background h-24"
          placeholder={'"access_key:secret_key"'}
        />
        <button
          onClick={saveCredential}
          className="px-4 py-2 bg-primary text-primary-foreground rounded"
        >
          {credSaved ? "Saved!" : "Save Credential"}
        </button>
      </div>
    </div>
  );
}

import { useState, useEffect, useRef } from "react";

interface Event {
  type: string;
  data: unknown;
  timestamp: string;
}

export function ActionPanel() {
  const [selectedAction, setSelectedAction] = useState("");
  const [args, setArgs] = useState("{}");
  const [events, setEvents] = useState<Event[]>([]);
  const [result, setResult] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const ws = new WebSocket(`ws://${window.location.host}/ws/events`);
    ws.onmessage = (e) => {
      const msg = JSON.parse(e.data);
      if (msg.type === "event") {
        setEvents((prev) => [
          ...prev,
          {
            type: msg.eventType,
            data: msg.data,
            timestamp: new Date().toISOString(),
          },
        ]);
      }
    };
    wsRef.current = ws;
    return () => ws.close();
  }, []);

  const execute = async () => {
    setLoading(true);
    setEvents([]);
    setResult(null);
    try {
      const resp = await fetch(`/api/action/${selectedAction}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: args,
      });
      const data = await resp.text();
      setResult(JSON.stringify(JSON.parse(data), null, 2));
    } catch (e: unknown) {
      setResult(`Error: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-4 max-w-2xl">
      <h2 className="text-lg font-semibold">Action Debugger</h2>

      <div>
        <label className="block text-sm font-medium mb-1">Action Name</label>
        <input
          value={selectedAction}
          onChange={(e) => setSelectedAction(e.target.value)}
          placeholder="e.g. browse, upload"
          className="w-full border rounded px-3 py-2 bg-background"
        />
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
        disabled={!selectedAction || loading}
        className="px-4 py-2 bg-primary text-primary-foreground rounded disabled:opacity-50"
      >
        {loading ? "Executing..." : "Execute"}
      </button>

      {events.length > 0 && (
        <div>
          <label className="block text-sm font-medium mb-1">Events ({events.length})</label>
          <div className="border rounded max-h-48 overflow-auto">
            {events.map((e, i) => (
              <div key={i} className="px-3 py-1 border-b text-sm font-mono">
                <span className="text-muted-foreground">{e.timestamp.split("T")[1]?.slice(0, 8)}</span>{" "}
                <span className="font-semibold">{e.type}</span> {JSON.stringify(e.data)}
              </div>
            ))}
          </div>
        </div>
      )}

      {result && (
        <div>
          <label className="block text-sm font-medium mb-1">Result</label>
          <pre className="border rounded p-3 bg-muted text-sm font-mono overflow-auto max-h-64">{result}</pre>
        </div>
      )}
    </div>
  );
}

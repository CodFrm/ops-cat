import { useState, useEffect, useRef } from "react";

interface LogEntry {
  level: string;
  message: string;
  timestamp: string;
}

const levelColors: Record<string, string> = {
  debug: "text-muted-foreground",
  info: "text-blue-500",
  warn: "text-yellow-500",
  error: "text-red-500",
};

export function LogPanel() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const ws = new WebSocket(`ws://${window.location.host}/ws/events`);
    ws.onmessage = (e) => {
      const msg = JSON.parse(e.data);
      if (msg.type === "log") {
        setLogs((prev) => [
          ...prev,
          {
            level: msg.level,
            message: msg.message,
            timestamp: new Date().toISOString(),
          },
        ]);
      }
    };
    return () => ws.close();
  }, []);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Logs</h2>
        <button onClick={() => setLogs([])} className="text-sm text-muted-foreground hover:text-foreground">
          Clear
        </button>
      </div>

      <div className="border rounded max-h-[70vh] overflow-auto bg-muted/50 p-2 font-mono text-sm">
        {logs.length === 0 && (
          <div className="text-muted-foreground p-4 text-center">
            Waiting for logs... Execute a tool or action to see output.
          </div>
        )}
        {logs.map((log, i) => (
          <div key={i} className="py-0.5">
            <span className="text-muted-foreground">{log.timestamp.split("T")[1]?.slice(0, 12)}</span>{" "}
            <span className={`font-semibold ${levelColors[log.level] ?? ""}`}>[{log.level.toUpperCase()}]</span>{" "}
            {log.message}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

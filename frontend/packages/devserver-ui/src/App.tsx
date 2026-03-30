import { useState } from "react";
import { ToolPanel } from "./panels/ToolPanel";
import { ActionPanel } from "./panels/ActionPanel";
import { ConfigPanel } from "./panels/ConfigPanel";
import { LogPanel } from "./panels/LogPanel";

type Tab = "tools" | "actions" | "config" | "logs";

export default function App() {
  const [activeTab, setActiveTab] = useState<Tab>("tools");

  const tabs: { id: Tab; label: string }[] = [
    { id: "tools", label: "Tools" },
    { id: "actions", label: "Actions" },
    { id: "config", label: "Config" },
    { id: "logs", label: "Logs" },
  ];

  return (
    <div className="h-screen flex flex-col bg-background text-foreground">
      <header className="border-b px-4 py-2 flex items-center gap-4">
        <h1 className="text-lg font-semibold">OpsKat DevServer</h1>
        <nav className="flex gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-3 py-1 rounded text-sm ${
                activeTab === tab.id
                  ? "bg-primary text-primary-foreground"
                  : "hover:bg-muted"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </nav>
      </header>
      <main className="flex-1 overflow-auto p-4">
        {activeTab === "tools" && <ToolPanel />}
        {activeTab === "actions" && <ActionPanel />}
        {activeTab === "config" && <ConfigPanel />}
        {activeTab === "logs" && <LogPanel />}
      </main>
    </div>
  );
}

import { useState } from "react";
import { ToolPanel } from "./panels/ToolPanel";
import { ConfigPanel } from "./panels/ConfigPanel";
import { LogPanel } from "./panels/LogPanel";
import { ExtensionPanel } from "./panels/ExtensionPanel";
import { LanguageContext, useLanguageState } from "./hooks/useLanguage";

type Tab = "tools" | "config" | "logs" | "extension";

const languages = [
  { value: "zh-CN", label: "中文" },
  { value: "en", label: "English" },
];

export default function App() {
  const [activeTab, setActiveTab] = useState<Tab>("tools");
  const languageState = useLanguageState();

  const tabs: { id: Tab; label: string }[] = [
    { id: "tools", label: "Tools" },
    { id: "config", label: "Config" },
    { id: "logs", label: "Logs" },
    { id: "extension", label: "Extension" },
  ];

  return (
    <LanguageContext.Provider value={languageState}>
      <div className="h-screen flex flex-col bg-background text-foreground">
        <header className="border-b px-4 py-2 flex items-center gap-4">
          <h1 className="text-lg font-semibold">OpsKat DevServer</h1>
          <nav className="flex gap-1">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`px-3 py-1 rounded text-sm ${
                  activeTab === tab.id ? "bg-primary text-primary-foreground" : "hover:bg-muted"
                }`}
              >
                {tab.label}
              </button>
            ))}
          </nav>
          <div className="ml-auto">
            <select
              value={languageState.language}
              onChange={(e) => languageState.setLanguage(e.target.value)}
              className="border rounded px-2 py-1 text-sm bg-background"
            >
              {languages.map((lng) => (
                <option key={lng.value} value={lng.value}>
                  {lng.label}
                </option>
              ))}
            </select>
          </div>
        </header>
        <main className="flex-1 overflow-auto p-4">
          {activeTab === "tools" && <ToolPanel />}
          {activeTab === "config" && <ConfigPanel />}
          {activeTab === "logs" && <LogPanel />}
          {activeTab === "extension" && <ExtensionPanel />}
        </main>
      </div>
    </LanguageContext.Provider>
  );
}

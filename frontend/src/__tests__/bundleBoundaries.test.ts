import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

function readSource(pathFromSrc: string): string {
  return readFileSync(join(process.cwd(), "src", pathFromSrc), "utf8");
}

function expectNoStaticImport(source: string, modulePath: string): void {
  const escaped = modulePath.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  expect(source).not.toMatch(new RegExp(`^import\\s+(?:type\\s+)?[\\s\\S]*?from\\s+["']${escaped}["'];?`, "m"));
  expect(source).not.toMatch(new RegExp(`^import\\s+["']${escaped}["'];?`, "m"));
}

describe("bundle boundaries", () => {
  it("keeps Monaco setup out of the startup entry", () => {
    const source = readSource("main.tsx");

    expectNoStaticImport(source, "./lib/monaco-setup");
  });

  it("keeps terminal renderer code out of the terminal store startup path", () => {
    const source = readSource("stores/terminalStore.ts");

    expectNoStaticImport(source, "@/components/terminal/terminalRegistry");
  });

  it("loads heavy tab surfaces through dynamic chunks", () => {
    const source = readSource("components/layout/MainPanel.tsx");

    for (const modulePath of [
      "@/components/asset/AssetDetail",
      "@/components/asset/GroupDetail",
      "@/components/terminal/SplitPane",
      "@/components/terminal/SessionToolbar",
      "@/components/terminal/TerminalToolbar",
      "@/components/terminal/FileManagerPanel",
      "@/components/settings/SettingsPage",
      "@/components/settings/CredentialManager",
      "@/components/audit/AuditLogPage",
      "@/components/forward/PortForwardPage",
      "@/components/snippet/SnippetsPage",
      "@/components/ai/AIChatContent",
      "@/components/query/DatabasePanel",
      "@/components/query/RedisPanel",
      "@/components/query/MongoDBPanel",
    ]) {
      expectNoStaticImport(source, modulePath);
    }
  });

  it("keeps AI chat content out of the side panel shell", () => {
    const source = readSource("components/ai/SideAssistantPanel.tsx");

    expectNoStaticImport(source, "./AIChatContent");
  });
});

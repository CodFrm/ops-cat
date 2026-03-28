import type { ExtensionAPI, ExtensionInfo } from "@opskat/ext-shared";

/**
 * ExtensionAPI implementation using Wails IPC bindings (production).
 */
export class WailsExtensionAPI implements ExtensionAPI {
  async getExtensions(): Promise<ExtensionInfo[]> {
    const { GetExtensions } = await import("../../wailsjs/go/app/App");
    const exts = await GetExtensions();
    return (exts as unknown as ExtensionInfo[]) || [];
  }

  async callTool(
    name: string,
    args: Record<string, unknown>
  ): Promise<unknown> {
    const { CallExtensionTool } = await import("../../wailsjs/go/app/App");
    const result = await CallExtensionTool(name, JSON.stringify(args));
    return JSON.parse(result);
  }

  async testConnection(assetType: string, config: string): Promise<void> {
    const { TestExtensionConnection } = await import(
      "../../wailsjs/go/app/App"
    );
    await TestExtensionConnection(assetType, config);
  }

  async installExtension(sourcePath: string): Promise<void> {
    const { InstallExtension } = await import("../../wailsjs/go/app/App");
    await InstallExtension(sourcePath);
  }

  async removeExtension(name: string): Promise<void> {
    const { RemoveExtension } = await import("../../wailsjs/go/app/App");
    await RemoveExtension(name);
  }
}

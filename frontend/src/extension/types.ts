// frontend/src/extension/types.ts

export interface ExtManifest {
  name: string;
  version: string;
  icon: string;
  minAppVersion?: string;
  i18n: { displayName: string; description: string };
  backend?: { runtime: string; binary: string };
  assetTypes?: ExtAssetType[];
  tools?: ExtToolDef[];
  policies?: ExtPolicies;
  frontend?: ExtFrontend;
}

export interface ExtAssetType {
  type: string;
  i18n: { name: string };
  configSchema?: Record<string, unknown>;
}

export interface ExtToolDef {
  name: string;
  i18n: { description: string };
  parameters: Record<string, unknown>;
}

export interface ExtPolicies {
  type: string;
  actions: string[];
  groups: { id: string; i18n: { name: string; description: string }; policy: Record<string, unknown> }[];
  default: string[];
}

export interface ExtFrontend {
  entry: string;
  styles: string;
  pages: ExtPage[];
}

export interface ExtPage {
  id: string;
  slot?: string;
  i18n: { name: string };
  component: string;
}

export interface LoadedExtension {
  name: string;
  manifest: ExtManifest;
  components: Record<string, React.ComponentType<{ assetId?: number }>>;
}

export interface ExtEvent {
  eventType: string;
  data: unknown;
}

export interface ExtAPI {
  callTool(extName: string, tool: string, args: unknown): Promise<unknown>;
  executeAction(
    extName: string,
    action: string,
    args: unknown,
    onEvent?: (e: ExtEvent) => void,
  ): Promise<unknown>;
}

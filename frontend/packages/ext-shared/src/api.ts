/**
 * ExtensionAPI abstracts backend communication for extension operations.
 * Wails implementation (production) and HTTP implementation (dev server) both
 * implement this interface.
 */
export interface ExtensionAPI {
  /** Get list of loaded extensions */
  getExtensions(): Promise<ExtensionInfo[]>;
  /** Call an extension tool by qualified name (e.g. "oss.list_buckets") */
  callTool(name: string, args: Record<string, unknown>): Promise<unknown>;
  /** Test connection for an asset type with given config */
  testConnection(assetType: string, config: string): Promise<void>;
  /** Install extension from a local directory path */
  installExtension(sourcePath: string): Promise<void>;
  /** Remove an installed extension by name */
  removeExtension(name: string): Promise<void>;
}

export interface ExtensionAssetType {
  type: string;
  name: string;
  name_zh?: string;
  namePlaceholder?: string;
  testConnection?: boolean;
  configSchema: Record<string, unknown>;
}

export interface ExtensionPage {
  id: string;
  name: string;
  name_zh?: string;
  component: string;
}

export interface ExtensionInfo {
  name: string;
  displayName: string;
  displayName_zh?: string;
  version: string;
  icon: string;
  description: string;
  description_zh?: string;
  assetTypes: ExtensionAssetType[];
  pages: ExtensionPage[];
  policyType?: string;
  policyActions?: string[];
}

/**
 * Get localized text from extension manifest fields.
 * English is the default (no suffix), Chinese uses _zh suffix.
 */
export function extLocalized(
  text: string,
  textZh: string | undefined,
  lang: string
): string {
  if (
    (lang === "zh" || lang === "zh-CN" || lang.startsWith("zh")) &&
    textZh
  )
    return textZh;
  return text;
}

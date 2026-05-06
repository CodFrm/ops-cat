// frontend/src/lib/assetTypes/options.ts
import type { ComponentType } from "react";
import { Monitor, Database, Cylinder, Leaf, Container, Server } from "lucide-react";
import { getIconComponent } from "@/components/asset/IconPicker";
import { KafkaIcon } from "@/components/asset/brand-icons";
import type { ExtManifest } from "@/extension/types";
import type { asset_entity } from "../../../wailsjs/go/models";

export interface AssetTypeOption {
  /** Stable identifier — used as the persisted "selected" value. */
  value: string;
  /** All `asset.Type` values that should match when this option is selected. */
  aliases: string[];
  /** Either an i18n key (built-in) or a literal display string (extension). */
  label: string;
  /** Marks `label` as i18n key vs literal. */
  labelIsI18nKey: boolean;
  /** Icon component for direct render. */
  icon: ComponentType<{ className?: string; style?: React.CSSProperties }>;
  group: "builtin" | "extension";
}

interface ExtensionEntryLike {
  manifest: ExtManifest;
}

const BUILTIN_OPTIONS: AssetTypeOption[] = [
  {
    value: "ssh",
    aliases: ["ssh"],
    label: "nav.ssh",
    labelIsI18nKey: true,
    icon: Monitor,
    group: "builtin",
  },
  {
    value: "database",
    aliases: ["database", "mysql", "postgresql"],
    label: "nav.database",
    labelIsI18nKey: true,
    icon: Database,
    group: "builtin",
  },
  {
    value: "redis",
    aliases: ["redis"],
    label: "nav.redis",
    labelIsI18nKey: true,
    icon: Cylinder,
    group: "builtin",
  },
  {
    value: "mongodb",
    aliases: ["mongodb", "mongo"],
    label: "nav.mongodb",
    labelIsI18nKey: true,
    icon: Leaf,
    group: "builtin",
  },
  {
    value: "kafka",
    aliases: ["kafka"],
    label: "nav.kafka",
    labelIsI18nKey: true,
    icon: KafkaIcon,
    group: "builtin",
  },
  {
    value: "k8s",
    aliases: ["k8s", "kubernetes"],
    label: "nav.k8s",
    labelIsI18nKey: true,
    icon: Container,
    group: "builtin",
  },
];

export function getAssetTypeOptions(extensions: Record<string, ExtensionEntryLike>): AssetTypeOption[] {
  const out: AssetTypeOption[] = [...BUILTIN_OPTIONS];
  for (const entry of Object.values(extensions)) {
    const m = entry.manifest;
    if (!m.assetTypes?.length) continue;
    for (const at of m.assetTypes) {
      out.push({
        value: at.type,
        aliases: [at.type],
        label: at.i18n?.name ?? at.type,
        labelIsI18nKey: false,
        icon: m.icon ? getIconComponent(m.icon) : Server,
        group: "extension",
      });
    }
  }
  return out;
}

export function matchSelectedTypes(
  assets: asset_entity.Asset[],
  selectedTypes: string[],
  options: AssetTypeOption[]
): asset_entity.Asset[] {
  if (selectedTypes.length === 0) return assets;
  const aliasSet = new Set<string>();
  for (const value of selectedTypes) {
    const opt = options.find((o) => o.value === value);
    if (opt) opt.aliases.forEach((a) => aliasSet.add(a.toLowerCase()));
    else aliasSet.add(value.toLowerCase());
  }
  return assets.filter((a) => aliasSet.has((a.Type || "").trim().toLowerCase()));
}

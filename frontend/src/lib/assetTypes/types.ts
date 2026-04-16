import type { ComponentType } from "react";
import type { asset_entity } from "../../../wailsjs/go/models";

export interface DetailInfoCardProps {
  asset: asset_entity.Asset;
  sshTunnelName: (id?: number) => string | null;
}

export interface PolicyFieldDef {
  key: string;
  labelKey: string;
  placeholderKey: string;
  variant: "allow" | "deny" | "warn";
}

export interface PolicyDefinition {
  policyType: string;
  titleKey: string;
  hintKey: string;
  testPlaceholderKey: string;
  fields: PolicyFieldDef[];
}

export interface AssetTypeDefinition {
  type: string;
  icon: ComponentType<{ className?: string; style?: React.CSSProperties }>;
  canConnect: boolean;
  canConnectInNewTab: boolean;
  connectAction: "terminal" | "query";
  DetailInfoCard: ComponentType<DetailInfoCardProps>;
  policy?: PolicyDefinition;
}

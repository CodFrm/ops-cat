import { Database } from "lucide-react";
import { registerAssetType } from "./_register";
import { DatabaseDetailInfoCard } from "@/components/asset/detail/DatabaseDetailInfoCard";

registerAssetType({
  type: "database",
  icon: Database,
  canConnect: true,
  canConnectInNewTab: false,
  connectAction: "query",
  DetailInfoCard: DatabaseDetailInfoCard,
  policy: {
    policyType: "database",
    titleKey: "asset.queryPolicy",
    hintKey: "asset.queryPolicyHint",
    testPlaceholderKey: "asset.policyTestSqlPlaceholder",
    fields: [
      {
        key: "allow_types",
        labelKey: "asset.queryPolicyAllowTypes",
        placeholderKey: "asset.queryPolicyPlaceholder",
        variant: "allow",
      },
      {
        key: "deny_types",
        labelKey: "asset.queryPolicyDenyTypes",
        placeholderKey: "asset.queryPolicyPlaceholder",
        variant: "deny",
      },
      {
        key: "deny_flags",
        labelKey: "asset.queryPolicyDenyFlags",
        placeholderKey: "asset.queryPolicyFlagPlaceholder",
        variant: "warn",
      },
    ],
  },
});

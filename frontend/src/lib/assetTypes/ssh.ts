import { Server } from "lucide-react";
import { registerAssetType } from "./_register";
import { SSHDetailInfoCard } from "@/components/asset/detail/SSHDetailInfoCard";

registerAssetType({
  type: "ssh",
  icon: Server,
  canConnect: true,
  canConnectInNewTab: true,
  connectAction: "terminal",
  DetailInfoCard: SSHDetailInfoCard,
  policy: {
    policyType: "ssh",
    titleKey: "asset.cmdPolicy",
    hintKey: "asset.cmdPolicyHint",
    testPlaceholderKey: "asset.policyTestPlaceholder",
    fields: [
      {
        key: "allow_list",
        labelKey: "asset.cmdPolicyAllowList",
        placeholderKey: "asset.cmdPolicyPlaceholder",
        variant: "allow",
      },
      {
        key: "deny_list",
        labelKey: "asset.cmdPolicyDenyList",
        placeholderKey: "asset.cmdPolicyPlaceholder",
        variant: "deny",
      },
    ],
  },
});

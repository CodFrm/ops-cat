import { Container } from "lucide-react";
import { registerAssetType } from "./_register";
import { K8sDetailInfoCard } from "@/components/asset/detail/K8sDetailInfoCard";

registerAssetType({
  type: "k8s",
  icon: Container,
  canConnect: true,
  canConnectInNewTab: false,
  connectAction: "terminal",
  DetailInfoCard: K8sDetailInfoCard,
  policy: {
    policyType: "k8s",
    titleKey: "asset.k8sPolicy",
    hintKey: "asset.k8sPolicyHint",
    testPlaceholderKey: "asset.k8sPolicyTestPlaceholder",
    fields: [
      {
        key: "allow_list",
        labelKey: "asset.k8sPolicyAllowList",
        placeholderKey: "asset.k8sPolicyPlaceholder",
        variant: "allow",
      },
      {
        key: "deny_list",
        labelKey: "asset.k8sPolicyDenyList",
        placeholderKey: "asset.k8sPolicyPlaceholder",
        variant: "deny",
      },
    ],
  },
});

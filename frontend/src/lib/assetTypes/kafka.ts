import { KafkaIcon } from "@/components/asset/brand-icons";
import { registerAssetType } from "./_register";
import { KafkaDetailInfoCard } from "@/components/asset/detail/KafkaDetailInfoCard";

registerAssetType({
  type: "kafka",
  icon: KafkaIcon,
  canConnect: false,
  canConnectInNewTab: false,
  connectAction: "query",
  DetailInfoCard: KafkaDetailInfoCard,
  policy: {
    policyType: "kafka",
    titleKey: "asset.kafkaPolicy",
    hintKey: "asset.kafkaPolicyHint",
    testPlaceholderKey: "asset.policyTestKafkaPlaceholder",
    fields: [
      {
        key: "allow_list",
        labelKey: "asset.kafkaPolicyAllowList",
        placeholderKey: "asset.kafkaPolicyPlaceholder",
        variant: "allow",
      },
      {
        key: "deny_list",
        labelKey: "asset.kafkaPolicyDenyList",
        placeholderKey: "asset.kafkaPolicyPlaceholder",
        variant: "deny",
      },
    ],
  },
});

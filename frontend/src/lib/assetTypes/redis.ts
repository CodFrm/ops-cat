import { Database } from "lucide-react";
import { registerAssetType } from "./_register";
import { RedisDetailInfoCard } from "@/components/asset/detail/RedisDetailInfoCard";

registerAssetType({
  type: "redis",
  icon: Database,
  canConnect: true,
  canConnectInNewTab: false,
  connectAction: "query",
  DetailInfoCard: RedisDetailInfoCard,
  policy: {
    policyType: "redis",
    titleKey: "asset.redisPolicy",
    hintKey: "asset.redisPolicyHint",
    testPlaceholderKey: "asset.policyTestRedisPlaceholder",
    fields: [
      {
        key: "allow_list",
        labelKey: "asset.redisPolicyAllowList",
        placeholderKey: "asset.redisPolicyPlaceholder",
        variant: "allow",
      },
      {
        key: "deny_list",
        labelKey: "asset.redisPolicyDenyList",
        placeholderKey: "asset.redisPolicyPlaceholder",
        variant: "deny",
      },
    ],
  },
});

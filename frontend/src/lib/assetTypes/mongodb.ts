import { Database } from "lucide-react";
import { registerAssetType } from "./_register";
import { MongoDBDetailInfoCard } from "@/components/asset/detail/MongoDBDetailInfoCard";

registerAssetType({
  type: "mongodb",
  icon: Database,
  canConnect: true,
  canConnectInNewTab: false,
  connectAction: "query",
  DetailInfoCard: MongoDBDetailInfoCard,
  policy: {
    policyType: "mongo",
    titleKey: "asset.mongoPolicy",
    hintKey: "asset.mongoPolicyHint",
    testPlaceholderKey: "asset.policyTestMongoPlaceholder",
    fields: [
      {
        key: "allow_types",
        labelKey: "asset.mongoPolicyAllowTypes",
        placeholderKey: "asset.mongoPolicyPlaceholder",
        variant: "allow",
      },
      {
        key: "deny_types",
        labelKey: "asset.mongoPolicyDenyTypes",
        placeholderKey: "asset.mongoPolicyPlaceholder",
        variant: "deny",
      },
    ],
  },
});

import { Usb } from "lucide-react";
import { registerAssetType } from "./_register";
import { SerialDetailInfoCard } from "@/components/asset/detail/SerialDetailInfoCard";

registerAssetType({
  type: "serial",
  icon: Usb,
  canConnect: true,
  canConnectInNewTab: true,
  connectAction: "terminal",
  DetailInfoCard: SerialDetailInfoCard,
});

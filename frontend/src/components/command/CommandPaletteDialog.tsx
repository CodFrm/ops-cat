import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogTitle } from "@opskat/ui";
import { CommandPalette } from "./CommandPalette";
import type { asset_entity } from "../../../wailsjs/go/models";

interface CommandPaletteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
}

/**
 * 居中的 CommandPalette 弹层 —— 当顶栏被隐藏时作为 Cmd+P 的回退入口。
 */
export function CommandPaletteDialog({ open, onOpenChange, onConnectAsset }: CommandPaletteDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-2xl p-0 gap-0 top-[12%] translate-y-0 data-[state=closed]:zoom-out-100 data-[state=open]:zoom-in-100 data-[state=closed]:slide-out-to-top-2 data-[state=open]:slide-in-from-top-2"
        showCloseButton={false}
      >
        <DialogTitle className="sr-only">{t("commandPalette.placeholder")}</DialogTitle>
        <CommandPalette open={open} onClose={() => onOpenChange(false)} onConnectAsset={onConnectAsset} />
      </DialogContent>
    </Dialog>
  );
}

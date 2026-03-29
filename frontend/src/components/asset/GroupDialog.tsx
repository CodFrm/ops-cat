import { useState, useEffect } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Button,
  Input,
  Textarea,
  Label,
} from "@opskat/ui";
import { IconPicker } from "@/components/asset/IconPicker";
import { GroupSelect } from "@/components/asset/GroupSelect";
import { useAssetStore } from "@/stores/assetStore";
import { group_entity } from "../../../wailsjs/go/models";

interface GroupDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editGroup?: group_entity.Group | null;
}

export function GroupDialog({ open, onOpenChange, editGroup }: GroupDialogProps) {
  const { t } = useTranslation();
  const { createGroup, updateGroup } = useAssetStore();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [parentId, setParentId] = useState(0);
  const [icon, setIcon] = useState("folder");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      if (editGroup) {
        setName(editGroup.Name);
        setDescription(editGroup.Description || "");
        setParentId(editGroup.ParentID || 0);
        setIcon(editGroup.Icon || "folder");
      } else {
        setName("");
        setDescription("");
        setParentId(0);
        setIcon("folder");
      }
    }
  }, [open, editGroup]);

  const handleSubmit = async () => {
    if (!name.trim()) return;
    setSaving(true);
    try {
      if (editGroup) {
        await updateGroup(
          new group_entity.Group({
            ...editGroup,
            Name: name.trim(),
            Description: description,
            ParentID: parentId,
            Icon: icon,
          })
        );
      } else {
        await createGroup(
          new group_entity.Group({
            Name: name.trim(),
            Description: description,
            ParentID: parentId,
            Icon: icon,
          })
        );
      }
      onOpenChange(false);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm" onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>
            {editGroup ? t("action.edit") : t("action.add")} {t("asset.group")}
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label>{t("asset.groupName")}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
              autoFocus
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.description")}</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
              placeholder={t("asset.groupDescriptionPlaceholder")}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.icon")}</Label>
            <IconPicker value={icon} onChange={setIcon} type="group" />
          </div>
          <div className="grid gap-2">
            <Label>{t("asset.parentGroup")}</Label>
            <GroupSelect
              value={parentId}
              onValueChange={setParentId}
              excludeGroupId={editGroup?.ID}
              placeholder={t("asset.parentGroupNone")}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("action.cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={saving || !name.trim()}>
            {t("action.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

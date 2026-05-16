import { forwardRef, useImperativeHandle, useMemo, useState } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { Server } from "lucide-react";
import { useAssetStore } from "@/stores/assetStore";
import { filterAssets } from "@/lib/assetSearch";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";

export interface MentionItem {
  id: number;
  label: string;
  type: string;
  icon: string;
  groupPath: string;
}

export interface MentionListProps {
  query: string;
  command: (item: MentionItem) => void;
}

export interface MentionListRef {
  onKeyDown: (props: { event: KeyboardEvent }) => boolean;
}

const MAX_ITEMS = 8;

export const MentionList = forwardRef<MentionListRef, MentionListProps>(function MentionList({ query, command }, ref) {
  const { t } = useTranslation();
  const { assets, groups } = useAssetStore();
  const [selection, setSelection] = useState({ itemCount: 0, index: 0 });

  const items: MentionItem[] = useMemo(() => {
    if (assets.length === 0) return [];
    return filterAssets(assets, groups, { query, limit: MAX_ITEMS }).map(({ asset, groupPath }) => ({
      id: asset.ID,
      label: asset.Name,
      type: asset.Type,
      icon: asset.Icon,
      groupPath,
    }));
  }, [assets, groups, query]);

  const selectedIndex = selection.itemCount === items.length ? selection.index : 0;

  useImperativeHandle(ref, () => ({
    onKeyDown: ({ event }) => {
      if (event.key === "ArrowUp") {
        flushSync(() =>
          setSelection((current) => {
            const currentIndex = current.itemCount === items.length ? current.index : 0;
            return { itemCount: items.length, index: (currentIndex + items.length - 1) % Math.max(items.length, 1) };
          })
        );
        return true;
      }
      if (event.key === "ArrowDown") {
        flushSync(() =>
          setSelection((current) => {
            const currentIndex = current.itemCount === items.length ? current.index : 0;
            return { itemCount: items.length, index: (currentIndex + 1) % Math.max(items.length, 1) };
          })
        );
        return true;
      }
      if (event.key === "Enter") {
        const item = items[selectedIndex];
        if (item) command(item);
        return true;
      }
      return false;
    },
  }));

  if (assets.length === 0) return null;

  if (items.length === 0) {
    return (
      <div className="bg-popover text-popover-foreground rounded-md border shadow-md px-3 py-2 text-xs text-muted-foreground">
        {t("ai.mentionNotFound", "未找到资产")}
      </div>
    );
  }

  return (
    <div
      role="listbox"
      className="bg-popover text-popover-foreground rounded-md border shadow-md overflow-hidden min-w-[240px] max-w-[360px]"
    >
      {items.map((item, idx) => {
        const Icon = item.icon ? getIconComponent(item.icon) : Server;
        return (
          <button
            role="option"
            aria-selected={idx === selectedIndex}
            key={item.id}
            onClick={() => command(item)}
            className={
              "flex items-center gap-2 w-full px-2.5 py-1.5 text-xs text-left " +
              (idx === selectedIndex ? "bg-accent" : "hover:bg-accent/60")
            }
          >
            <Icon
              className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
              style={item.icon ? { color: getIconColor(item.icon) } : undefined}
            />
            <span className="flex-1 min-w-0 truncate">
              {item.groupPath && <span className="text-muted-foreground">{item.groupPath}/</span>}
              <span className="text-foreground">{item.label}</span>
            </span>
          </button>
        );
      })}
    </div>
  );
});

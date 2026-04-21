import { forwardRef, useImperativeHandle, useMemo, useState, useEffect } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { Server, Database, HardDrive, Leaf } from "lucide-react";
import { useAssetStore } from "@/stores/assetStore";
import { filterAssets } from "@/lib/assetSearch";

export interface MentionItem {
  id: number;
  label: string;
  type: string;
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

function iconForType(type: string) {
  switch (type) {
    case "mysql":
    case "postgresql":
    case "mongo":
      return <Database className="h-3.5 w-3.5 text-muted-foreground" />;
    case "redis":
      return <HardDrive className="h-3.5 w-3.5 text-muted-foreground" />;
    case "ssh":
      return <Server className="h-3.5 w-3.5 text-muted-foreground" />;
    default:
      return <Leaf className="h-3.5 w-3.5 text-muted-foreground" />;
  }
}

export const MentionList = forwardRef<MentionListRef, MentionListProps>(function MentionList({ query, command }, ref) {
  const { t } = useTranslation();
  const { assets, groups } = useAssetStore();
  const [selectedIndex, setSelectedIndex] = useState(0);

  const items: MentionItem[] = useMemo(() => {
    if (assets.length === 0) return [];
    return filterAssets(assets, groups, { query, limit: MAX_ITEMS }).map(({ asset, groupPath }) => ({
      id: asset.ID,
      label: asset.Name,
      type: asset.Type,
      groupPath,
    }));
  }, [assets, groups, query]);

  useEffect(() => setSelectedIndex(0), [items.length]);

  useImperativeHandle(ref, () => ({
    onKeyDown: ({ event }) => {
      if (event.key === "ArrowUp") {
        flushSync(() => setSelectedIndex((i) => (i + items.length - 1) % Math.max(items.length, 1)));
        return true;
      }
      if (event.key === "ArrowDown") {
        flushSync(() => setSelectedIndex((i) => (i + 1) % Math.max(items.length, 1)));
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
      {items.map((item, idx) => (
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
          {iconForType(item.type)}
          <span className="flex-1 min-w-0 truncate">
            {item.groupPath && <span className="text-muted-foreground">{item.groupPath}/</span>}
            <span className="text-foreground">{item.label}</span>
          </span>
        </button>
      ))}
    </div>
  );
});

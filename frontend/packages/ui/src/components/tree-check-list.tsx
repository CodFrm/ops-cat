import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronRight, Search } from "lucide-react";
import type { TreeNode } from "./tree-select";
import { cn } from "../lib/utils";
import { pinyinMatch } from "../lib/pinyin";

interface TreeCheckListProps {
  values: number[];
  onValuesChange: (values: number[]) => void;
  nodes: TreeNode[];
  searchable?: boolean;
  searchPlaceholder?: string;
  emptyText?: string;
  className?: string;
}

type TriState = "unchecked" | "checked" | "indeterminate";

function filterTree(nodes: TreeNode[], query: string): TreeNode[] {
  if (!query) return nodes;
  function matches(node: TreeNode): boolean {
    if (pinyinMatch(node.label, query)) return true;
    if (node.children?.some(matches)) return true;
    return false;
  }
  return nodes.filter(matches).map((node) => ({
    ...node,
    children: node.children ? filterTree(node.children, query) : undefined,
  }));
}

function collectLeafIds(node: TreeNode): number[] {
  if (node.selectable === false) {
    const ids: number[] = [];
    for (const child of node.children ?? []) ids.push(...collectLeafIds(child));
    return ids;
  }
  return [node.id];
}

function groupState(node: TreeNode, selected: Set<number>): TriState {
  const leaves = collectLeafIds(node);
  if (leaves.length === 0) return "unchecked";
  let checkedCount = 0;
  for (const id of leaves) if (selected.has(id)) checkedCount++;
  if (checkedCount === 0) return "unchecked";
  if (checkedCount === leaves.length) return "checked";
  return "indeterminate";
}

function TriCheckbox({ state, onChange, ariaLabel }: { state: TriState; onChange: () => void; ariaLabel: string }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = state === "indeterminate";
  }, [state]);
  return (
    <input
      ref={ref}
      type="checkbox"
      className="h-3.5 w-3.5 shrink-0"
      checked={state === "checked"}
      onChange={onChange}
      onClick={(e) => e.stopPropagation()}
      aria-label={ariaLabel}
    />
  );
}

function TreeRow({
  node,
  selected,
  onToggle,
  depth,
}: {
  node: TreeNode;
  selected: Set<number>;
  onToggle: (ids: number[], nextChecked: boolean) => void;
  depth: number;
}) {
  const [expanded, setExpanded] = useState(true);
  const hasChildren = !!(node.children && node.children.length > 0);
  const isGroup = node.selectable === false;

  if (isGroup) {
    const state = groupState(node, selected);
    const handleToggle = () => {
      const ids = collectLeafIds(node);
      const nextChecked = state !== "checked";
      onToggle(ids, nextChecked);
    };
    return (
      <div>
        <div
          className="flex items-center gap-1 px-2 py-1.5 rounded text-sm hover:bg-accent/50"
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
        >
          {hasChildren ? (
            <button
              type="button"
              className="p-0 h-4 w-4 shrink-0 flex items-center justify-center"
              onClick={() => setExpanded((v) => !v)}
              aria-label={expanded ? "collapse" : "expand"}
            >
              {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            </button>
          ) : (
            <span className="w-4 shrink-0" />
          )}
          <TriCheckbox state={state} onChange={handleToggle} ariaLabel={node.label} />
          {node.icon && <span className="shrink-0">{node.icon}</span>}
          <span className="truncate flex-1 text-muted-foreground">{node.label}</span>
        </div>
        {hasChildren && expanded && (
          <div>
            {node.children!.map((child) => (
              <TreeRow key={child.id} node={child} selected={selected} onToggle={onToggle} depth={depth + 1} />
            ))}
          </div>
        )}
      </div>
    );
  }

  const checked = selected.has(node.id);
  return (
    <label
      className={cn(
        "flex items-center gap-1 px-2 py-1.5 rounded text-sm cursor-pointer hover:bg-accent transition-colors",
        checked && "bg-accent/60"
      )}
      style={{ paddingLeft: `${depth * 16 + 8}px` }}
    >
      <span className="w-4 shrink-0" />
      <TriCheckbox
        state={checked ? "checked" : "unchecked"}
        onChange={() => onToggle([node.id], !checked)}
        ariaLabel={node.label}
      />
      {node.icon && <span className="shrink-0">{node.icon}</span>}
      <span className="truncate flex-1">{node.label}</span>
    </label>
  );
}

/**
 * Embedded multi-select tree with checkboxes. Group nodes (selectable: false) show
 * tri-state check based on descendant leaf selection and toggle all descendants on click.
 */
export function TreeCheckList({
  values,
  onValuesChange,
  nodes,
  searchable = false,
  searchPlaceholder,
  emptyText,
  className,
}: TreeCheckListProps) {
  const [search, setSearch] = useState("");
  const selected = useMemo(() => new Set(values), [values]);
  const filtered = searchable ? filterTree(nodes, search) : nodes;

  const handleToggle = (ids: number[], nextChecked: boolean) => {
    const next = new Set(selected);
    if (nextChecked) for (const id of ids) next.add(id);
    else for (const id of ids) next.delete(id);
    onValuesChange(Array.from(next));
  };

  return (
    <div className={cn("flex flex-col min-h-0", className)}>
      {searchable && (
        <div className="relative mb-2 shrink-0">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={searchPlaceholder}
            className="w-full h-8 pl-7 pr-2 text-xs rounded-md border bg-background outline-none focus:ring-1 focus:ring-ring"
          />
        </div>
      )}
      <div className="flex-1 overflow-y-auto min-h-0">
        {filtered.length === 0 ? (
          <p className="text-xs text-muted-foreground text-center py-8">{emptyText}</p>
        ) : (
          filtered.map((node) => (
            <TreeRow key={node.id} node={node} selected={selected} onToggle={handleToggle} depth={0} />
          ))
        )}
      </div>
    </div>
  );
}

export type { TreeCheckListProps };

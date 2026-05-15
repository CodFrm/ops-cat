import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  ArrowDown,
  ArrowDownNarrowWide,
  ArrowUp,
  ArrowUpNarrowWide,
  Ban,
  Check,
  ClipboardPaste,
  Copy,
  Plus,
  Search,
  Trash2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Input,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@opskat/ui";
import { cellValueToText } from "@/lib/cellValue";
import {
  addFilterCondition,
  addFilterGroup,
  addSortCriterion,
  cloneFilterItem,
  createFilterCondition,
  findFilterItem,
  insertFilterItemAfter,
  moveFilterItem,
  pickNextFilterColumn,
  removeFilterItem,
  setAllFilterItemsEnabled,
  toggleFilterItemEnabled,
  toggleFilterJoin,
  toggleFilterNegator,
  toggleSortDirection,
  unwrapFilterGroup,
  updateFilterGroup,
  updateFilterGroupItems,
  updateFilterItem,
  updateSortCriterion,
  type TableFilterCondition,
  type TableFilterItem,
  type TableFilterOperator,
  type TableSortItem,
} from "@/lib/tableFilter";
import {
  filterOperatorNeedsRange,
  filterOperatorNeedsValue,
  TABLE_FILTER_OPERATOR_LABEL_KEYS,
  TABLE_FILTER_OPERATOR_OPTIONS,
} from "@/lib/tableFilterOperators";

const FILTER_ACTION_BUTTON_CLASS =
  "border-primary/70 text-primary hover:bg-primary/10 disabled:border-border disabled:text-muted-foreground disabled:opacity-40";
const FILTER_MENU_ITEM_CLASS =
  "flex w-full cursor-default items-center gap-2 rounded-sm px-3 py-1.5 text-left text-sm outline-hidden hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40";
type FilterContextTarget = {
  id: string;
  kind: "condition" | "group";
  x: number;
  y: number;
};

interface TableFilterBuilderProps {
  columns: string[];
  rows: Record<string, unknown>[];
  filters: TableFilterItem[];
  sorts: TableSortItem[];
  driver?: string;
  onChange: (items: TableFilterItem[]) => void;
  onSortsChange: (items: TableSortItem[]) => void;
  onApply: () => void;
}

interface DistinctValue {
  key: string;
  value: unknown;
  label: string;
  count: number;
}

function valueKey(value: unknown): string {
  if (value == null) return "__opskat_null__";
  return cellValueToText(value);
}

function distinctValues(rows: Record<string, unknown>[], column: string): DistinctValue[] {
  const map = new Map<string, DistinctValue>();
  for (const row of rows) {
    const value = row[column];
    const key = valueKey(value);
    const hit = map.get(key);
    if (hit) {
      hit.count += 1;
    } else {
      map.set(key, {
        key,
        value: value == null ? null : value,
        label: value == null ? "NULL" : cellValueToText(value),
        count: 1,
      });
    }
  }
  return Array.from(map.values());
}

export function TableFilterBuilder({
  columns,
  rows,
  filters,
  sorts,
  driver,
  onChange,
  onSortsChange,
  onApply,
}: TableFilterBuilderProps) {
  const { t } = useTranslation();
  const [selectedFilterId, setSelectedFilterId] = useState<string | null>(null);
  const [ctxTarget, setCtxTarget] = useState<FilterContextTarget | null>(null);
  const [copiedFilterItems, setCopiedFilterItems] = useState<TableFilterItem[]>([]);
  const ctxMenuRef = useRef<HTMLDivElement>(null);

  const addCondition = useCallback(() => {
    onChange(addFilterCondition(filters, columns, driver));
  }, [columns, driver, filters, onChange]);
  const addSort = useCallback(() => {
    onSortsChange(addSortCriterion(sorts, columns, driver));
  }, [columns, driver, onSortsChange, sorts]);
  const addGroup = useCallback(() => {
    onChange(addFilterGroup(filters, columns, driver));
  }, [columns, driver, filters, onChange]);

  useEffect(() => {
    if (!ctxTarget) return;
    const close = () => setCtxTarget(null);
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    const onPointer = (event: PointerEvent) => {
      if (ctxMenuRef.current?.contains(event.target as Node)) return;
      close();
    };
    const timer = setTimeout(() => document.addEventListener("pointerdown", onPointer, true), 50);
    document.addEventListener("keydown", onKey);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("pointerdown", onPointer, true);
      document.removeEventListener("keydown", onKey);
    };
  }, [ctxTarget]);

  const openContextMenu = useCallback((target: FilterContextTarget) => {
    setSelectedFilterId(target.id);
    setCtxTarget(target);
  }, []);

  const closeContextMenu = useCallback(() => setCtxTarget(null), []);

  const handleDeleteFilterItem = useCallback(() => {
    if (!ctxTarget) return;
    onChange(removeFilterItem(filters, ctxTarget.id));
    setSelectedFilterId(null);
    setCtxTarget(null);
  }, [ctxTarget, filters, onChange]);

  const handleUnwrapGroup = useCallback(() => {
    if (!ctxTarget || ctxTarget.kind !== "group") return;
    onChange(unwrapFilterGroup(filters, ctxTarget.id));
    setSelectedFilterId(null);
    setCtxTarget(null);
  }, [ctxTarget, filters, onChange]);

  const handleMoveFilterItem = useCallback(
    (direction: "up" | "down") => {
      if (!ctxTarget) return;
      onChange(moveFilterItem(filters, ctxTarget.id, direction));
      setCtxTarget(null);
    },
    [ctxTarget, filters, onChange]
  );

  const handleMoveSelectedFilterItem = useCallback(
    (direction: "up" | "down") => {
      if (!selectedFilterId) return;
      onChange(moveFilterItem(filters, selectedFilterId, direction));
    },
    [filters, onChange, selectedFilterId]
  );

  const targetItem = useMemo(() => (ctxTarget ? findFilterItem(filters, ctxTarget.id) : null), [ctxTarget, filters]);
  const targetEnabled = targetItem?.kind === "condition" ? targetItem.enabled : targetItem?.enabled !== false;

  const handleToggleFilterItemEnabled = useCallback(() => {
    if (!ctxTarget) return;
    onChange(toggleFilterItemEnabled(filters, ctxTarget.id));
    setCtxTarget(null);
  }, [ctxTarget, filters, onChange]);

  const handleToggleNegator = useCallback(() => {
    if (!ctxTarget) return;
    onChange(toggleFilterNegator(filters, ctxTarget.id));
    setCtxTarget(null);
  }, [ctxTarget, filters, onChange]);

  const handleCopyFilterItem = useCallback(() => {
    if (!targetItem) return;
    setCopiedFilterItems([targetItem]);
    setCtxTarget(null);
  }, [targetItem]);

  const handleCopyAllFilters = useCallback(() => {
    setCopiedFilterItems(filters);
    setCtxTarget(null);
  }, [filters]);

  const handlePasteFilterItem = useCallback(() => {
    if (!ctxTarget || copiedFilterItems.length === 0) return;
    let nextItems = filters;
    let afterId = ctxTarget.id;
    for (const copiedItem of copiedFilterItems) {
      const cloned = cloneFilterItem(copiedItem);
      nextItems = insertFilterItemAfter(nextItems, afterId, cloned);
      afterId = cloned.id;
    }
    onChange(nextItems);
    setCtxTarget(null);
  }, [copiedFilterItems, ctxTarget, filters, onChange]);

  return (
    <div className="shrink-0 border-b border-border bg-background">
      <div className="px-3 pt-2 pb-1">
        <div className="mb-1.5 flex items-center gap-3">
          <span className="text-sm font-semibold text-foreground">{t("query.filterBuilderTitle")}</span>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon-xs"
              title={t("query.moveFilterUp")}
              onClick={() => handleMoveSelectedFilterItem("up")}
              disabled={!selectedFilterId}
            >
              <ArrowUp className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              title={t("query.moveFilterDown")}
              onClick={() => handleMoveSelectedFilterItem("down")}
              disabled={!selectedFilterId}
            >
              <ArrowDown className="h-4 w-4" />
            </Button>
          </div>
        </div>
        <div className="min-h-[132px] space-y-1">
          {filters.length === 0 ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Button
                variant="outline"
                size="icon-xs"
                className={FILTER_ACTION_BUTTON_CLASS}
                title={t("query.addFilter")}
                onClick={addCondition}
                disabled={columns.length === 0}
              >
                <Plus className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="outline"
                size="icon-xs"
                className={FILTER_ACTION_BUTTON_CLASS}
                title={t("query.addFilterGroup")}
                onClick={addGroup}
                disabled={columns.length === 0}
              >
                ()+
              </Button>
              <span>{t("query.filterBuilderEmpty")}</span>
            </div>
          ) : (
            <FilterItems
              columns={columns}
              rows={rows}
              items={filters}
              driver={driver}
              rootItems={filters}
              onChange={onChange}
              selectedId={selectedFilterId}
              onSelect={setSelectedFilterId}
              onContextMenu={openContextMenu}
            />
          )}
        </div>
      </div>
      <div className="flex items-center gap-3 border-t border-border px-3 py-2">
        <span className="text-sm font-semibold text-foreground">{t("query.sortBuilderTitle")}</span>
        {sorts.map((sort) => (
          <SortCriterionChip key={sort.id} columns={columns} item={sort} items={sorts} onChange={onSortsChange} />
        ))}
        <Button variant="outline" size="icon-xs" title={t("query.addSort")} onClick={addSort}>
          <Plus className="h-3.5 w-3.5" />
        </Button>
        {sorts.length === 0 && <span className="text-xs text-muted-foreground">{t("query.sortBuilderEmpty")}</span>}
      </div>
      <div className="px-3 pb-2">
        <Button size="sm" className="h-8 text-xs" onClick={onApply}>
          {t("query.applyFilterSort")}
        </Button>
      </div>
      {ctxTarget &&
        createPortal(
          <div
            ref={ctxMenuRef}
            className="z-50 min-w-56 overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-lg"
            style={{ position: "fixed", top: ctxTarget.y + 2, left: ctxTarget.x + 2 }}
            role="menu"
          >
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => {
                addCondition();
                closeContextMenu();
              }}
            >
              <Plus className="h-3.5 w-3.5" />
              {t("query.addFilter")}
            </button>
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => {
                addGroup();
                closeContextMenu();
              }}
            >
              <span className="w-3.5 text-center">()</span>
              {t("query.addFilterGroup")}
            </button>
            <div className="my-1 h-px bg-border" />
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={handleToggleFilterItemEnabled}
            >
              <Ban className="h-3.5 w-3.5" />
              {targetEnabled ? t("query.disableFilterItem") : t("query.enableFilterItem")}
            </button>
            <button type="button" role="menuitem" className={FILTER_MENU_ITEM_CLASS} onClick={handleToggleNegator}>
              {t("query.toggleFilterNegator")}
            </button>
            <div className="my-1 h-px bg-border" />
            <button type="button" role="menuitem" className={FILTER_MENU_ITEM_CLASS} onClick={handleDeleteFilterItem}>
              <Trash2 className="h-3.5 w-3.5" />
              {ctxTarget.kind === "group" ? t("query.deleteFilterGroupWithChildren") : t("query.deleteFilterItem")}
            </button>
            {ctxTarget.kind === "group" && (
              <button type="button" role="menuitem" className={FILTER_MENU_ITEM_CLASS} onClick={handleUnwrapGroup}>
                {t("query.deleteFilterGroupOnly")}
              </button>
            )}
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => {
                onChange([]);
                closeContextMenu();
              }}
            >
              {t("query.clearAllFilters")}
            </button>
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => {
                onChange([]);
                onSortsChange([]);
                closeContextMenu();
              }}
            >
              {t("query.clearFilterSort")}
            </button>
            <div className="my-1 h-px bg-border" />
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => handleMoveFilterItem("up")}
            >
              <ArrowUp className="h-3.5 w-3.5" />
              {t("query.moveFilterUp")}
            </button>
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => handleMoveFilterItem("down")}
            >
              <ArrowDown className="h-3.5 w-3.5" />
              {t("query.moveFilterDown")}
            </button>
            <div className="my-1 h-px bg-border" />
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => {
                onChange(setAllFilterItemsEnabled(filters, true));
                closeContextMenu();
              }}
            >
              {t("query.enableAllFilters")}
            </button>
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={() => {
                onChange(setAllFilterItemsEnabled(filters, false));
                closeContextMenu();
              }}
            >
              {t("query.disableAllFilters")}
            </button>
            <div className="my-1 h-px bg-border" />
            <button type="button" role="menuitem" className={FILTER_MENU_ITEM_CLASS} onClick={handleCopyFilterItem}>
              <Copy className="h-3.5 w-3.5" />
              {t("query.copyFilterItem")}
            </button>
            <button type="button" role="menuitem" className={FILTER_MENU_ITEM_CLASS} onClick={handleCopyAllFilters}>
              <Copy className="h-3.5 w-3.5" />
              {t("query.copyAllFilters")}
            </button>
            <button
              type="button"
              role="menuitem"
              className={FILTER_MENU_ITEM_CLASS}
              onClick={handlePasteFilterItem}
              disabled={copiedFilterItems.length === 0}
            >
              <ClipboardPaste className="h-3.5 w-3.5" />
              {t("query.pasteFilterItem")}
            </button>
          </div>,
          document.body
        )}
    </div>
  );
}

interface SortCriterionChipProps {
  columns: string[];
  item: TableSortItem;
  items: TableSortItem[];
  onChange: (items: TableSortItem[]) => void;
}

function SortCriterionChip({ columns, item, items, onChange }: SortCriterionChipProps) {
  const { t } = useTranslation();
  const DirectionIcon = item.dir === "asc" ? ArrowUpNarrowWide : ArrowDownNarrowWide;

  return (
    <div className="flex h-9 items-center gap-2 rounded-md border border-primary bg-primary/5 px-2 text-primary">
      <Select
        value={item.column}
        onValueChange={(value) => onChange(updateSortCriterion(items, item.id, { column: value }))}
      >
        <SelectTrigger
          size="sm"
          className="h-7 min-w-20 gap-1 border-0 bg-transparent px-1.5 py-0 text-sm font-medium text-primary shadow-none hover:bg-accent focus-visible:ring-1 [&>svg]:opacity-60"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {columns.map((column) => (
            <SelectItem key={column} value={column}>
              {column}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <button
        type="button"
        className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-primary"
        title={`${t("query.toggleSortDirection")}:${item.column}`}
        onClick={() => onChange(toggleSortDirection(items, item.id))}
      >
        <DirectionIcon className="h-4 w-4" />
      </button>
    </div>
  );
}

interface FilterItemsProps {
  columns: string[];
  rows: Record<string, unknown>[];
  items: TableFilterItem[];
  rootItems: TableFilterItem[];
  driver?: string;
  groupId?: string;
  onChange: (items: TableFilterItem[]) => void;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onContextMenu: (target: FilterContextTarget) => void;
}

function FilterItems({
  columns,
  rows,
  items,
  rootItems,
  driver,
  groupId,
  onChange,
  selectedId,
  onSelect,
  onContextMenu,
}: FilterItemsProps) {
  const { t } = useTranslation();

  const addSibling = useCallback(() => {
    if (!groupId) {
      onChange(addFilterCondition(rootItems, columns, driver));
      return;
    }
    const column = pickNextFilterColumn(rootItems, columns);
    if (!column) return;
    onChange(
      updateFilterGroupItems(rootItems, groupId, (children) => [
        ...children,
        createFilterCondition(`filter-${Date.now()}`, column),
      ])
    );
  }, [columns, driver, groupId, onChange, rootItems]);

  const addSiblingGroup = useCallback(() => {
    if (!groupId) {
      onChange(addFilterGroup(rootItems, columns, driver));
      return;
    }
    onChange(updateFilterGroupItems(rootItems, groupId, (children) => addFilterGroup(children, columns, driver)));
  }, [columns, driver, groupId, onChange, rootItems]);

  if (items.length === 0) {
    return (
      <div className="ml-4 flex items-center gap-2 text-xs text-muted-foreground">
        <Button
          variant="outline"
          size="icon-xs"
          className={FILTER_ACTION_BUTTON_CLASS}
          title={t("query.addFilter")}
          onClick={addSibling}
          disabled={columns.length === 0}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="outline"
          size="icon-xs"
          className={FILTER_ACTION_BUTTON_CLASS}
          title={t("query.addFilterGroup")}
          onClick={addSiblingGroup}
          disabled={columns.length === 0}
        >
          ()+
        </Button>
        <span>{t("query.filterBuilderEmpty")}</span>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      {items.map((item, index) =>
        item.kind === "condition" ? (
          <FilterConditionRow
            key={item.id}
            columns={columns}
            rows={rows}
            item={item}
            isLast={index === items.length - 1}
            rootItems={rootItems}
            onChange={onChange}
            onAddAfter={addSibling}
            onAddGroupAfter={addSiblingGroup}
            selected={selectedId === item.id}
            onSelect={() => onSelect(item.id)}
            onContextMenu={(event) => {
              event.preventDefault();
              onContextMenu({ id: item.id, kind: "condition", x: event.clientX, y: event.clientY });
            }}
          />
        ) : (
          <div key={item.id} className={`space-y-1 ${item.enabled === false ? "opacity-50" : ""}`}>
            <div
              data-testid={`filter-group-${item.id}-open`}
              className={`flex min-h-8 items-center gap-2 rounded-sm px-2 py-1 text-sm text-foreground ${
                selectedId === item.id ? "bg-primary/10" : "hover:bg-accent/50"
              }`}
              onClick={() => onSelect(item.id)}
              onContextMenu={(event) => {
                event.preventDefault();
                onContextMenu({ id: item.id, kind: "group", x: event.clientX, y: event.clientY });
              }}
            >
              <input
                type="checkbox"
                className="h-4 w-4 accent-primary"
                checked={item.enabled !== false}
                onChange={(event) => onChange(updateFilterGroup(rootItems, item.id, { enabled: event.target.checked }))}
                aria-label={t("query.filterEnabled")}
              />
              {item.negated && (
                <span className="rounded bg-destructive/10 px-1 text-xs font-medium text-destructive">not</span>
              )}
              <span data-testid={`filter-item-${item.id}`} className="font-mono">
                (
              </span>
            </div>
            <div data-testid={`filter-group-${item.id}-body`} className="rounded-sm py-0.5">
              <div className="ml-5 border-l border-primary/30 pl-3">
                <FilterItems
                  columns={columns}
                  rows={rows}
                  items={item.items}
                  rootItems={rootItems}
                  driver={driver}
                  groupId={item.id}
                  onChange={onChange}
                  selectedId={selectedId}
                  onSelect={onSelect}
                  onContextMenu={onContextMenu}
                />
              </div>
            </div>
            <div
              data-testid={`filter-group-${item.id}-close`}
              className={`flex min-h-8 items-center gap-2 rounded-sm px-2 py-1 text-sm text-foreground ${
                selectedId === item.id ? "bg-primary/10" : "hover:bg-accent/50"
              }`}
              onClick={() => onSelect(item.id)}
              onContextMenu={(event) => {
                event.preventDefault();
                onContextMenu({ id: item.id, kind: "group", x: event.clientX, y: event.clientY });
              }}
            >
              <span className="ml-5 font-mono">)</span>
              {index === items.length - 1 ? (
                <>
                  <Button
                    variant="outline"
                    size="icon-xs"
                    className={FILTER_ACTION_BUTTON_CLASS}
                    title={t("query.addFilter")}
                    onClick={addSibling}
                    disabled={columns.length === 0}
                  >
                    <Plus className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="outline"
                    size="icon-xs"
                    className={FILTER_ACTION_BUTTON_CLASS}
                    title={t("query.addFilterGroup")}
                    onClick={addSiblingGroup}
                    disabled={columns.length === 0}
                  >
                    ()+
                  </Button>
                </>
              ) : (
                <button
                  type="button"
                  className="text-sm text-muted-foreground hover:text-primary"
                  onClick={() => onChange(toggleFilterJoin(rootItems, item.id))}
                >
                  {item.join}
                </button>
              )}
            </div>
          </div>
        )
      )}
    </div>
  );
}

interface FilterConditionRowProps {
  columns: string[];
  rows: Record<string, unknown>[];
  item: TableFilterCondition;
  isLast: boolean;
  rootItems: TableFilterItem[];
  onChange: (items: TableFilterItem[]) => void;
  onAddAfter: () => void;
  onAddGroupAfter: () => void;
  selected: boolean;
  onSelect: () => void;
  onContextMenu: (event: React.MouseEvent<HTMLDivElement>) => void;
}

function FilterConditionRow({
  columns,
  rows,
  item,
  isLast,
  rootItems,
  onChange,
  onAddAfter,
  onAddGroupAfter,
  selected,
  onSelect,
  onContextMenu,
}: FilterConditionRowProps) {
  const { t } = useTranslation();

  const setItem = useCallback(
    (patch: Partial<TableFilterCondition>) => onChange(updateFilterItem(rootItems, item.id, patch)),
    [item.id, onChange, rootItems]
  );
  const operator = item.operator ?? "=";
  const operatorNeedsValue = filterOperatorNeedsValue(operator);
  const operatorNeedsRange = filterOperatorNeedsRange(operator);

  return (
    <div
      data-testid={`filter-item-${item.id}`}
      className={`flex items-center gap-1.5 rounded-sm border px-2 py-1 text-sm ${
        selected ? "border-primary/40 bg-primary/10" : "border-transparent hover:border-border"
      } ${item.enabled ? "" : "opacity-50"}`}
      onClick={onSelect}
      onContextMenu={onContextMenu}
    >
      <input
        type="checkbox"
        className="h-4 w-4 shrink-0 accent-primary"
        checked={item.enabled}
        onChange={(event) => setItem({ enabled: event.target.checked })}
        aria-label={t("query.filterEnabled")}
      />
      <Select value={item.column} onValueChange={(value) => setItem({ column: value, value: undefined })}>
        <SelectTrigger
          size="sm"
          className="h-7 min-w-20 gap-1 border-0 bg-transparent px-1.5 py-0 text-sm font-medium text-primary shadow-none hover:bg-accent focus-visible:ring-1 [&>svg]:opacity-60"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {columns.map((column) => (
            <SelectItem key={column} value={column}>
              {column}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={operator}
        onValueChange={(value) => {
          const nextOperator = value as TableFilterOperator;
          setItem({
            operator: nextOperator,
            value: filterOperatorNeedsValue(nextOperator) ? item.value : undefined,
          });
        }}
      >
        <SelectTrigger
          size="sm"
          className="h-7 min-w-12 gap-1 border-0 bg-transparent px-1.5 py-0 text-sm font-medium text-muted-foreground shadow-none hover:bg-accent focus-visible:ring-1 [&>svg]:opacity-60"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {TABLE_FILTER_OPERATOR_OPTIONS.map((op) => (
            <SelectItem key={op} value={op}>
              {t(TABLE_FILTER_OPERATOR_LABEL_KEYS[op])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {operatorNeedsRange ? (
        <FilterRangePicker value={item.value} onChange={(value) => setItem({ value })} />
      ) : operatorNeedsValue ? (
        <FilterValuePicker
          value={item.value}
          rows={rows}
          column={item.column}
          onChange={(value) => setItem({ value })}
        />
      ) : null}
      {isLast && (
        <>
          <Button
            variant="outline"
            size="icon-xs"
            className={FILTER_ACTION_BUTTON_CLASS}
            title={t("query.addFilter")}
            onClick={onAddAfter}
            disabled={columns.length === 0}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="outline"
            size="icon-xs"
            className={FILTER_ACTION_BUTTON_CLASS}
            title={t("query.addFilterGroup")}
            onClick={onAddGroupAfter}
            disabled={columns.length === 0}
          >
            ()+
          </Button>
        </>
      )}
      {!isLast && (
        <button
          type="button"
          className="rounded px-1 text-sm text-muted-foreground hover:text-primary"
          onClick={() => onChange(toggleFilterJoin(rootItems, item.id))}
        >
          {item.join}
        </button>
      )}
    </div>
  );
}

interface FilterValuePickerProps {
  value: unknown;
  rows: Record<string, unknown>[];
  column: string;
  onChange: (value: unknown) => void;
}

interface FilterRangePickerProps {
  value: unknown;
  onChange: (value: unknown[]) => void;
}

function rangePart(value: unknown, index: number): string {
  if (!Array.isArray(value)) return "";
  const part = value[index];
  return part == null ? "" : cellValueToText(part);
}

function FilterRangePicker({ value, onChange }: FilterRangePickerProps) {
  const { t } = useTranslation();
  const start = rangePart(value, 0);
  const end = rangePart(value, 1);

  return (
    <div className="flex items-center gap-1">
      <Input
        value={start}
        onChange={(event) => onChange([event.target.value, end])}
        placeholder={t("query.filterRangeStart")}
        className="h-7 w-24 px-2 py-0 text-xs"
      />
      <span className="text-xs text-muted-foreground">-</span>
      <Input
        value={end}
        onChange={(event) => onChange([start, event.target.value])}
        placeholder={t("query.filterRangeEnd")}
        className="h-7 w-24 px-2 py-0 text-xs"
      />
    </div>
  );
}

function FilterValuePicker({ value, rows, column, onChange }: FilterValuePickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [customValue, setCustomValue] = useState("");
  const [search, setSearch] = useState("");
  const label = value === undefined ? "?" : value == null ? "NULL" : cellValueToText(value);
  // distinctValues 是 O(rows) 全表扫描;只在 popover 打开后才计算,关闭后丢弃。
  // 否则父组件每次 re-render 都会因 rows 引用变化触发整列重算,大表上 ~几十~几百 ms。
  const suggestions = useMemo<DistinctValue[]>(() => (open ? distinctValues(rows, column) : []), [open, rows, column]);
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return suggestions;
    return suggestions.filter((item) => item.label.toLowerCase().includes(q));
  }, [search, suggestions]);

  const commitCustomValue = useCallback(() => {
    if (!customValue.trim()) return;
    onChange(customValue);
    setOpen(false);
    setCustomValue("");
  }, [customValue, onChange]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          title={t("query.chooseFilterValue")}
          className={`min-w-[28px] rounded px-1 text-left font-medium ${
            value === undefined ? "text-muted-foreground" : "text-primary"
          } hover:bg-accent`}
        >
          {label}
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-[420px] p-3" align="start" sideOffset={4}>
        <div className="space-y-2">
          <Input
            className="h-8 font-mono text-xs"
            value={customValue}
            autoFocus
            onChange={(event) => setCustomValue(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") commitCustomValue();
            }}
          />
          <div className="text-sm font-medium">{t("query.suggestedValues")}</div>
          <ScrollArea className="h-[180px] border border-border bg-background">
            <div className="divide-y divide-border/30">
              {filtered.map((item, index) => {
                const checked = valueKey(value) === item.key;
                return (
                  <button
                    key={item.key}
                    type="button"
                    className={`flex h-8 w-full items-center gap-2 px-3 text-left text-sm ${
                      index % 2 === 0 ? "bg-background" : "bg-muted/40"
                    } hover:bg-accent`}
                    onClick={() => {
                      onChange(item.value);
                      setOpen(false);
                    }}
                  >
                    <span className="flex h-4 w-4 items-center justify-center rounded border border-input bg-background">
                      {checked && <Check className="h-3 w-3 text-primary" />}
                    </span>
                    <span className="min-w-0 flex-1 truncate font-mono">{item.label}</span>
                    <span className="text-xs text-muted-foreground tabular-nums">{item.count}</span>
                  </button>
                );
              })}
            </div>
          </ScrollArea>
          <div className="relative">
            <Search className="pointer-events-none absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="h-8 pl-8 text-sm"
              placeholder={t("query.search")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}

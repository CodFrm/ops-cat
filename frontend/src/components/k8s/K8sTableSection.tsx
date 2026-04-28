import type { ReactNode } from "react";
import { K8sSectionCard } from "./K8sSectionCard";

interface Column<T> {
  key: string;
  label: ReactNode;
  className?: string;
}

interface K8sTableSectionProps<T> {
  title: string;
  columns: Column<T>[];
  data: T[];
  renderRow: (item: T, index: number) => ReactNode;
  emptyText?: string;
}

export function K8sTableSection<T>({ title, columns, data, renderRow, emptyText }: K8sTableSectionProps<T>) {
  return (
    <K8sSectionCard title={title}>
      {data.length === 0 ? (
        <p className="text-xs text-muted-foreground">{emptyText || "No data"}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b">
                {columns.map((col) => (
                  <th
                    key={col.key}
                    className={`text-left py-2 pr-4 text-xs text-muted-foreground font-medium ${col.className || ""}`}
                  >
                    {col.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((item, i) => renderRow(item, i))}
            </tbody>
          </table>
        </div>
      )}
    </K8sSectionCard>
  );
}

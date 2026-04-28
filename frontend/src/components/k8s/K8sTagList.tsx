import { K8sSectionCard } from "./K8sSectionCard";

interface K8sTagListProps {
  tags: Record<string, string>;
  title?: string;
}

export function K8sTagList({ tags, title }: K8sTagListProps) {
  const entries = Object.entries(tags);
  if (entries.length === 0) return null;

  return (
    <K8sSectionCard title={title || "Labels"}>
      <div className="flex flex-wrap gap-2">
        {entries.map(([k, v]) => (
          <span
            key={k}
            className="inline-flex items-center rounded-md border bg-muted/50 px-2 py-0.5 text-xs font-mono"
          >
            {k}: {v}
          </span>
        ))}
      </div>
    </K8sSectionCard>
  );
}

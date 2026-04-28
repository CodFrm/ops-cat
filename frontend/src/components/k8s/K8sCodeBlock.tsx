import { K8sSectionCard } from "./K8sSectionCard";

interface K8sCodeBlockProps {
  code: string;
  title?: string;
  maxHeight?: string;
}

export function K8sCodeBlock({ code, title, maxHeight = "max-h-96" }: K8sCodeBlockProps) {
  return (
    <K8sSectionCard title={title || "YAML"}>
      <pre
        className={`bg-muted/50 rounded-lg p-3 text-xs font-mono overflow-y-auto whitespace-pre-wrap ${maxHeight}`}
      >
        {code}
      </pre>
    </K8sSectionCard>
  );
}

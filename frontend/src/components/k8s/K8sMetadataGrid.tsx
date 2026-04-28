import { InfoItem } from "@/components/asset/detail/InfoItem";

interface MetadataItem {
  label: string;
  value: string;
  mono?: boolean;
}

interface K8sMetadataGridProps {
  items: MetadataItem[];
  className?: string;
}

export function K8sMetadataGrid({ items, className }: K8sMetadataGridProps) {
  return (
    <div className={`grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4 ${className || ""}`}>
      {items.map((item) => (
        <InfoItem key={item.label} label={item.label} value={item.value} mono={item.mono} />
      ))}
    </div>
  );
}

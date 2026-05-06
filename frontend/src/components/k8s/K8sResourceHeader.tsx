import { statusVariantToClass, type StatusVariant } from "./utils";

interface K8sResourceHeaderProps {
  name: string;
  subtitle?: string;
  status?: {
    text: string;
    variant: StatusVariant;
  };
}

export function K8sResourceHeader({ name, subtitle, status }: K8sResourceHeaderProps) {
  return (
    <div className="flex items-center justify-between mb-4">
      <div>
        <h3 className="font-mono text-sm font-medium">{name}</h3>
        {subtitle && <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>}
      </div>
      {status && (
        <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${statusVariantToClass(status.variant)}`}>
          {status.text}
        </span>
      )}
    </div>
  );
}

export type StatusVariant = "success" | "warning" | "error" | "info" | "neutral";

export function getK8sStatusColor(status: string): StatusVariant {
  const s = status.toLowerCase();
  if (s === "running" || s === "true" || s === "ready") return "success";
  if (s === "pending") return "warning";
  if (s === "failed" || s === "false" || s === "unknown") return "error";
  return "neutral";
}

export function getContainerStateColor(state: string): StatusVariant {
  if (state.startsWith("Running")) return "success";
  if (state.startsWith("Waiting")) return "warning";
  return "error";
}

export function statusVariantToClass(variant: StatusVariant): string {
  const map: Record<StatusVariant, string> = {
    success: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-400",
    warning: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-400",
    error: "bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-400",
    info: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-400",
    neutral: "bg-muted text-muted-foreground",
  };
  return map[variant];
}

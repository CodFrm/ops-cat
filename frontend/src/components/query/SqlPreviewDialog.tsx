import { useTranslation } from "react-i18next";
import { Copy, Loader2 } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  Button,
} from "@opskat/ui";
import { toast } from "sonner";
import { CodeEditor } from "@/components/CodeEditor";

interface SqlPreviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  statements: string[];
  // When provided, the dialog shows a confirm action (e.g. "Execute") and
  // treats the Cancel button as "Cancel". When omitted, the dialog is read-only
  // and the cancel slot renders as "Close".
  onConfirm?: () => void;
  submitting?: boolean;
}

export function SqlPreviewDialog({ open, onOpenChange, statements, onConfirm, submitting }: SqlPreviewDialogProps) {
  const { t } = useTranslation();
  const isConfirm = !!onConfirm;

  const handleCopy = async () => {
    const text = statements.join("\n\n");
    if (!text) return;
    await navigator.clipboard.writeText(text);
    toast.success(t("query.copied"));
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-w-2xl" onOverlayClick={() => onOpenChange(false)}>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("query.sqlPreviewTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("query.sqlPreviewDesc", { count: statements.length })}</AlertDialogDescription>
        </AlertDialogHeader>
        <div className="h-[360px] rounded-md border border-border overflow-hidden bg-muted/30">
          <CodeEditor value={statements.join("\n\n")} language="sql" readOnly />
        </div>
        <AlertDialogFooter>
          <Button
            variant="outline"
            size="sm"
            className="h-7 text-xs gap-1"
            onClick={handleCopy}
            disabled={!statements.length || submitting}
          >
            <Copy className="h-3.5 w-3.5" />
            {t("action.copy")}
          </Button>
          <AlertDialogCancel disabled={submitting}>
            {isConfirm ? t("action.cancel") : t("action.close")}
          </AlertDialogCancel>
          {isConfirm && (
            <AlertDialogAction variant="default" onClick={onConfirm} disabled={submitting}>
              {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : null}
              {t("query.confirmExecute")}
            </AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

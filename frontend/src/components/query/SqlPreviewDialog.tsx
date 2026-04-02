import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
  ScrollArea,
} from "@opskat/ui";

interface SqlPreviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  statements: string[];
  onConfirm: () => void;
  submitting?: boolean;
}

export function SqlPreviewDialog({ open, onOpenChange, statements, onConfirm, submitting }: SqlPreviewDialogProps) {
  const { t } = useTranslation();

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-w-2xl" onOverlayClick={() => onOpenChange(false)}>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("query.sqlPreviewTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("query.sqlPreviewDesc", { count: statements.length })}</AlertDialogDescription>
        </AlertDialogHeader>
        <ScrollArea className="max-h-[400px]">
          <pre className="bg-muted rounded-md p-3 text-xs font-mono whitespace-pre-wrap break-all border border-border">
            {statements.join("\n\n")}
          </pre>
        </ScrollArea>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>{t("action.cancel")}</AlertDialogCancel>
          <AlertDialogAction variant="default" onClick={onConfirm} disabled={submitting}>
            {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : null}
            {t("query.confirmExecute")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

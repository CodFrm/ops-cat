import { useTranslation } from "react-i18next";
import { CheckCircle2, Download, Loader2, Upload, X, XCircle } from "lucide-react";
import { Button, ScrollArea } from "@opskat/ui";
import { type SFTPTransfer, useSFTPStore } from "@/stores/sftpStore";

interface TransferSectionProps {
  sessionId: string;
  transfers: SFTPTransfer[];
}

function TransferRow({ transfer }: { transfer: SFTPTransfer }) {
  const cancelTransfer = useSFTPStore((s) => s.cancelTransfer);
  const clearTransfer = useSFTPStore((s) => s.clearTransfer);

  const percent = transfer.bytesTotal > 0 ? Math.round((transfer.bytesDone / transfer.bytesTotal) * 100) : 0;
  const fileName = transfer.currentFile ? transfer.currentFile.split("/").pop() || transfer.currentFile : "";

  return (
    <div className="flex items-center gap-1.5 text-[11px]">
      <div className="shrink-0">
        {transfer.status === "active" && <Loader2 className="h-3 w-3 animate-spin text-primary" />}
        {transfer.status === "done" && <CheckCircle2 className="h-3 w-3 text-green-500" />}
        {(transfer.status === "error" || transfer.status === "cancelled") && (
          <XCircle className="h-3 w-3 text-destructive" />
        )}
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1">
          {transfer.direction === "upload" ? (
            <Upload className="h-2.5 w-2.5 text-muted-foreground shrink-0" />
          ) : (
            <Download className="h-2.5 w-2.5 text-muted-foreground shrink-0" />
          )}
          <span className="truncate">{fileName}</span>
          {transfer.status === "active" && <span className="shrink-0 text-muted-foreground ml-auto">{percent}%</span>}
        </div>
        {transfer.status === "active" && (
          <div className="h-1 rounded-full bg-muted overflow-hidden mt-0.5">
            <div
              className="h-full rounded-full bg-primary transition-all duration-300"
              style={{ width: `${percent}%` }}
            />
          </div>
        )}
        {transfer.status === "error" && transfer.error && (
          <span className="text-destructive truncate block text-[10px]" title={transfer.error}>
            {transfer.error}
          </span>
        )}
      </div>

      <Button
        variant="ghost"
        size="icon-xs"
        className="shrink-0 h-4 w-4"
        onClick={() =>
          transfer.status === "active" ? cancelTransfer(transfer.transferId) : clearTransfer(transfer.transferId)
        }
      >
        <X className="h-2.5 w-2.5" />
      </Button>
    </div>
  );
}

export function TransferSection({ sessionId, transfers }: TransferSectionProps) {
  const { t } = useTranslation();
  const clearCompletedForSession = useSFTPStore((s) => s.clearCompletedForSession);

  if (transfers.length === 0) {
    return null;
  }

  return (
    <div className="border-t shrink-0">
      <div className="flex items-center justify-between px-2 py-0.5">
        <span className="text-[11px] font-medium text-muted-foreground">{t("sftp.transfers")}</span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="h-4 w-4"
          onClick={() => clearCompletedForSession(sessionId)}
          title={t("sftp.clear")}
        >
          <X className="h-2.5 w-2.5" />
        </Button>
      </div>
      <ScrollArea className="max-h-28">
        <div className="px-2 pb-1 space-y-0.5">
          {transfers.map((transfer) => (
            <TransferRow key={transfer.transferId} transfer={transfer} />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

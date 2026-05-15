import { useTranslation } from "react-i18next";
import { AlertTriangle, GitMerge } from "lucide-react";
import { Button, cn, Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@opskat/ui";
import type { ExternalEditSession } from "@/lib/externalEditApi";
import type { ExternalEditAttentionItem } from "@/stores/externalEditStore";

export type ExternalEditPendingItem =
  | ExternalEditAttentionItem
  | {
      id: string;
      type: "pending" | "conflict" | "remote_missing";
      session: ExternalEditSession;
      decisionType?: "pending" | "conflict";
      sourceType?: "runtime" | "recovery";
    };

const ACTION_BUTTON_CLASS =
  "!h-auto max-w-full justify-start !whitespace-normal break-words px-2 py-1.5 text-left leading-4";

interface PendingDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pendingItems: ExternalEditPendingItem[];
  savingSessionId: string | null;
  mergePrepareErrors: Record<string, string>;
  continueEditLabel: string;
  onOpenErrorDetail: (sessionId: string) => void;
  onMerge: (session: ExternalEditSession) => void | Promise<void>;
  onAcceptRemote: (session: ExternalEditSession) => void | Promise<void>;
  onOverwrite: (session: ExternalEditSession) => void | Promise<void>;
  onContinueEdit: (session: ExternalEditSession, sourceType?: "runtime" | "recovery") => void | Promise<void>;
}

export function ExternalEditPendingDialog({
  open,
  onOpenChange,
  pendingItems,
  savingSessionId,
  mergePrepareErrors,
  continueEditLabel,
  onOpenErrorDetail,
  onMerge,
  onAcceptRemote,
  onOverwrite,
  onContinueEdit,
}: PendingDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[82vh] max-w-3xl grid-rows-[auto,minmax(0,1fr),auto] gap-0 overflow-hidden p-0"
        data-testid="external-edit-pending-dialog"
      >
        <DialogHeader className="shrink-0 border-b px-6 py-4" data-testid="external-edit-pending-dialog-header">
          <DialogTitle>{t("externalEdit.pending.title")}</DialogTitle>
          <DialogDescription>{t("externalEdit.pending.description")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 space-y-3 overflow-y-auto px-6 py-4" data-testid="external-edit-pending-dialog-body">
          {pendingItems.length === 0 ? (
            <div className="rounded border bg-muted/20 px-3 py-4 text-sm text-muted-foreground">
              {t("externalEdit.pending.empty")}
            </div>
          ) : (
            pendingItems.map((item) => {
              const session = item.session;
              const fileName = session.remotePath.split("/").filter(Boolean).pop() || session.remotePath;
              const isPendingDecision = item.decisionType === "pending";
              const isConflictDecision = item.decisionType === "conflict";
              const isError = item.type === "error";
              const isRemoteMissing = item.type === "remote_missing" || session.state === "remote_missing";
              const cardTone = isConflictDecision
                ? "border-amber-400/30 bg-amber-500/5"
                : isPendingDecision
                  ? "border-sky-400/30 bg-sky-500/5"
                  : isError
                    ? "border-rose-400/30 bg-rose-500/5"
                    : "border-amber-400/30 bg-amber-500/5";
              return (
                <div
                  key={item.id}
                  className={cn("rounded border px-4 py-4 text-sm", cardTone)}
                  data-testid={`external-edit-pending-${item.type}`}
                >
                  <div className="flex flex-col gap-3">
                    <div className="min-w-0 space-y-1.5" data-testid={`external-edit-pending-content-${session.id}`}>
                      <div
                        className="break-words font-medium text-foreground"
                        data-testid={`external-edit-pending-file-${session.id}`}
                      >
                        {fileName}
                      </div>
                      <div
                        className="break-all whitespace-normal text-xs text-muted-foreground"
                        data-testid={`external-edit-pending-path-${session.id}`}
                      >
                        {session.remotePath}
                      </div>
                      <div
                        className="break-words whitespace-normal text-xs leading-5 text-muted-foreground"
                        data-testid={`external-edit-pending-summary-${session.id}`}
                      >
                        {isConflictDecision && t("externalEdit.conflict.remoteChangedTitle")}
                        {isPendingDecision && t("externalEdit.recovery.summary")}
                        {!isConflictDecision &&
                          !isPendingDecision &&
                          !isError &&
                          isRemoteMissing &&
                          t("externalEdit.conflict.remoteMissingTitle")}
                        {isError && (session.lastError?.summary || t("externalEdit.error.title"))}
                      </div>
                    </div>
                    <div
                      className="flex w-full flex-wrap items-start gap-2"
                      data-testid={`external-edit-pending-actions-${session.id}`}
                    >
                      {isConflictDecision && (
                        <>
                          <Button
                            size="xs"
                            variant="outline"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id || session.state !== "conflict"}
                            onClick={() => void onMerge(session)}
                          >
                            <GitMerge className="mr-1 h-3 w-3" />
                            {t("externalEdit.actions.merge")}
                          </Button>
                          <Button
                            size="xs"
                            variant="outline"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id || session.state !== "conflict"}
                            onClick={() => void onAcceptRemote(session)}
                          >
                            {t("externalEdit.actions.acceptRemote")}
                          </Button>
                          <Button
                            size="xs"
                            variant="destructive"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id || session.state !== "conflict"}
                            onClick={() => void onOverwrite(session)}
                          >
                            {t("externalEdit.actions.overwrite")}
                          </Button>
                        </>
                      )}
                      {isPendingDecision && (
                        <>
                          <Button
                            size="xs"
                            variant="outline"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id}
                            onClick={() => void onContinueEdit(session, item.sourceType)}
                          >
                            {continueEditLabel}
                          </Button>
                          <Button
                            size="xs"
                            variant="outline"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id}
                            onClick={() => void onAcceptRemote(session)}
                          >
                            {t("externalEdit.actions.reread")}
                          </Button>
                          <Button
                            size="xs"
                            variant="destructive"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id}
                            onClick={() => void onOverwrite(session)}
                          >
                            {t("externalEdit.actions.overwrite")}
                          </Button>
                        </>
                      )}
                      {!isConflictDecision && !isPendingDecision && isRemoteMissing && (
                        <>
                          <Button
                            size="xs"
                            variant="outline"
                            className={ACTION_BUTTON_CLASS}
                            disabled={savingSessionId === session.id}
                            onClick={() => void onOverwrite(session)}
                          >
                            {t("externalEdit.actions.saveAgain")}
                          </Button>
                        </>
                      )}
                      {isError && (
                        <>
                          <Button
                            size="xs"
                            variant="outline"
                            className={ACTION_BUTTON_CLASS}
                            onClick={() => onOpenErrorDetail(session.id)}
                          >
                            <AlertTriangle className="mr-1 h-3 w-3" />
                            {t("externalEdit.actions.viewError")}
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                  {mergePrepareErrors[session.id] && (
                    <div className="mt-2 rounded border border-destructive/30 bg-destructive/5 px-2 py-1 text-xs text-destructive">
                      {mergePrepareErrors[session.id]}
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
        <div className="flex shrink-0 justify-end border-t px-6 py-3" data-testid="external-edit-pending-dialog-footer">
          <Button
            variant="outline"
            onClick={() => {
              onOpenChange(false);
            }}
          >
            {t("action.close")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

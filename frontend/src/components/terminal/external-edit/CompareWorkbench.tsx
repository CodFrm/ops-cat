import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { Button } from "@opskat/ui";
import { CodeDiffViewer } from "@/components/CodeDiffViewer";
import type { ExternalEditCompareResult } from "@/lib/externalEditApi";
import { ExternalEditIdeaFrame } from "./IdeaFrame";

interface CompareWorkbenchProps {
  compareResult: ExternalEditCompareResult;
  onDismiss: () => void;
}

export function ExternalEditCompareWorkbench({ compareResult, onDismiss }: CompareWorkbenchProps) {
  const { t } = useTranslation();
  const [activeBlockIndex, setActiveBlockIndex] = useState(0);
  const [navigationToken, setNavigationToken] = useState(0);
  const [diffTotal, setDiffTotal] = useState(0);

  useEffect(() => {
    setActiveBlockIndex(0);
    setNavigationToken((token) => token + 1);
  }, [compareResult]);

  const navigate = (direction: -1 | 1) => {
    if (diffTotal === 0) return;
    setActiveBlockIndex((current) => {
      const next = Math.min(Math.max(current + direction, 0), diffTotal - 1);
      if (next !== current) {
        setNavigationToken((token) => token + 1);
      }
      return next;
    });
  };

  return (
    <ExternalEditIdeaFrame
      fileName={compareResult.fileName}
      helper={t("externalEdit.compare.helper")}
      layoutLabel={t("externalEdit.compare.remoteLeftLocalRight")}
      mode="compare"
      remotePath={compareResult.remotePath}
      sidebarLabel={t("externalEdit.compare.projectView")}
      status={t("externalEdit.compare.status")}
      testId="external-edit-compare-workbench"
      title={t("externalEdit.compare.title")}
      actions={
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="xs"
            className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
            disabled={diffTotal === 0 || activeBlockIndex === 0}
            onClick={() => navigate(-1)}
          >
            {t("externalEdit.compare.previous")}
          </Button>
          <div
            className="min-w-14 rounded border border-slate-600 bg-slate-800 px-2 py-1 text-center text-xs text-slate-200"
            data-testid="external-edit-compare-diff-count"
          >
            {diffTotal === 0 ? "0 / 0" : `${activeBlockIndex + 1} / ${diffTotal}`}
          </div>
          <Button
            variant="outline"
            size="xs"
            className="border-slate-600 bg-transparent text-slate-200 hover:bg-slate-700 hover:text-white"
            disabled={diffTotal === 0 || activeBlockIndex >= diffTotal - 1}
            onClick={() => navigate(1)}
          >
            {t("externalEdit.compare.next")}
          </Button>
          <Button
            size="icon"
            variant="ghost"
            className="text-slate-300 hover:bg-slate-700 hover:text-white"
            onClick={onDismiss}
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      }
    >
      <div
        className="min-h-0 flex-1 bg-[#1f2329] p-2"
        data-idea-layout="read-only-diff"
        data-testid="external-edit-compare-idea-layout"
      >
        <CodeDiffViewer
          activeBlockIndex={activeBlockIndex}
          badge={t("externalEdit.compare.readOnly")}
          className="border-slate-700 bg-[#f8fafc] text-slate-950 dark:bg-[#1f2329] dark:text-slate-100"
          height="100%"
          language="plaintext"
          modified={compareResult.localContent || ""}
          modifiedTitle={t("externalEdit.compare.localDraft")}
          navigationToken={navigationToken}
          original={compareResult.remoteContent || ""}
          originalTitle={t("externalEdit.compare.remoteSnapshot")}
          testId="external-edit-compare-diff-editor"
          onDiffStatsChange={({ total }) => {
            setDiffTotal(total);
            setActiveBlockIndex((current) => Math.min(current, Math.max(total - 1, 0)));
          }}
        />
      </div>
    </ExternalEditIdeaFrame>
  );
}

import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@opskat/ui";
import { CircleCheck, Info, TriangleAlert, OctagonX, ChevronDown, ChevronRight } from "lucide-react";
import { GetSystemStatus } from "../../../wailsjs/go/app/App";

interface StatusEntry {
  level: "info" | "warn" | "error";
  source: string;
  message: string;
  detail: string;
  time: string;
}

const levelConfig = {
  info: { icon: Info, color: "text-blue-500", bg: "bg-blue-50 dark:bg-blue-950/30" },
  warn: { icon: TriangleAlert, color: "text-yellow-500", bg: "bg-yellow-50 dark:bg-yellow-950/30" },
  error: { icon: OctagonX, color: "text-red-500", bg: "bg-red-50 dark:bg-red-950/30" },
};

function StatusEntryItem({ entry }: { entry: StatusEntry }) {
  const [expanded, setExpanded] = useState(false);
  const config = levelConfig[entry.level] || levelConfig.info;
  const Icon = config.icon;

  return (
    <div className={`rounded-md border p-3 ${config.bg}`}>
      <div className="flex items-center gap-2 cursor-pointer" onClick={() => entry.detail && setExpanded(!expanded)}>
        <Icon className={`h-4 w-4 shrink-0 ${config.color}`} />
        <span className="text-xs font-medium text-muted-foreground px-1.5 py-0.5 rounded bg-muted">{entry.source}</span>
        <span className="text-sm flex-1">{entry.message}</span>
        {entry.detail && (
          <button className="text-muted-foreground hover:text-foreground">
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </button>
        )}
      </div>
      {expanded && entry.detail && (
        <pre className="mt-2 text-xs text-muted-foreground bg-muted/50 rounded p-2 overflow-x-auto whitespace-pre-wrap break-all">
          {entry.detail}
        </pre>
      )}
    </div>
  );
}

export function SystemStatusSection() {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<StatusEntry[]>([]);

  useEffect(() => {
    GetSystemStatus().then((data) => {
      setEntries((data as StatusEntry[]) || []);
    });
  }, []);

  const hasProblems = entries.some((e) => e.level === "warn" || e.level === "error");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("systemStatus.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {!hasProblems ? (
          <div className="flex flex-col items-center justify-center py-8 text-center">
            <CircleCheck className="h-12 w-12 text-green-500 mb-3" />
            <p className="font-medium">{t("systemStatus.allGood")}</p>
            <p className="text-sm text-muted-foreground mt-1">{t("systemStatus.allGoodDesc")}</p>
          </div>
        ) : (
          <div className="space-y-2">
            {entries.map((entry, i) => (
              <StatusEntryItem key={i} entry={entry} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

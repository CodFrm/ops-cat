import { useState } from "react";
import { useTranslation } from "react-i18next";
import { FlaskConical, ShieldCheck, ShieldX, ShieldAlert, Loader2 } from "lucide-react";
import { cn, Input, Button } from "@opskat/ui";
import { TestPolicyRule } from "../../../wailsjs/go/app/App";
import { app } from "../../../wailsjs/go/models";

interface PolicyTestPanelProps {
  policyType: string;
  buildPolicyJSON: () => string;
  assetID?: number;
  groupID?: number;
}

interface TestResult {
  decision: "allow" | "deny" | "need_confirm";
  matchedPattern: string;
  matchedSource: string;
  message: string;
}

const PLACEHOLDER_MAP: Record<string, string> = {
  ssh: "asset.policyTestPlaceholder",
  k8s: "asset.k8sPolicyTestPlaceholder",
  database: "asset.policyTestSqlPlaceholder",
  redis: "asset.policyTestRedisPlaceholder",
  mongodb: "asset.policyTestMongoPlaceholder",
};

const RESULT_CONFIG = {
  allow: {
    icon: ShieldCheck,
    label: "asset.policyTestAllow",
    className: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/20",
  },
  deny: {
    icon: ShieldX,
    label: "asset.policyTestDeny",
    className: "bg-red-500/10 text-red-700 dark:text-red-400 border-red-500/20",
  },
  need_confirm: {
    icon: ShieldAlert,
    label: "asset.policyTestConfirm",
    className: "bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/20",
  },
};

export function PolicyTestPanel({ policyType, buildPolicyJSON, assetID, groupID }: PolicyTestPanelProps) {
  const { t } = useTranslation();
  const [command, setCommand] = useState("");
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<TestResult | null>(null);

  const handleTest = async () => {
    const cmd = command.trim();
    if (!cmd) return;

    setTesting(true);
    setResult(null);
    try {
      const req = new app.PolicyTestRequest({
        policyType,
        policyJSON: buildPolicyJSON(),
        command: cmd,
        assetID: assetID || 0,
        groupID: groupID || 0,
      });
      const res = await TestPolicyRule(req);
      setResult({
        decision: res.decision as TestResult["decision"],
        matchedPattern: res.matchedPattern,
        matchedSource: res.matchedSource,
        message: res.message,
      });
    } catch {
      setResult({ decision: "deny", matchedPattern: "", matchedSource: "", message: "Test failed" });
    } finally {
      setTesting(false);
    }
  };

  const cfg = result ? RESULT_CONFIG[result.decision] : null;
  const ResultIcon = cfg?.icon;

  return (
    <div className="mt-3 pt-3 border-t">
      <div className="flex items-center gap-1.5 mb-2">
        <FlaskConical className="h-3.5 w-3.5 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">{t("asset.policyTest")}</span>
      </div>

      <div className="flex gap-2">
        <Input
          className="h-7 text-xs font-mono flex-1"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              handleTest();
            }
          }}
          placeholder={t(PLACEHOLDER_MAP[policyType])}
        />
        <Button
          size="sm"
          variant="outline"
          className="h-7 px-3 text-xs"
          onClick={handleTest}
          disabled={testing || !command.trim()}
        >
          {testing ? <Loader2 className="h-3 w-3 animate-spin" /> : t("asset.policyTestButton")}
        </Button>
      </div>

      {result && cfg && ResultIcon && (
        <div className={cn("flex items-center gap-2 mt-2 px-2.5 py-1.5 rounded-md border text-xs", cfg.className)}>
          <ResultIcon className="h-3.5 w-3.5 shrink-0" />
          <span className="font-medium">{t(cfg.label)}</span>
          {result.matchedSource && result.matchedSource !== "default" && (
            <>
              <span className="text-muted-foreground">|</span>
              <span className="opacity-80">{t("asset.policyTestSourceGroup", { name: result.matchedSource })}</span>
            </>
          )}
          {result.matchedPattern && (
            <>
              <span className="text-muted-foreground">|</span>
              <span className="font-mono opacity-80">
                {t("asset.policyTestMatched", { pattern: result.matchedPattern })}
              </span>
            </>
          )}
        </div>
      )}
    </div>
  );
}

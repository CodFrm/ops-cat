import { memo, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { RotateCcw } from "lucide-react";
import type { ChatMessageRetryStatus } from "@/stores/aiStore";
import { classifyError } from "@/lib/aiError";

// RetryBanner —— assistant 气泡顶部的"重试中"提示条。
//
// 仅当 ChatMessage.retryStatus 非空时挂载；retryStatus 在任何非 retry 流式事件
// 到达时被 aiStore handleStreamEvent 清掉（见 clearRetryStatusOnLastAssistant），
// 因此重试一旦成功，banner 自然卸载，无须组件内部判定。
//
// 倒计时基于 startedAt + delayMs，避免 setInterval 累积误差：每秒重算剩余秒数；
// 归零后改显示"正在重试..."直到下一帧事件到达卸载该组件。
function computeRemaining(status: ChatMessageRetryStatus): number {
  const target = status.startedAt + status.delayMs;
  return Math.max(0, Math.ceil((target - Date.now()) / 1000));
}

export const RetryBanner = memo(function RetryBanner({ status }: { status: ChatMessageRetryStatus }) {
  const { t } = useTranslation();
  const statusTimerKey = `${status.startedAt}:${status.delayMs}`;
  const [remainingState, setRemainingState] = useState(() => ({
    timerKey: statusTimerKey,
    value: computeRemaining(status),
  }));
  const remaining = remainingState.timerKey === statusTimerKey ? remainingState.value : computeRemaining(status);

  useEffect(() => {
    if (status.delayMs <= 0) return;
    const id = window.setInterval(() => {
      const next = computeRemaining(status);
      setRemainingState({ timerKey: statusTimerKey, value: next });
      if (next <= 0) window.clearInterval(id);
    }, 1000);
    return () => window.clearInterval(id);
  }, [status, statusTimerKey]);

  const attemptLabel = status.attempt > 0 ? t("ai.retry.attempt", { n: status.attempt }) : "";
  const countdownLabel = remaining > 0 ? t("ai.retry.countdown_seconds", { n: remaining }) : t("ai.retry.now");
  // 错误归因 + 原始错误：用户在 retry 期间想知道为什么在重试（503 / 网络 / 鉴权 …）。
  // 友好标题走 classifyError(走和 ErrorBlock 同一份分类规则)，原始 cause 紧随其后以
  // monospace 小字呈现，方便复制 request id。
  const classified = status.cause ? classifyError(status.cause) : null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex flex-col gap-0.5 rounded-md border border-amber-400/40 bg-amber-50 dark:bg-amber-950/40 px-2.5 py-1 text-xs text-amber-700 dark:text-amber-300"
    >
      <div className="flex items-center gap-1.5">
        <RotateCcw className="h-3 w-3 animate-spin-slow" aria-hidden="true" />
        <span className="font-medium">{t("ai.retry.retrying")}</span>
        {attemptLabel && <span className="opacity-70">·</span>}
        {attemptLabel && <span>{attemptLabel}</span>}
        <span className="opacity-70">·</span>
        <span>{countdownLabel}</span>
        {classified && <span className="opacity-70">·</span>}
        {classified && <span className="font-medium">{classified.message}</span>}
      </div>
      {status.cause && (
        <pre className="whitespace-pre-wrap break-words text-[11px] font-mono leading-relaxed m-0 opacity-80">
          {status.cause}
        </pre>
      )}
    </div>
  );
});

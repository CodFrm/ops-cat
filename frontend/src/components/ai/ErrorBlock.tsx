import { memo } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle } from "lucide-react";
import type { ContentBlock } from "@/stores/aiStore";
import type { ErrorKind } from "@/lib/aiError";

// ErrorBlock —— 持久化的对话级错误块。
//
// 由两条路径生成：
//   1) EventError 命中后由 aiStore handleEvent 的 "error" case 推入；
//   2) 重试中被关闭 tab / 切会话 / 应用退出时，由 materializeRetryStatusAsError
//      把 retryStatus 物化为 kind="interrupted"。
//
// 用户明确要求该块纯展示 —— 不带"重试"/"复制"/"已达上限"等按钮。
// 用户如需重试，可走顶部 AssistantToolbar 的"重新生成"按钮（统一入口）。
export const ErrorBlock = memo(function ErrorBlock({ block }: { block: ContentBlock }) {
  const { t } = useTranslation();
  const kind = (block.errorKind as ErrorKind | undefined) ?? "unknown";
  // 优先用 block.content（落盘时已经是 classifyError 后的友好标题），缺失时按 kind 取 i18n。
  const title = block.content || t(`ai.error.${kind}`);
  const detail = block.errorDetail || "";

  return (
    <div role="alert" className="rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2 text-sm">
      <div className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
        <div className="min-w-0 flex-1 space-y-1">
          <div className="font-medium text-destructive">{title}</div>
          {detail && (
            <pre className="whitespace-pre-wrap break-words text-xs text-muted-foreground font-mono leading-relaxed m-0">
              {detail}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
});

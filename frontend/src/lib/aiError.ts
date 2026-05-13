import i18n from "../i18n";

// ErrorKind: AI 对话错误分类。
// rate_limit  — 429 / 限流
// server      — 5xx / 服务暂不可用
// network     — 连接抖动、超时、TLS 握手失败、EOF
// auth        — 401/403 / API Key 无效（cago 默认不重试，UI 直接呈现）
// interrupted — 重试进行中被用户关闭 tab / 切会话 / 应用退出 时落盘的中断标记
// unknown     — 未命中任何模式，按原文展示
export type ErrorKind = "rate_limit" | "server" | "network" | "auth" | "interrupted" | "unknown";

const I18N_KEY_BY_KIND: Record<ErrorKind, string> = {
  rate_limit: "ai.error.rate_limit",
  server: "ai.error.server",
  network: "ai.error.network",
  auth: "ai.error.auth",
  interrupted: "ai.error.interrupted",
  unknown: "ai.error.unknown",
};

const FALLBACK_LABEL_BY_KIND: Record<ErrorKind, string> = {
  rate_limit: "请求过于频繁，稍后再试",
  server: "服务暂时不可用",
  network: "网络连接不稳定",
  auth: "鉴权失败，请检查 API Key",
  interrupted: "对话被中断",
  unknown: "对话出错",
};

// 匹配优先级遵循 README 的从高到低顺序：
// auth 优先于 rate_limit/server（避免 401 被 5xx 关键字误捞），
// rate_limit/network 优先于 server（429/timeout 在响应里有时也带 5xx 串）。
const PATTERNS: Array<{ kind: ErrorKind; re: RegExp }> = [
  { kind: "auth", re: /\b(401|403)\b|api[\s._-]?key|invalid_api|authentication|unauthorized/i },
  { kind: "rate_limit", re: /\b429\b|too many|rate[\s._-]?limit/i },
  { kind: "network", re: /timeout|EOF|connection reset|broken pipe|i\/o timeout|tls handshake/i },
  { kind: "server", re: /\b5\d{2}\b|service unavailable|bad gateway|gateway timeout|server error/i },
];

export interface ClassifiedError {
  kind: ErrorKind;
  message: string;
}

// classifyError 把原始 error 文本归类成 ErrorKind + 友好标题。
// 显式传入 kind 时跳过正则（用于 interrupted 路径），message 仍走 i18n。
export function classifyError(raw: string | undefined | null, explicitKind?: ErrorKind): ClassifiedError {
  const text = (raw ?? "").trim();
  let kind: ErrorKind = explicitKind ?? "unknown";
  if (!explicitKind) {
    for (const { kind: k, re } of PATTERNS) {
      if (re.test(text)) {
        kind = k;
        break;
      }
    }
  }
  const message = i18n.t(I18N_KEY_BY_KIND[kind], FALLBACK_LABEL_BY_KIND[kind]);
  return { kind, message };
}

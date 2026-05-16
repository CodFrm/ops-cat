export const MASKED_SECRET = "\u25CF\u25CF\u25CF\u25CF\u25CF\u25CF";
export const ENABLED_VALUE = "\u2713";
export const DISABLED_VALUE = "\u2717";

export function parseDetailConfig<T>(config?: string): T | null {
  try {
    return JSON.parse(config || "{}") as T;
  } catch {
    return null;
  }
}

import { Settings, KeyRound, ScrollText, ArrowRightLeft, FileCode } from "lucide-react";
import type { TFunction } from "i18next";
import type { Tab, PageTabMeta } from "@/stores/tabStore";

export interface BuiltinPageMeta {
  icon: typeof Settings;
  labelKey: string;
}

export const builtinPageTabMeta: Record<string, BuiltinPageMeta> = {
  settings: { icon: Settings, labelKey: "nav.settings" },
  forward: { icon: ArrowRightLeft, labelKey: "nav.forward" },
  sshkeys: { icon: KeyRound, labelKey: "nav.sshKeys" },
  audit: { icon: ScrollText, labelKey: "nav.audit" },
  snippets: { icon: FileCode, labelKey: "nav.snippets" },
};

export function getBuiltinPageMeta(tab: Tab): BuiltinPageMeta | undefined {
  if (tab.type !== "page") return undefined;
  return builtinPageTabMeta[(tab.meta as PageTabMeta).pageId];
}

export function resolveTabLabel(tab: Tab, t: TFunction): string {
  const meta = getBuiltinPageMeta(tab);
  return meta ? t(meta.labelKey) : tab.label;
}

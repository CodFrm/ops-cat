import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@opskat/ui";
import { useAIStore } from "@/stores/aiStore";
import { CreateAIProvider, SetActiveAIProvider } from "../../../wailsjs/go/app/App";
import { Bot, Zap, Sparkles, ArrowRight, Settings, SquareTerminal } from "lucide-react";
import { toast } from "sonner";
import { AIProviderForm, type AIProviderFormValues } from "./AIProviderForm";
import { useTabStore } from "@/stores/tabStore";

export function AISetupWizard() {
  const { t } = useTranslation();
  const [providerType, setProviderType] = useState<"openai" | "anthropic" | null>(null);
  const [saving, setSaving] = useState(false);

  const goToSettings = () => {
    const tabStore = useTabStore.getState();
    const existing = tabStore.tabs.find((tab) => tab.id === "settings");
    if (existing) {
      tabStore.activateTab("settings");
    } else {
      tabStore.openTab({
        id: "settings",
        type: "page",
        label: t("nav.settings"),
        meta: { type: "page", pageId: "settings" },
      });
    }
  };

  const handleSave = async (values: AIProviderFormValues) => {
    setSaving(true);
    try {
      const created = await CreateAIProvider(
        values.name,
        values.type,
        values.apiBase,
        values.apiKey,
        values.model,
        values.maxOutputTokens,
        values.contextWindow,
        values.reasoningEnabled,
        values.reasoningEffort
      );
      await SetActiveAIProvider(created.id);
      await useAIStore.getState().checkConfigured();
      await useAIStore.getState().fetchConversations();
    } catch (e: unknown) {
      toast.error(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex h-full overflow-y-auto">
      <div className="m-auto w-full max-w-xl p-6 space-y-6">
        {/* Header */}
        <div className="flex items-center gap-2.5">
          <Bot className="h-5 w-5 text-primary" />
          <h2 className="text-lg font-semibold">{t("setup.title")}</h2>
        </div>

        {/* opsctl Plugin Banner */}
        <div className="rounded-xl border border-border/60 bg-gradient-to-r from-primary/5 to-purple-500/5 p-5 flex items-center gap-4">
          <div className="flex-1 space-y-2">
            <div className="flex items-center gap-2">
              <SquareTerminal className="h-4 w-4 text-primary" />
              <span className="text-sm font-semibold">{t("setup.opsctlBannerTitle")}</span>
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed">{t("setup.opsctlBannerDesc")}</p>
            <div className="flex gap-1.5">
              {["Claude Code", "Codex", "Gemini CLI"].map((tag) => (
                <span
                  key={tag}
                  className="text-[11px] font-medium text-muted-foreground bg-muted/60 rounded px-2 py-0.5 font-mono"
                >
                  {tag}
                </span>
              ))}
            </div>
          </div>
          <Button variant="outline" size="sm" onClick={goToSettings} className="shrink-0">
            {t("setup.goToSettings")}
            <ArrowRight className="h-3.5 w-3.5 ml-1" />
          </Button>
        </div>

        {/* Provider Type Selection */}
        <div className="space-y-3">
          <h3 className="text-sm font-semibold">{t("setup.selectProvider")}</h3>
          <div className="grid grid-cols-2 gap-3">
            <button
              onClick={() => setProviderType("openai")}
              className={`rounded-lg border-2 p-4 text-left transition-all hover:border-primary/50 ${
                providerType === "openai" ? "border-primary bg-primary/5" : "border-border"
              }`}
            >
              <div className="flex items-center gap-2.5 mb-2">
                <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-primary/10">
                  <Zap className="h-4.5 w-4.5 text-primary" />
                </div>
                <div>
                  <div className="font-medium text-sm">{t("setup.openAICompatible")}</div>
                  <div className="text-[11px] text-primary font-medium">{t("setup.recommended")}</div>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">{t("setup.openAICompatibleDesc")}</p>
            </button>

            <button
              onClick={() => setProviderType("anthropic")}
              className={`rounded-lg border-2 p-4 text-left transition-all hover:border-primary/50 ${
                providerType === "anthropic" ? "border-primary bg-primary/5" : "border-border"
              }`}
            >
              <div className="flex items-center gap-2.5 mb-2">
                <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-amber-500/10">
                  <Sparkles className="h-4.5 w-4.5 text-amber-500" />
                </div>
                <div>
                  <div className="font-medium text-sm">{t("settings.anthropic")}</div>
                  <div className="text-[11px] text-muted-foreground font-medium">Claude API</div>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">{t("setup.anthropicDesc")}</p>
            </button>
          </div>
        </div>

        {/* Configuration Form */}
        {providerType && (
          <div className="space-y-4">
            <div className="rounded-lg border p-5">
              <h3 className="text-sm font-semibold mb-4">
                {t("setup.configureProvider", {
                  provider: providerType === "openai" ? t("setup.openAICompatible") : "Anthropic",
                })}
              </h3>
              <AIProviderForm
                key={providerType}
                providerType={providerType}
                showTypeSelector={false}
                onSave={handleSave}
                saving={saving}
                submitLabel={t("setup.complete")}
                submitIcon={<ArrowRight className="h-4 w-4 mr-1.5" />}
              />
            </div>
          </div>
        )}

        {/* Bottom Hint */}
        <div className="flex items-center justify-center gap-1.5 text-xs text-muted-foreground">
          <Settings className="h-3 w-3" />
          <span>{t("setup.settingsHint")}</span>
        </div>
      </div>
    </div>
  );
}

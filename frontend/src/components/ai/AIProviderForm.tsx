import { useState, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollableContainer,
} from "@opskat/ui";
import { Check, ChevronsUpDown, Loader2, RefreshCw } from "lucide-react";
import { FetchAIModels, GetModelDefaults } from "../../../wailsjs/go/app/App";
import { app } from "../../../wailsjs/go/models";
import { toast } from "sonner";

function getDefaultApiBase(providerType: string): string {
  if (providerType === "anthropic") return "https://api.anthropic.com";
  return "https://api.openai.com/v1";
}

export interface AIProviderFormValues {
  name: string;
  type: string;
  apiBase: string;
  apiKey: string;
  model: string;
  maxOutputTokens: number;
  contextWindow: number;
  reasoningEnabled: boolean;
  reasoningEffort: "none" | "low" | "medium" | "high" | "xhigh";
}

function supportsOpenAIReasoningModel(model: string): boolean {
  const lower = model.trim().toLowerCase();
  return (
    lower.startsWith("o1") ||
    lower.startsWith("o3") ||
    lower.startsWith("o4") ||
    lower.startsWith("gpt-5") ||
    lower.startsWith("deepseek")
  );
}

export interface AIProviderFormProps {
  initialValues?: {
    name: string;
    type: string;
    apiBase: string;
    apiKey: string;
    maskedApiKey?: string;
    model: string;
    maxOutputTokens: number;
    contextWindow: number;
    reasoningEffort: "none" | "low" | "medium" | "high" | "xhigh";
  };
  isEditing?: boolean;
  /** Locks the provider type (used by wizard where cards handle type selection) */
  providerType?: "openai" | "anthropic";
  showTypeSelector?: boolean;
  onSave: (values: AIProviderFormValues) => Promise<void>;
  submitLabel?: string;
  submitIcon?: React.ReactNode;
  saving?: boolean;
}

export function AIProviderForm({
  initialValues,
  isEditing,
  providerType: externalType,
  showTypeSelector = true,
  onSave,
  submitLabel,
  submitIcon,
  saving = false,
}: AIProviderFormProps) {
  const { t } = useTranslation();

  const [formName, setFormName] = useState(initialValues?.name ?? "");
  const [formType, setFormType] = useState(initialValues?.type ?? externalType ?? "openai");
  const [formApiBase, setFormApiBase] = useState(initialValues?.apiBase ?? "");
  const [formApiKey, setFormApiKey] = useState(initialValues?.apiKey ?? "");
  const [formModel, setFormModel] = useState(initialValues?.model ?? "");
  const [formMaxOutputTokens, setFormMaxOutputTokens] = useState(initialValues?.maxOutputTokens ?? 0);
  const [formContextWindow, setFormContextWindow] = useState(initialValues?.contextWindow ?? 0);
  const [formReasoningEffort, setFormReasoningEffort] = useState<"none" | "low" | "medium" | "high" | "xhigh">(
    initialValues?.reasoningEffort ?? "none"
  );

  const [modelOptions, setModelOptions] = useState<app.AIModelInfo[]>([]);
  const [modelPopoverOpen, setModelPopoverOpen] = useState(false);
  const [modelSearch, setModelSearch] = useState("");
  const [fetchingModels, setFetchingModels] = useState(false);
  const isOpenAIProvider = formType === "openai";
  const currentModelSupportsReasoning = supportsOpenAIReasoningModel(formModel);

  // When external providerType prop changes (wizard card click), reset relevant fields
  useEffect(() => {
    if (externalType) {
      setFormType(externalType);
      setFormName(externalType === "anthropic" ? "Anthropic" : t("setup.openAICompatible"));
      setFormApiBase("");
      setFormApiKey("");
      setFormModel("");
      setFormMaxOutputTokens(0);
      setFormContextWindow(0);
      setFormReasoningEffort("none");
      setModelOptions([]);
    }
  }, [externalType, t]);

  const filteredModels = modelOptions.filter((m) => m.id.toLowerCase().includes(modelSearch.toLowerCase()));

  const handleFetchModels = useCallback(async () => {
    if (!formApiKey) {
      toast.error(t("settings.apiKey") + " required");
      return;
    }
    setFetchingModels(true);
    try {
      const models = await FetchAIModels(formType, formApiBase || getDefaultApiBase(formType), formApiKey);
      setModelOptions(models || []);
      if (models && models.length > 0) {
        setModelPopoverOpen(true);
      } else {
        toast.info(t("settings.noModelsFound"));
      }
    } catch (e) {
      toast.error(`${t("settings.fetchModelsError")}: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setFetchingModels(false);
    }
  }, [formApiKey, formType, formApiBase, t]);

  const handleSelectModel = useCallback(
    (model: app.AIModelInfo) => {
      setFormModel(model.id);
      setModelPopoverOpen(false);
      if (model.maxOutputTokens > 0) {
        setFormMaxOutputTokens(model.maxOutputTokens);
      }
      if (model.contextWindow > 0) {
        setFormContextWindow(model.contextWindow);
      }
      if (model.maxOutputTokens > 0 || model.contextWindow > 0) {
        toast.info(t("settings.modelDefaultsApplied"));
      }
    },
    [t]
  );

  const handleModelInputBlur = useCallback(async () => {
    if (!formModel) return;
    try {
      const defaults = await GetModelDefaults(formModel);
      if (defaults) {
        if (defaults.maxOutputTokens > 0) {
          setFormMaxOutputTokens(defaults.maxOutputTokens);
        }
        if (defaults.contextWindow > 0) {
          setFormContextWindow(defaults.contextWindow);
        }
        toast.info(t("settings.modelDefaultsApplied"));
      }
    } catch {
      // Unknown model, ignore
    }
  }, [formModel, t]);

  const handleSubmit = async () => {
    await onSave({
      name: formName,
      type: formType,
      apiBase: formApiBase,
      apiKey: formApiKey,
      model: formModel,
      maxOutputTokens: formMaxOutputTokens,
      contextWindow: formContextWindow,
      reasoningEnabled: isOpenAIProvider && formReasoningEffort !== "none",
      reasoningEffort: formReasoningEffort,
    });
  };

  return (
    <div className="space-y-4">
      {showTypeSelector && (
        <div className="grid gap-2">
          <Label>{t("settings.providerType")}</Label>
          <Select value={formType} onValueChange={setFormType}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="openai">{t("settings.openai")}</SelectItem>
              <SelectItem value="anthropic">{t("settings.anthropic")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      )}

      <div className="grid gap-2">
        <Label>{t("settings.providerName")}</Label>
        <Input value={formName} onChange={(e) => setFormName(e.target.value)} />
      </div>

      <div className="grid gap-2">
        <Label>{t("settings.model")}</Label>
        <div className="flex gap-2">
          <Popover open={modelPopoverOpen} onOpenChange={setModelPopoverOpen}>
            <PopoverTrigger asChild>
              <div className="relative flex-1">
                <Input
                  value={formModel}
                  onChange={(e) => {
                    setFormModel(e.target.value);
                    setModelSearch(e.target.value);
                    if (modelOptions.length > 0) setModelPopoverOpen(true);
                  }}
                  onBlur={handleModelInputBlur}
                  placeholder={t("settings.selectModel")}
                  className="pr-8"
                />
                {modelOptions.length > 0 && (
                  <button
                    type="button"
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    aria-label={t("settings.selectModel")}
                    title={t("settings.selectModel")}
                    onClick={() => setModelPopoverOpen(!modelPopoverOpen)}
                  >
                    <ChevronsUpDown className="h-4 w-4" />
                  </button>
                )}
              </div>
            </PopoverTrigger>
            {modelOptions.length > 0 && (
              <PopoverContent className="p-0 w-[var(--radix-popover-trigger-width)]" align="start">
                <div className="p-2">
                  <Input
                    value={modelSearch}
                    onChange={(e) => setModelSearch(e.target.value)}
                    placeholder={t("settings.selectModel")}
                    className="h-8 text-xs"
                  />
                </div>
                <ScrollableContainer className="max-h-[200px]">
                  <div className="p-1">
                    {filteredModels.length === 0 ? (
                      <p className="text-xs text-muted-foreground text-center py-2">{t("settings.noModelsFound")}</p>
                    ) : (
                      filteredModels.map((model) => (
                        <button
                          key={model.id}
                          type="button"
                          className="w-full text-left px-2 py-1.5 text-sm rounded-sm hover:bg-accent hover:text-accent-foreground flex items-center justify-between"
                          onClick={() => handleSelectModel(model)}
                        >
                          <span className="truncate">{model.id}</span>
                          {model.id === formModel && <Check className="h-3.5 w-3.5 shrink-0 ml-2" />}
                        </button>
                      ))
                    )}
                  </div>
                </ScrollableContainer>
              </PopoverContent>
            )}
          </Popover>
          <Button
            variant="outline"
            size="sm"
            onClick={handleFetchModels}
            disabled={fetchingModels || !formApiKey}
            className="shrink-0"
          >
            {fetchingModels ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            <span className="ml-1">{fetchingModels ? t("settings.fetchingModels") : t("settings.fetchModels")}</span>
          </Button>
        </div>
      </div>

      <div className="grid gap-2">
        <Label>{t("settings.apiBase")}</Label>
        <Input
          value={formApiBase}
          onChange={(e) => setFormApiBase(e.target.value)}
          placeholder={getDefaultApiBase(formType)}
        />
        <p className="text-xs text-muted-foreground">{t("settings.defaultApiBase")}</p>
      </div>

      <div className="grid gap-2">
        <Label>{t("settings.apiKey")}</Label>
        <Input
          type="password"
          value={formApiKey}
          onChange={(e) => setFormApiKey(e.target.value)}
          placeholder={isEditing && initialValues?.maskedApiKey ? initialValues.maskedApiKey : "sk-..."}
        />
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="grid gap-2">
          <Label>{t("settings.maxOutputTokens")}</Label>
          <Input
            type="number"
            min={0}
            value={formMaxOutputTokens}
            onChange={(e) => setFormMaxOutputTokens(parseInt(e.target.value) || 0)}
          />
          <p className="text-xs text-muted-foreground">{t("settings.maxOutputTokensHint")}</p>
        </div>
        <div className="grid gap-2">
          <Label>{t("settings.contextWindow")}</Label>
          <Input
            type="number"
            min={0}
            value={formContextWindow}
            onChange={(e) => setFormContextWindow(parseInt(e.target.value) || 0)}
          />
          <p className="text-xs text-muted-foreground">{t("settings.contextWindowHint")}</p>
        </div>
      </div>

      {isOpenAIProvider && (
        <div className="rounded-lg border p-4 space-y-2">
          <Label>{t("settings.reasoningEffort")}</Label>
          <Select
            value={formReasoningEffort}
            onValueChange={(value) => setFormReasoningEffort(value as "none" | "low" | "medium" | "high" | "xhigh")}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{t("settings.reasoningEffortNone")}</SelectItem>
              <SelectItem value="low">{t("settings.reasoningEffortLow")}</SelectItem>
              <SelectItem value="medium">{t("settings.reasoningEffortMedium")}</SelectItem>
              <SelectItem value="high">{t("settings.reasoningEffortHigh")}</SelectItem>
              <SelectItem value="xhigh">{t("settings.reasoningEffortXHigh")}</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {currentModelSupportsReasoning || !formModel
              ? t("settings.reasoningEffortHint")
              : t("settings.reasoningEffortUnsupportedHint")}
          </p>
        </div>
      )}

      <Button
        onClick={handleSubmit}
        disabled={saving || (!formApiBase.trim() && !formApiKey.trim())}
        className="w-full"
      >
        {saving ? <Loader2 className="h-4 w-4 mr-1.5 animate-spin" /> : submitIcon}
        {submitLabel ?? t("action.save")}
      </Button>
    </div>
  );
}

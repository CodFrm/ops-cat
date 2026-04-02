import { useState, useEffect, useCallback } from "react";
import { useLanguage } from "./useLanguage";

/**
 * Loads extension locale files and returns a translation function.
 * Uses the language from LanguageContext, then tries base language, then falls back to "en".
 */
export function useLocale(extensionName: string | undefined) {
  const { language } = useLanguage();
  const [translations, setTranslations] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!extensionName) return;

    const candidates = [language];
    if (language.includes("-")) {
      candidates.push(language.split("-")[0]);
    }
    if (!candidates.includes("en")) {
      candidates.push("en");
    }

    (async () => {
      for (const candidate of candidates) {
        try {
          const res = await fetch(`/extensions/${extensionName}/locales/${candidate}.json`);
          if (res.ok) {
            setTranslations(await res.json());
            return;
          }
        } catch {
          // continue to next candidate
        }
      }
    })();
  }, [extensionName, language]);

  const t = useCallback(
    (key: string | undefined): string => {
      if (!key) return "";
      return translations[key] ?? key;
    },
    [translations]
  );

  return { t, translations };
}

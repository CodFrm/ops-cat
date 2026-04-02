// frontend/src/extension/i18n.ts
import i18n from "../i18n";

export async function loadExtensionLocales(name: string): Promise<void> {
  const lang = i18n.language;
  try {
    const res = await fetch(`/extensions/${name}/locales/${lang}.json`);
    if (res.ok) {
      const translations = await res.json();
      i18n.addResourceBundle(lang, `ext-${name}`, translations);
    }
  } catch {
    // Extension may not have locales — silently continue
  }
}

import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import zhCommon from "./locales/zh-CN/common.json";
import enCommon from "./locales/en/common.json";

const resources = {
  "zh-CN": { common: zhCommon },
  en: { common: enCommon },
};

function detectLanguage(): string {
  const saved = localStorage.getItem("language");
  if (saved) return saved;
  return (navigator.language || "").toLowerCase().startsWith("zh") ? "zh-CN" : "en";
}

i18n.use(initReactI18next).init({
  resources,
  lng: detectLanguage(),
  fallbackLng: "en",
  defaultNS: "common",
  interpolation: {
    escapeValue: false,
  },
});

export default i18n;

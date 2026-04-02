import { createContext, useContext, useState, useCallback } from "react";

type LanguageContextValue = {
  language: string;
  setLanguage: (lng: string) => void;
};

export const LanguageContext = createContext<LanguageContextValue>({
  language: localStorage.getItem("devserver_language") || navigator.language || "zh-CN",
  setLanguage: () => {},
});

export function useLanguage() {
  return useContext(LanguageContext);
}

export function useLanguageState() {
  const [language, setLang] = useState(
    () => localStorage.getItem("devserver_language") || navigator.language || "zh-CN"
  );

  const setLanguage = useCallback((lng: string) => {
    setLang(lng);
    localStorage.setItem("devserver_language", lng);
  }, []);

  return { language, setLanguage };
}

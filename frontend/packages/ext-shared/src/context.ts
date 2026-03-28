import { createContext, useContext } from "react";
import type { ExtensionAPI } from "./api";

const ExtensionAPIContext = createContext<ExtensionAPI | null>(null);

export const ExtensionAPIProvider = ExtensionAPIContext.Provider;

export function useExtensionAPI(): ExtensionAPI {
  const api = useContext(ExtensionAPIContext);
  if (!api) {
    throw new Error(
      "useExtensionAPI must be used within an ExtensionAPIProvider"
    );
  }
  return api;
}

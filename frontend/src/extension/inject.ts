// frontend/src/extension/inject.ts
import React from "react";
import ReactDOM from "react-dom/client";
import i18n from "../i18n";
import * as ui from "@opskat/ui";
import type { ExtAPI } from "./types";

declare global {
  interface Window {
    __OPSKAT_EXT__?: {
      React: typeof React;
      ReactDOM: typeof ReactDOM;
      i18n: typeof i18n;
      ui: typeof ui;
      api: ExtAPI;
    };
  }
}

export function injectExtensionAPI(api: ExtAPI): void {
  window.__OPSKAT_EXT__ = { React, ReactDOM, i18n, ui, api };
}

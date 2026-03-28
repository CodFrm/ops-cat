import React from "react";
import ReactDOM from "react-dom/client";
import * as jsxRuntime from "react/jsx-runtime";
import "./i18n";
import "./styles/globals.css";
import App from "./App";
import { ExtensionAPIProvider } from "@opskat/ext-shared";
import { WailsExtensionAPI } from "./lib/wailsExtensionAPI";

const extensionAPI = new WailsExtensionAPI();

// 暴露共享依赖给扩展前端
// eslint-disable-next-line @typescript-eslint/no-explicit-any
(window as any).__OPSKAT_EXT__ = {
  React,
  ReactDOM,
  jsxRuntime,
  api: {
    callTool: (name: string, args: Record<string, unknown>) =>
      extensionAPI.callTool(name, args),
  },
};

const container = document.getElementById("root");
const root = ReactDOM.createRoot(container!);

root.render(
  <React.StrictMode>
    <ExtensionAPIProvider value={extensionAPI}>
      <App />
    </ExtensionAPIProvider>
  </React.StrictMode>
);

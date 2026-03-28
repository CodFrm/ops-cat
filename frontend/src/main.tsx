import React from "react";
import ReactDOM from "react-dom/client";
import * as jsxRuntime from "react/jsx-runtime";
import "./i18n";
import "./styles/globals.css";
import App from "./App";

// 暴露共享依赖给扩展前端
(window as any).__OPSKAT_EXT__ = {
  React,
  ReactDOM,
  jsxRuntime,
  api: {
    callTool: async (name: string, args: any) => {
      try {
        const { CallExtensionTool } = await import("../wailsjs/go/app/App");
        const result = await CallExtensionTool(name, JSON.stringify(args));
        return JSON.parse(result);
      } catch {
        throw new Error("CallExtensionTool binding not available");
      }
    },
  },
};

const container = document.getElementById("root");
const root = ReactDOM.createRoot(container!);

root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

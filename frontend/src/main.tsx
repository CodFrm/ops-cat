import React from "react";
import { createRoot } from "react-dom/client";
import "./i18n";
import "./lib/monaco-setup";
import "./styles/globals.css";
import App from "./App";

// 立即渲染，不阻塞首屏；数据加载在 App 内部异步进行
const container = document.getElementById("root");
createRoot(container!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

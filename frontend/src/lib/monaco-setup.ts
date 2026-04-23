// Monaco editor + Vite worker 装配。
// 必须在任何 monaco 实例创建之前 import 一次（main.tsx 内）。

import * as monaco from "monaco-editor";
import editorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import jsonWorker from "monaco-editor/esm/vs/language/json/json.worker?worker";
import tsWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker";
import { loader } from "@monaco-editor/react";
import { registerCompletions } from "./monaco-completions";

// 用本地的 monaco，避免 @monaco-editor/react 默认走 CDN（在 Wails 环境会断网失败）
self.MonacoEnvironment = {
  getWorker(_workerId, label) {
    if (label === "json") return new jsonWorker();
    if (label === "javascript" || label === "typescript") return new tsWorker();
    return new editorWorker();
  },
};

// 自定义主题：把 editor 自身的背景设为透明，让外层容器 bg-background 决定底色，
// 这样 monaco 颜色能跟项目的浅色/深色主题（包括 oklch 自定义色）保持一致。
const TRANSPARENT = "#00000000";
monaco.editor.defineTheme("opskat-light", {
  base: "vs",
  inherit: true,
  rules: [],
  colors: {
    "editor.background": TRANSPARENT,
    "editorGutter.background": TRANSPARENT,
    "editor.lineHighlightBackground": "#00000010",
    "editor.lineHighlightBorder": TRANSPARENT,
    "editorLineNumber.foreground": "#9ca3af",
    "editorLineNumber.activeForeground": "#374151",
  },
});
monaco.editor.defineTheme("opskat-dark", {
  base: "vs-dark",
  inherit: true,
  rules: [],
  colors: {
    "editor.background": TRANSPARENT,
    "editorGutter.background": TRANSPARENT,
    "editor.lineHighlightBackground": "#ffffff10",
    "editor.lineHighlightBorder": TRANSPARENT,
    "editorLineNumber.foreground": "#6b7280",
    "editorLineNumber.activeForeground": "#d1d5db",
  },
});

loader.config({ monaco });

// 注册 SQL / JS(MongoDB) 的关键字、函数、snippet、operator 补全
registerCompletions();

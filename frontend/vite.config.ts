/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  // dev: 预热首屏关键路径，避免 Vite 按需 transform 在窗口出现后串行排队
  server: {
    warmup: {
      clientFiles: [
        "./src/main.tsx",
        "./src/App.tsx",
        "./src/i18n/index.ts",
        "./src/components/layout/Sidebar.tsx",
        "./src/components/layout/AssetTree.tsx",
        "./src/components/layout/MainPanel.tsx",
        "./src/components/layout/TopBar.tsx",
        "./src/components/ai/SideAssistantPanel.tsx",
      ],
    },
  },
  // dev: 预声明重依赖，让 Vite server 启动时一次性 pre-bundle，
  // 避免首次浏览器请求触发"new dependencies optimized"导致整页重载
  optimizeDeps: {
    include: [
      "react",
      "react-dom",
      "react-dom/client",
      "react-i18next",
      "i18next",
      "sonner",
      "@iconify/react",
      "@floating-ui/dom",
      "@radix-ui/react-tooltip",
      "@radix-ui/react-dialog",
      "@radix-ui/react-popover",
      "@radix-ui/react-select",
      "@radix-ui/react-dropdown-menu",
      "@radix-ui/react-scroll-area",
      "tailwind-merge",
      "clsx",
      "zustand",
    ],
  },
  build: {
    // 改用 terser：esbuild 的死代码消除存在一个 bug——把 xterm.js 里
    // `requestMode` 中 `let r; (IIFE)(r||={});` 错误地改成 `(IIFE)(void 0||(n={}))`，
    // `n` 未声明，导致运行时 `ReferenceError: Can't find variable: n`，
    // vim 等通过 DECRQM 查询终端能力的程序会让 xterm.js parser 崩溃，
    // 从而屏幕渲染停滞、键盘看上去"无响应"。
    minify: "terser",
  },
  test: {
    environment: "happy-dom",
    setupFiles: ["./src/__tests__/setup.ts"],
  },
});

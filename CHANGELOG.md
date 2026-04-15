<a name="1.1.2"></a>

## 1.1.2 (2026-04-15)

本次版本引入全新的 **扩展系统 (Extension System)**，支持通过 WASM 模块加载第三方扩展。同时完善了 SSH 密钥 passphrase 的全流程支持，并修复了若干影响使用体验的关键问题。

### 🚀 主要新功能

- ✨ 扩展系统 (Extension System)：支持加载 WASM 扩展模块，通过 manifest 声明工具并接入 AI 助手 ([#11](https://github.com/opskat/opskat/pull/11)) (by @CodFrm)
- ✨ 完善 SSH 密钥 passphrase 全流程支持 ([#14](https://github.com/opskat/opskat/pull/14)) (by @yqdaddy)

### 🐛 Bug 修复

- 🐛 修复数据库直连返回 typed-nil 接口导致的 panic
- 🐛 修复 AI Provider 对话框内模型下拉列表无法滚动问题
- 🐛 修复 SSH 终端 vim 等全屏程序无响应问题

### ♻️ 重构

- ♻️ 内置权限组改用字符串 ID 并支持 i18n

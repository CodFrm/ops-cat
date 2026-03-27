<p align="right">
<a href="./README.md">English</a> | <a href="./README_zh.md">中文</a>
</p>

<h1 align="center">
<img src="build/appicon.png" width="128" height="128"/><br/>
OpsKat
</h1>

<p align="center">AI 优先的桌面运维工具。描述你的需求，AI Agent 代你执行，每一步都有策略管控和完整审计日志。</p>

<p align="center">
<a href="https://opskat.github.io/">官网</a> ·
<a href="https://opskat.github.io/docs/getting-started/installation">文档</a> ·
<a href="https://github.com/opskat/opskat/releases">下载</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  &nbsp;
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=for-the-badge&logo=react&logoColor=white" alt="React">
  &nbsp;
  <img src="https://img.shields.io/badge/Wails-v2-EB4034?style=for-the-badge&logo=wails&logoColor=white" alt="Wails">
  &nbsp;
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=for-the-badge&logo=windows&logoColor=white" alt="Platform">
</p>

<p align="center">
  <img src="docs/images/screenshot-main.png" alt="OpsKat 截图">
</p>

## 关于

OpsKat 是一个 **AI 优先** 的桌面运维工具。无需在菜单和表单之间跳转，直接描述你的需求 — AI Agent 会代你执行命令、查询和文件传输，每一步都有策略管控和完整审计日志。

同时提供独立命令行工具 `opsctl`，共享相同核心，支持无 GUI 的脚本化操作。

**如果觉得好用，请给我们一个 Star ⭐ 这是对我们最大的支持！**

## 演示

https://github.com/user-attachments/assets/06627133-373b-4e53-8d76-ebe7fce0866e

## ✨ 核心特性

### 🤖 AI Agent

多轮对话 + 工具调用，支持 OpenAI 兼容 API、Claude CLI、Codex CLI。Agent 可以管理资产、执行命令、查询数据库、传输文件等，所有操作都经过策略管控和审计。

### 🖥️ 资产管理

以树形结构组织基础设施。目前支持 SSH 服务器、MySQL/PostgreSQL 数据库和 Redis，未来将支持更多资产类型。凭据加密存储，集成系统密钥链。支持从 SSH config、Tabby 导入，导出到文件或 GitHub Gist。

### 🔌 SSH 终端

交互式终端，支持分屏、自定义主题、SFTP 文件浏览器、跳板机链式连接、连接池、端口转发和 SOCKS 代理。

### 🗄️ 查询编辑器

SQL 编辑器 + 结果表格（MySQL/PostgreSQL 可通过 SSH 隧道），Redis 命令执行与 Key 浏览器，基于 TiDB Parser 的 SQL 分析。

### 🛡️ 策略管控

SSH 命令、SQL 语句、Redis 操作的允许/拒绝规则。策略组系统：内置模板 + 自定义策略组。

### 📋 审计与审批

每个操作都记录决策信息。

### 🌐 国际化

支持英文和简体中文。

## ⌨️ opsctl CLI

独立命令行工具，与桌面端共享相同核心，无需 GUI 即可脚本化操作。支持从桌面端一键安装。

```bash
opsctl exec <asset> -- <command>    # 执行远程命令
opsctl ssh <asset>                  # 交互式 SSH 会话
opsctl cp <src> <dst>               # 文件传输（本地/远程/跨服务器）
opsctl sql <asset> "<query>"        # 执行 SQL 查询
opsctl redis <asset> "<command>"    # 执行 Redis 命令
opsctl list assets|groups           # 列出资产或分组
opsctl grant submit ...             # 预审批命令模式
```

桌面端运行时，opsctl 会复用其连接池，并通过桌面端 UI 进行审批。

## 🧩 AI 编程工具集成

OpsKat 内置了 AI 编程 CLI 集成 — **Claude Code** 和 **Codex**。从桌面端一键安装 Skill，让 AI 编程助手学会使用 `opsctl`，直接管理服务器、执行命令、传输文件和查询数据库。

<p align="center">
  <img src="docs/images/screenshot-skill.png" alt="Skill 安装">
</p>

## 🛠️ 技术栈

| | |
|---------|------------|
| 桌面端 | [Wails v2](https://wails.io/) (Go + Web) |
| 前端 | React 19 + TypeScript + Tailwind CSS |
| 后端 | Go 1.25、SQLite |

## 🚀 快速开始

**前置依赖：** [Go 1.25+](https://go.dev/)、[Node.js 22+](https://nodejs.org/) + [pnpm](https://pnpm.io/)、[Wails v2 CLI](https://wails.io/docs/gettingstarted/installation)

```bash
make install        # 安装前端依赖
make dev            # 开发模式（热重载）
make build          # 生产构建
make build-embed    # 生产构建（内嵌 opsctl）
make build-cli      # 仅构建 opsctl CLI
```

---

## 🤝 参与贡献

我们欢迎所有形式的贡献！查看 Issues 或提交 Pull Request。

---

## 📄 开源许可

本项目基于 [GPLv3](./LICENSE) 协议开源。

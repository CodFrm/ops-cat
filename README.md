<p align="right">
<a href="./README.md">English</a> | <a href="./README_zh.md">中文</a>
</p>

<h1 align="center">
<img src="build/appicon.png" width="128" height="128"/><br/>
OpsKat
</h1>

<p align="center">An open-source, AI-first desktop application for managing remote infrastructure. Describe what you need — the AI agent handles the rest, with policy enforcement and full audit logging.</p>

<p align="center">
<a href="https://opskat.dev/">Website</a> ·
<a href="https://opskat.dev/docs/getting-started/installation">Docs</a> ·
<a href="https://github.com/opskat/opskat/releases">Download</a>
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
  <img src="docs/images/screenshot-main.png" alt="OpsKat Screenshot">
</p>

## About

Managing servers often means juggling multiple tools — SSH clients, database GUIs, Redis managers — constantly switching between them. OpsKat brings everything together in one place. With its AI agent, you can describe what you need in natural language, and it handles the rest. Even without the AI features, all common asset operations are unified in a single interface.

Currently supports SSH servers, MySQL/PostgreSQL databases, and Redis, with more asset types planned via a plugin system.

**If you find it useful, please give us a Star ⭐ — it means a lot!**

## Demo

https://github.com/user-attachments/assets/035fc0df-230c-456b-87bd-8a4a125feaec

## ✨ Use Cases

- **"Show me the recent nginx error logs on web-01"** → AI automatically SSHs in, runs the command, and returns the results
- **"Count users by status in the db-prod users table"** → AI connects to the database via SSH tunnel and executes the SQL query
- **"Check the health of the k3s cluster"** → AI runs kubectl commands and summarizes node and pod status

## 🛡️ Security & Audit

Giving AI permission to operate on your servers — how do you keep it safe?

- **Operation policies** — SSH commands, SQL statements, and Redis operations all support allow/deny lists. SQL is analyzed by a parser that automatically blocks dangerous operations like DELETE/UPDATE without WHERE clauses
- **Policy groups** — Built-in templates (Linux read-only, dangerous command deny, etc.) plus custom user-defined groups
- **Pre-approved permissions** — AI or opsctl can request a batch of command patterns upfront. Once approved, matching commands execute automatically without per-command confirmation
- **Audit logs** — Every operation is automatically recorded: who, when, which server, what command, and the full decision trail

## 🖥️ Also a Full-Featured Terminal & Asset Manager

Beyond the AI, OpsKat is a complete terminal and asset management tool:

- Tree-structured grouping for SSH servers, databases, and Redis
- Split pane terminal with customizable themes
- SFTP file browser
- Jump host chain connections
- SQL query editor (MySQL/PostgreSQL via SSH tunnel)
- Redis command execution with key browser
- Port forwarding and SOCKS proxy
- Encrypted credential storage
- Import from SSH config / Tabby

## ⌨️ opsctl CLI + AI Coding Tool Integration

OpsKat ships a standalone CLI tool (`opsctl`), primarily designed for AI coding assistants like **Claude Code**, **Codex**, and **Gemini CLI**. One-click skill installation from the desktop app teaches these AI assistants to use `opsctl` — so they can directly manage servers, check logs, query databases, and troubleshoot production issues.

When the desktop app is running, opsctl reuses its connection pool and approval workflow, with all operations subject to the same policy enforcement and audit logging.

You can also use it manually:

```bash
opsctl exec web-01 -- tail -n 100 /var/log/nginx/error.log
opsctl sql db-prod "SELECT status, COUNT(*) FROM users GROUP BY status"
opsctl ssh web-01
```

## 🛠️ Tech Stack

| | |
|---------|------------|
| Desktop | [Wails v2](https://wails.io/) (Go + Web) |
| Frontend | React 19 + TypeScript + Tailwind CSS |
| Backend | Go 1.25, SQLite |

## 🚀 Getting Started

**Prerequisites:** [Go 1.25+](https://go.dev/), [Node.js 22+](https://nodejs.org/) with [pnpm](https://pnpm.io/), [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation)

```bash
make install        # Install frontend dependencies
make dev            # Development mode (hot reload)
make build          # Production build
make build-embed    # Production build with embedded opsctl
make build-cli      # Build opsctl CLI only
```

---

## 🤝 Contributing

We welcome all forms of contribution! Check out the issues or submit a pull request.

---

## 📄 License

This project is open-sourced under the [GPLv3](./LICENSE) license.

## 🔗 Links

- [LINUX DO](https://linux.do/)

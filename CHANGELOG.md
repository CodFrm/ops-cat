<a name="1.4.1"></a>

## 1.4.1 (2026-04-28)

本次版本对资产树和 SSH 终端做了体验优化，并修复非中英文系统下 i18n 兜底语言的问题。

### ✨ 新功能

- ✨ 资产树支持隐藏空文件夹 (by @CodFrm)
- ✨ SSH 终端回滚缓冲区可配置，默认调整为 25000 行 (by @CodFrm)

### 🐛 Bug 修复

- 🐛 修复非中英文系统下 i18n 兜底为中文的问题 (by @CodFrm)

<a name="1.4.0"></a>

## 1.4.0 (2026-04-27)

本次版本带来代码片段（Snippets）复用系统与 WebDAV 备份提供方，AI 助手扩展为多会话标签栏并支持会话重命名、历史消息编辑重发与工具卡片展开；首页分区与设置页完成整合，资产树新增类型筛选，Redis 面板补齐 Stream 类型展示；同时修复了终端分屏内容丢失、Ctrl+A 全选、DeepSeek thinking 模式 400 等多项问题。

### 🚀 主要新功能

- 💥 代码片段（Snippets）复用系统 (by @CodFrm)
- 💥 新增 WebDAV 备份提供方 ([#47](https://github.com/opskat/opskat/pull/47)) (by @Pililink)
- 💥 侧边 AI 助手多会话标签与右侧会话栏 ([#35](https://github.com/opskat/opskat/pull/35)) (by @2849236173)
- ✨ 新增 AI 会话重命名功能 ([#38](https://github.com/opskat/opskat/pull/38)) (by @2849236173)
- ✨ 支持 AI 对话编辑历史消息后重发 ([#30](https://github.com/opskat/opskat/pull/30)) (by @2849236173)
- ✨ AI 工具卡片可展开查看调用参数 (by @CodFrm)
- ✨ AI 资产工具补齐密码/私钥/分组管理并触发左侧树刷新 (by @CodFrm)
- ✨ 实现 Redis Stream 类型数据展示 ([#36](https://github.com/opskat/opskat/pull/36)) (by @shanaiardor)
- ✨ 资产树类型筛选 + 移除 Sidebar 分区按钮 ([#51](https://github.com/opskat/opskat/pull/51)) (by @CodFrm)
- ✨ 首页分区与设置页整合，修复列表交互状态 ([#37](https://github.com/opskat/opskat/pull/37)) (by @tangqiu0205)
- ✨ 设置页新增 Bug 反馈、Debug 日志开关与打开日志目录 (by @CodFrm)
- ✨ 设置页显示仓库地址 ([#45](https://github.com/opskat/opskat/pull/45)) (by @Pililink)
- ✨ 扩展框架支持通用 TCP IO、deadline、action 取消与 textarea 格式化 ([#31](https://github.com/opskat/opskat/pull/31)) (by @CodFrm)

### 🐛 Bug 修复

- 🐛 修复 SSH 分屏后已存在终端内容被清空 (by @CodFrm)
- 🐛 修复 Ctrl+A 全选导致整页文本被选中 [#48](https://github.com/opskat/opskat/issues/48) (by @CodFrm)
- 🐛 修复终端 Ctrl+F 硬编码导致用户改绑无效 ([#32](https://github.com/opskat/opskat/pull/32)) (by @CodFrm)
- 🐛 修复 DeepSeek thinking 模式下多轮对话报 400 错误的问题 ([#42](https://github.com/opskat/opskat/pull/42)) (by @shanaiardor)
- 🐛 修复 Windows 环境下 OpenDirectory 因隐藏界面而无法正常显示 explorer 的问题 ([#41](https://github.com/opskat/opskat/pull/41)) (by @shanaiardor)
- 🐛 修复 GitHub Releases 手动安装链接 ([#50](https://github.com/opskat/opskat/pull/50)) (by @Pililink)

<a name="1.3.0"></a>

## 1.3.0 (2026-04-23)

本次版本带来侧边 AI 助手面板与全新 Sidebar 布局，数据库面板补齐建库/建表/设计表全流程并接入 Monaco 编辑器，AI 对话支持 @ 提及资产与 Token 用量展示，同时大幅优化查询面板性能，修复了 AI 助手、SSH/SOCKS 代理、终端等大量稳定性问题。

### 🚀 主要新功能

- ✨ 侧边 AI 助手面板：aiStore 重构 + 常驻 SideAssistantPanel ([#18](https://github.com/opskat/opskat/pull/18)) (by @CodFrm)
- ✨ 侧边 Tab 布局：ActivityBar → Sidebar 合并 + 左右布局切换 ([#17](https://github.com/opskat/opskat/pull/17)) (by @CodFrm)
- ✨ 数据库面板补齐建库/建表/设计表流程并统一 SQL 预览确认 ([#27](https://github.com/opskat/opskat/pull/27)) (by @tangqiu0205)
- ✨ 数据库面板接入 Monaco 编辑器并优化查询体验 (by @CodFrm)
- ✨ AI 对话 @ 提及资产 + 统一资产搜索（支持拼音） ([#22](https://github.com/opskat/opskat/pull/22)) (by @CodFrm)
- ✨ AI 对话框展示 Token 用量 + 复制优化 (by @CodFrm)
- ✨ MongoDB 结果面板向 database 对齐：复用 QueryResultTable + FILTER/SORT 查询栏 (by @CodFrm)
- ✨ 资产分组折叠状态持久化 (by @CodFrm)

### ⚡️ 性能优化

- ⚡️ 查询面板编辑/拖拽/渲染链路重构，消除键入与拖拽卡顿 (by @CodFrm)

### 🐛 Bug 修复

- 🐛 修复 AI 助手 run_command 卡死与会话丢失问题 ([#20](https://github.com/opskat/opskat/pull/20)) (by @2849236173)
- 🐛 修复 AI 助手复制与输入历史交互 ([#25](https://github.com/opskat/opskat/pull/25)) (by @2849236173)
- 🐛 修复 AI 助手侧边历史下拉无法滚动且删除无效 (by @CodFrm)
- 🐛 修复切换 AI 供应商后仍使用旧 provider 的问题 (by @CodFrm)
- 🐛 修复关闭软件时丢失 AI 对话在途内容 (by @CodFrm)
- 🐛 修复 AI 停止会话在 SFTP 文件传输时卡死的问题 (by @CodFrm)
- 🐛 统一 SSH dial 路径，修复 AI 命令忽略 SOCKS5 代理 (by @CodFrm)
- 🐛 移除 SOCKS4 / HTTP 代理类型残留 (by @CodFrm)
- 🐛 修复 SSH 资产从跳板机切回直连后保存不生效 (by @CodFrm)
- 🐛 修复 PostgreSQL 表格内联编辑生成 UPDATE 时 WHERE 列出所有列 (by @CodFrm)
- 🐛 修复 SSH 终端右键菜单关闭后失去焦点 (by @CodFrm)
- 🐛 修复 IME 合成中 Enter 误触发问题 (by @CodFrm)
- 🐛 修复页面切换时文字重叠残影 ([#26](https://github.com/opskat/opskat/pull/26)) (by @tangqiu0205)
- 🐛 修复 Tab 过滤弹窗被 DropdownMenu 卸载误关 / 退出动画 focus-outside 问题 (by @CodFrm)

### 🎨 UI 改进

- 🎨 Tab 过滤入口文案改为"查找标签页"并修复 DatabasePanel 格式 (by @CodFrm)

<a name="1.2.0"></a>

## 1.2.0 (2026-04-16)

本次版本新增 MongoDB 完整集成支持，优化了终端和查询面板的交互体验。

### 🚀 主要新功能

- ✨ MongoDB 集成：完整的 MongoDB 资产管理与查询功能 ([#15](https://github.com/opskat/opskat/pull/15)) (by @CodFrm)
- ✨ SSH 终端复制提示

### 🎨 UI 改进

- ✨ 优化 tab 栏：等宽压缩自适应 + 颜色指示条
- ✨ SQL 查询分页、结果表列宽调整与终端快捷键提示

# OneBot 页功能对照

更新：2026-11-03。React 来源：`apps/desktop/frontend/src/features/onebot.tsx` 及其引用的 `features/cindy/bots/**`、`features/cindy/host/onebot*`（Cindy b7e46bc，Apache-2.0）。MyGo 实现：`apps/desktop-mygo/pages/onebot`（原生页框架）+ `apps/desktop-mygo/webui`（网页视图与桥）。

用户 2026-11-01 选择混合方案：OneBot 管理页在 MyGo 网页视图中运行现有 React/Cindy 组件。本页因此整体运行 `chat-embed.html?view=onebot`，React 页面与 Rust 窗口中的完全相同。

状态：**已验证** = 有单元测试或实窗探针；**待验证** = 已实现、还缺证据。

## 与用户决定的差异（待确认）

队友对话（`CCAgentSessionView`）在 React 中是 Chat 页加 Bot 模式（时间线、引用、语音朗读、@提及、已读边界）。混合方案原计划让它复用原生 Chat；目前它随 OneBot 页一起在网页视图中运行 React Chat。功能不缺，但这部分不是原生界面。改为原生需要先把 Bot 模式移植到原生 Chat（见 [Chat 对照](chat.md) M7），再把原生 Chat 叠放到 OneBot 页面指定位置。

## 对照

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| O1 | 侧栏 OneBot 入口（Panda 图标，位于 Chat 之后） | `shell/pages.go` 的 `onebot`、`pages/onebot` | 代码审查 | 已实现 |
| O2 | 首次打开读取 AI 服务、Chat 设置、Chat 目录与语音数据；失败可重试 | React `OneBotPage` 原样运行 | 实窗探针：无 AI 服务时显示“先连接一个 AI 服务” | 已验证 |
| O3 | 伙伴列表、主页、名册、市场、私聊、群聊、会话、历史、IM、语音、记忆、例行任务 | React 路由树原样运行；Host 方法按 Rust 白名单（`webui/allow.go`，147 个 onebot 方法） | `TestTheBridgeAllowsWhatTheRustWindowAllowed` | 部分验证（真实 Host 未联调） |
| O4 | 队友对话（React Chat，OneBot 传输范围 `onebot.chat.*`），含侧面板与内置浏览器 | React `ChatPage` 在同一网页视图中；网页标签由 Go 网页视图叠放（`webui.Browser`） | 代码审查 | 待验证 |
| O5 | 打开普通 Chat 会话（`winone-open-chat`）、Chat 设置、AI 服务（`winone-open-page`）经未保存保护跳转 | `webui.Navigate`、`Env.Open`、Chat 的 `takeIntents` | `TestLinksOfPagesGoThroughTheShell`、`TestLinksFromOtherPagesOpenTheirConversationAndFile` | 已验证 |
| O6 | 修改型请求进行中时阻止关闭窗口或离开页面（Rust `Request.mutates`） | `Surface.Busy`、`chatMutates`（只读表由 `component_bridge.rs` 生成） | 代码审查 | 待验证（实窗关闭） |
| O7 | 系统挂起、锁屏时停止语音输入 | 窗口子类处理 `WM_POWERBROADCAST`/`WM_WTSSESSION_CHANGE`，向所有网页视图发出 `winone:voice-system-suspend`/`winone:voice-screen-locked` | `TestSuspendAndLockStopVoiceInput` | 部分验证（真实挂起未测） |
| O8 | 麦克风权限 | MyGo 默认允许自有页面；面板中外部网页默认不允许 | 代码审查 | 待验证（真实麦克风） |
| O9 | 队友对话导出 `.cshare` | `webui.ExportTask`（与原生 Chat 共用；只认窗口保存对话框选择的路径与匹配回执） | `TestExportsSaveOnlyWhereTheWindowChose` | 已验证 |

## 未验证范围

- 真实 Host 与真实账号下的伙伴创建、IM 连接、语音、群聊、例行任务未联调。
- 实窗只验证了首屏（私有桌面探针 `tests/desktop-mygo-webui/run-private.ps1`）。

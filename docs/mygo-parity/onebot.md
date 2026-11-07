# OneBot 页功能对照

更新：2026-11-05。React 来源：`apps/desktop/frontend/src/features/onebot.tsx` 及 `features/cindy/bots/**`（Cindy 2d8d3c3，Apache-2.0）。MyGo 实现：`apps/desktop-mygo/pages/onebot` 与 `pages/chat` 的 Bot 模式。OneBot 不再创建 React 视图，生产包也不含 React；只有 HTML 文件预览使用隔离的网页视图。

状态：**已测** = 有 `ui.Tester` 或单元测试；**Host 已测** = 真实 Host→OneBot→Pi 回路（模拟模型）；**实窗** = 隔离实窗证据，结果见 [完成度核查](chat-onebot-audit.md)。

| 编号 | 能力 | MyGo 实现 | 证据 |
| --- | --- | --- | --- |
| O1 | 侧栏入口、伙伴列表、名册、搜索、置顶、生命周期菜单 | `view.go`、`onebot.go` | 已测、实窗 |
| O2 | 首屏读取、失败重试、未配置模型提示 | `onebot.go` | 已测、实窗 |
| O3 | 创建（草稿、头像、肖像）、市场、导入 Hermes/OpenClaw | `creation.go`、`portrait.go`、`market_import.go` | Host 已测；真实账号未测 |
| O4 | 设置：资料、模型链、能力、技能、记忆、项目、历史、例行、夜间学习、待办、渠道、语音、麦克风、生命周期 | `settings.go` 及同目录各页 | Host 已测 |
| O5 | 伙伴对话复用原生 Chat 控制器 | `chat.NewOneBotConversation`、`bot_binding.go` | 已测 |
| O6 | 时间组、未读线、已读推进（只由渲染尾部推进） | `pages/chat/bot_mode.go` | 已测 |
| O7 | 引用回复、回复对应的消息（全局开关在默认模型设置） | `bot_mode.go`、`settings.go` | 已测 |
| O8 | @ 提及面板与线上引用（UTF-16 范围） | `bot_mode.go` | 已测 |
| O9 | 学习回执、开场选项、邀请等待提示、占位文字 | `bot_mode.go`、`view.go` | 已测 |
| O10 | 后台任务卡、回执、补充说明、停止、PR 状态、互发消息留痕、通话记录 | `pages/chat/bot_cards.go` | 已测 |
| O11 | 私聊话题：列表、创建、面板、未读 | `bot_binding.go` | 已测 |
| O12 | 消息朗读 | `bot_binding.go` | 分块已测；真实音频未测 |
| O13 | 语音通话：内联面板、手动发送、审批时暂停、口述回答问题 | `voice_session.go` | 语法与暂停已测；真实麦克风未测 |
| O14 | 语音输入 | `dictation.go` | Host 已测；真实麦克风未测 |
| O15 | 系统挂起或锁屏停止听写、朗读，通话暂停并保留草稿 | `core/voice_pause.go`、`onebot.go` | 已测；真实挂起未测 |
| O16 | 群聊：消息、发送、附件、分工计划、继续/停止、文件预览、成员与设置、删除 | `group.go` | Host 已测 |
| O17 | 伙伴互发消息线程（只读） | `direct.go` | 已测（链接） |
| O18 | 导出 `.cshare`、打开普通 Chat 任务、跳转设置 | `webui.ExportTask`、`Env.Open` | 已测 |

## 与 React 的差异

逐项原因见 [上游审阅](upstream-review-2d8d3c3.md)。

- 群聊中的权限与问题在说话伙伴的任务里回答，群聊显示等待提示和打开任务按钮。
- Chat Server 群、邀请、面具、多设备属于 Cindy 云端，不在 WinOne 范围。

## 未验证范围

- 真实账号：市场安装、IM 渠道、个人微信。
- 真实设备：麦克风、声音输出、真实系统挂起。

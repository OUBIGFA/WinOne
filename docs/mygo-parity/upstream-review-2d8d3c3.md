# 原生模块对 Cindy 2d8d3c3 的逐项审阅

审阅日期：2026-11-07。固定上游范围不变；下文旧“未移植”决定只用于追溯，不能作为省略本地能力的授权。当前逐能力/行为证据见 [本地插件平台](local-plugins.md) 与 [续接核查](chat-onebot-audit.md)。

原位编辑、群聊权限/问题、过程图片、完整分组/恢复投影、Windows 空格图片、富内容音视频与 MathJax 均已接原实现并取得真实行为或原生窗口证据；本地 SDK/MCP/邮件/browser bridge 不再是旧表的“无插件宿主”。全量本地功能、真实前台手势/媒体草稿ACK、typed COM/官方App与当前候选构建/安装均已通过；固定上游strict核对通过。Git/正式安装器交付与未覆盖外部条件见 [本次验收](../2026-11-07-native-local-delivery.md)，不以SHA一致替代行为验证。

## 2026-11-05 续接修订（历史当时状态）

下文“未移植”是早期缺口记录。现已有原位编辑、群聊内联权限/问题、裸空格图片路径处理、富文本音视频和 MathJax 排版实现；群聊 PromptPanel 与公式又经实测修正。**完整行为与许可验收仍未完成**，最新证据和未接通的本地插件 API 见 [续接核查](chat-onebot-audit.md)。公式字体原始许可缺失是当前交付阻塞，不能用 TeX 源码替代后称全量完成。

## 已移植

| 上游变化 | 原生位置 |
| --- | --- |
| 私聊引用回复、回复对应的消息 | `pages/chat/bot_mode.go` |
| 私聊话题的入口、预览与未读 | `bot_mode.go`、`pages/onebot/bot_binding.go` |
| 伙伴回复的头像列、时间组、未读线、@ 提及、开场选项、学习回执 | `bot_mode.go` |
| 后台任务卡、回执、补充说明留痕、PR 状态、伙伴互发消息留痕、通话记录 | `pages/chat/bot_cards.go` |
| 语音通话的阻塞暂停、口述回答问题（整句语法） | `pages/onebot/voice_session.go`、`bot_mode.go` |
| 消息朗读（`voice.startPlayback` 租约） | `pages/onebot/bot_binding.go` |
| 富文本答案 `cindy-ui`：解析、算术、组件、发送回执 | `chat/richanswer`、`pages/chat/rich_answer.go` |
| Mermaid 图（与结果窗共用本地渲染器） | `pages/chat/rich_answer.go` |
| 问题卡片的文字支持链接与文件定位 | `pages/chat/composer.go` |
| 伙伴工作过程摘要显示当前动作（`teammateWorkSummaryText` 的工具部分） | `pages/chat/tools.go`、`transcript.go` |

## 历史“未移植”记录（2026-11-05，当时状态，已由新对照取代）

| 上游变化 | 决定与原因 |
| --- | --- |
| `workRunGrouping` 的恢复边界、答案尾折叠 | 未移植。原生按 WinOne 的 `chat-process.ts` 分组，WinOne 没有 Cindy 的隐藏续跑入口，触发条件不存在 |
| `ErrorBanner` 的余额页、成员预算、远程压缩、Codex 恢复、隐藏续跑 | 不适用。依赖 Cindy 账号、远程电脑或 Codex |
| `AgentActionRow` 的 Ghost 身份、工具过程媒体、远程单行 | 不适用。没有插件宿主和远程任务 |
| `WorkGroupBlock` 的远程单行、完整 journal 分段 | 不适用或部分移植。只移植摘要文字；过程内容仍用原生折叠区 |
| `GeneratedFilesCard`、`TurnChangesCard` 的 `onActivate`、键盘激活辅助 | 未移植。这些是 Cindy 右键菜单和中键的统一入口；原生按钮已可点击和键盘操作，中键打开不提供 |
| `SendButton` 的通话/发送同一按钮 | 未移植外观。原生把通话放在工具栏独立按钮，没有“按下时动作被替换”的问题 |
| `MarkdownRenderer` 的裸空格本地图片、视频站点预览 | 未移植。含空格的本地图片路径需写成 `<路径>`；视频链接按普通链接打开 |
| `sessionActivity.getSessionListActivityAt`、`dropdown-menu` 的 portal | 不适用。WinOne 会话目录由 Host 排序；原生菜单没有 portal |
| 公式 KaTeX 渲染 | 未实现。MyGo 没有公式组件，与结果窗一致显示可选中的 TeX 源码 |

## 历史差异与限制（以本轮能力对照为当前事实）

- **群聊权限与问题**：Cindy 在群聊内直接显示提示。原生显示“在等你确认”并提供打开任务按钮，在该任务里回答。
- **富文本答案里的音视频**：Cindy 内嵌播放。原生显示按钮，在右侧面板的原生播放器打开。
- **话题面板宽度**：固定 440，可随窗口收缩；上游 2d8d3c3 之后才加入拖动调整。
- **插件前置检查**：原Ghost核心/Host/原生选择、真实调度allow/skip/block、Node/Library/OAuth连接、MCP App/mail/browser以及全原生容器组合均已按本轮完整能力对照接入验证；不把本机fixture误称真实外部账号或生产登记验收。
- **Chat Server 群、邀请、面具、多设备**：Cindy 云端能力，在 WinOne 范围外。

## 2d8d3c3 之后

本次交付固定在 2d8d3c3。2026-11-05 查询 `origin/main` 为 `fbb89b7b36`，多 145 个提交；其中影响伙伴、自动化、聊天和共享包的是 18 个文件，包括话题面板拖动、工作过程旁的头像、GitLab MR 状态、群聊定位不移动输入框。下一轮用下面的命令逐模块跟进：

```
git -C dist/resources/sources/cindy fetch origin main
node build/check-cindy-ports.mjs --cindy dist/resources/sources/cindy
```

# Chat 页功能对照

更新：2026-11-01。React 来源：`apps/desktop/frontend/src/features/chat*.ts(x)` 及其引用的 `features/cindy/**`（功能清单见下文分区）。MyGo 实现：`apps/desktop-mygo/pages/chat`（视图与操作）、`apps/desktop-mygo/chat/model`（`chat-state.ts`、`chat-process.ts` 与浏览器批注格式的 Go 移植）。

用户 2026-11-01 选择**混合方案**：会话列表、消息流、输入框、排队、审批、提问、计划等核心交互原生重写；代码编辑器、差异审阅、PDF、终端、内置浏览器与 HTML 预览、自动化整页等重型组件在 MyGo WebView 中运行现有 React/Cindy 组件。

状态：**已验证** = 有单元测试或 `ui.Tester` 渲染；**待验证** = 已实现、还缺证据；**未开始** = 尚未实现。单元测试在 `pages/chat/chat_test.go` 与 `chat/model/state_test.go`；渲染图在本地 `_temp/mygo-chat/shots/`（不入库，深浅色 × 中英 × 1100/640 宽）。

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| 编辑最后一条消息用对话框，不在气泡里原位编辑 | 原位编辑需要测量原气泡宽度；对话框保留同样的停止、文件恢复选项和失败提示，行为一致。 |
| 窄输入框里权限和思考强度只显示图标，名称在提示里 | 640 宽以下工具一行放不下，避免逐行堆叠。 |
| 完成提示音由 Go 合成同样的双音（C5、E5，0.18 秒间隔，0.45 秒衰减），用系统播放 | 原生窗口没有 WebAudio。 |
| 视图偏好（上次模型、提示音、折叠的项目与分组、面板尺寸）存于配置目录 `desktop-chat-view.json` | React 存在 WebView 的 localStorage。 |
| 导出任务的保存对话框由 MyGo 窗口弹出，Host 回执必须指向所选文件 | 与 Rust 桥 `chat_export.rs` 的约束相同，只是位置从 Rust 移到 Go。 |

## 会话运行（核心）

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| R1 | 事件归并 `applyChatEvent`、`upsertChatMessage`、回合投影 `projectChatProcess` | `chat/model` | 移植的上游用例 15 项 | 已验证 |
| R2 | 打开会话：`draft.get` 与 `open` 并行；按游标 `poll`（more 立即、运行 160ms、空闲 1.2s、出错 2s；reset 时 `state`）；离开时 `release` | `Chat.open`/`poll`/`release` | `TestOpeningAConversationShowsItsTurnsAndFollowsItsEvents`、`TestNavigationWaitsForDraftSaves` | 已验证 |
| R3 | 草稿保存串行，切换会话等待保存完成 | `serial`、`updateText`、`choose` | `TestNavigationWaitsForDraftSaves` | 已验证 |
| R4 | 发送：先清空输入框，回执 `requestId` 去重，乐观插入，失败还原；新对话首次发送时创建 | `Chat.send` | `TestSendingClearsTheDraftAndSendsOneReceipt`、`TestANewChatIsCreatedOnItsFirstMessage` | 已验证 |
| R5 | 运行中发送：排队 / 插话 / 继续编辑确认；Ctrl+Enter 直接插话；输入法组字时 Enter 不发送 | `requestSend`、`dialogs`、`composer` | `TestARunningTurnAsksWhetherToQueueOrSteer` | 部分验证（Ctrl+Enter、输入法待测） |
| R6 | 停止：先暂停队列再停止，取消未完成的发送预检 | `stopCurrent` | 代码审查 | 待验证 |
| R7 | 待发送队列：编辑（加锁→更新→解锁）、插话、取消、排序、恢复；离开页面 `resetLocks` | `queuePanel`、`queueAction` | 代码审查 | 待验证（React 用拖拽排序，MyGo 用上移按钮） |
| R8 | 工具审批卡：Esc 拒绝、Enter 本次允许、Ctrl+Enter 本会话允许 | `approvalCard` | `TestApprovalsAnswerWithTheirKeys` | 已验证 |
| R9 | 提问卡：选项或自填，取消，停止 | `questionCard` | 渲染图 question-* | 部分验证 |
| R10 | 重试、编辑重发（可恢复文件） | `retryMessage`、`editMessage`、`beginEdit` | 代码审查 | 待验证 |
| R11 | 自动重试状态条 | `retryStatus` | 代码审查 | 待验证 |
| R12 | 加载更早消息 | `loadEarlier` | 代码审查 | 待验证 |
| R13 | 完成提示音、压缩完成提示 | `playDone`、`poll` | 代码审查 | 待验证 |

## 会话与项目管理

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| S1 | 侧栏：新对话、搜索、置顶、自动化入口、打开或新建项目、项目分组（折叠、新对话、重命名、移除）、对话分组折叠、设置、归档切换；状态图标（运行蓝、错误/中断红、待回答橙）在标题左侧空白处 | `sidebar.go` | `TestTheListShowsProjectsAndPinnedConversations`；渲染图 | 已验证 |
| S2 | 每 1.5 秒读 `activity`（侧栏可见时） | `pollActivity` | 代码审查 | 待验证 |
| S3 | 会话菜单：重命名、AI 重命名、置顶、归档、分支、压缩、复制源目录、保存位置、移动到项目、导出、删除 | `conversationMenu`、`actions.go`、`dialogs.go` | `TestExportRetriesWithoutMediaWhenTooLarge` | 部分验证 |
| S4 | 首次发送后自动生成标题；手动改名优先 | `generateTitle` | 代码审查 | 待验证 |
| S5 | 打开或新建项目（权限三选一）、保存位置对话框 | `projectFields`、`locationFields` | 代码审查 | 待验证 |

## 输入框

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| C1 | 多行输入、Enter 发送、Shift+Enter 换行 | `composer` | 渲染图 | 部分验证 |
| C2 | 模型选择（按供应商分组、搜索、折叠、图标与徽章、记住上次） | `model_picker.go` | 渲染图 | 部分验证 |
| C3 | 权限（完全访问先确认；属于项目时改项目权限）、思考强度与快速模式 | `permissionPill`、`thinkingPill`、`configure` | 代码审查 | 待验证 |
| C4 | 会话工具：压缩、提示音、任务图、暂停；继续任务 | `sessionTools` | 代码审查 | 待验证 |
| C5 | 上下文用量弹层 | `usageButton` | 代码审查 | 待验证 |
| C6 | 子代理列表与停止 | `subagents` | 代码审查 | 待验证（React 的子代理对话窗口未开始） |
| C7 | 附件：选择（≤16）、拖放（文件夹转 `@路径`）、移除（浏览器批注链修复） | `addPaths`、`DropArea`、`model.RemoveAttachment` | 代码审查 | 待验证 |
| C8 | 附件粘贴（剪贴板文件路径或截图字节，`attachments.upload`） | — | — | 未开始 |
| C9 | 浏览器批注随消息发送（`# Browser comments:` 段） | `model.AnnotatedPrompt` | 代码审查 | 待验证 |

## 消息区

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| M1 | 用户气泡、复制/编辑/重试/分支 | `userMessage` | 渲染图 | 部分验证 |
| M2 | 工作过程折叠块（思考、旁白、工具、操作）、显示推理开关 | `workProcess`、`entryView` | 渲染图 | 部分验证（工具行的 Cindy 动词与文件快照未移植） |
| M3 | 回答 Markdown（流式）、模型来源 | `answerView` | 渲染图 | 部分验证（Mermaid、公式、表格适配待与结果窗共用渲染器） |
| M4 | 计划卡 | `planCard` | 代码审查 | 待验证（吸底提醒未开始） |
| M5 | 自动跟随到底部，上滚取消，靠近底部 80 DIP 恢复 | `transcriptView` | 代码审查 | 待验证 |
| M6 | 生成文件 chip、本轮改动卡、文件链接 chip、滚动导航、Bot 模式 | — | — | 未开始 |

## 其余区域

| 编号 | 内容 | 状态 |
| --- | --- | --- |
| P1 | Chat 设置（常规、资源、MCP、自动化连接） | 未开始 |
| P2 | 右侧工作面板：标签容器、资源、文件库（原生）；文件预览/编辑、PDF、Review、终端、浏览器批注（WebView） | 未开始 |
| P3 | 自动化整页（WebView） | 未开始（入口显示暂不可用） |
| P4 | OneBot（队友对话复用原生 Chat，其余页面 WebView） | 未开始 |

## 未验证范围

- 真实 Host 与真实模型未联调；以上测试都用假 Host。
- 物理中文输入法、真实拖放、粘贴未测。

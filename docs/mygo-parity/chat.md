# Chat 页功能对照

更新：2026-11-04。React 来源：`apps/desktop/frontend/src/features/chat*.ts(x)` 及其引用的 `features/cindy/**`（功能清单见下文分区）。MyGo 实现：`apps/desktop-mygo/pages/chat`（视图与操作）、`apps/desktop-mygo/chat/model`（`chat-state.ts`、`chat-process.ts` 与浏览器批注格式的 Go 移植）。

用户 2026-11-04 改为**全部原生**（取代 11-01 的混合方案）：P2、P3 与 M7 仍在网页视图中运行 React，按 [迁移计划](../MyGo前端迁移.md#9-全部原生改写计划2026-11-04) 改写；只有浏览器标签与 HTML 预览继续用网页视图。各原生模块跟随的 Cindy 文件登记在 `third_party/cindy/native-ports.json`。

状态：**已验证** = 有单元测试、`ui.Tester` 渲染或实窗探针；**待验证** = 已实现、还缺证据；**未开始** = 尚未实现。单元测试在 `pages/chat/chat_test.go` 与 `chat/model/state_test.go`；渲染图在本地 `_temp/mygo-chat/shots/`（不入库，深浅色 × 中英 × 1100/640 宽）。

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| 编辑最后一条消息用对话框，不在气泡里原位编辑 | 原位编辑需要测量原气泡宽度；对话框保留同样的停止、文件恢复选项和失败提示，行为一致。 |
| 窄输入框里权限和思考强度只显示图标，名称在提示里 | 640 宽以下工具一行放不下，避免逐行堆叠。 |
| 完成提示音由 Go 合成同样的双音（C5、E5，0.18 秒间隔，0.45 秒衰减），用系统播放 | 原生窗口没有 WebAudio。 |
| 视图偏好（上次模型、提示音、折叠的项目与分组、面板尺寸）存于配置目录 `desktop-chat-view.json` | React 存在 WebView 的 localStorage。 |
| 导出任务的保存对话框由 MyGo 窗口弹出，Host 回执必须指向所选文件 | 与 Rust 桥 `chat_export.rs` 的约束相同，只是位置从 Rust 移到 Go。 |
| 新对话粘贴的截图先写入临时文件（`%TEMP%\WinOne\chat-paste`），草稿不再引用时删除 | 原生草稿只保存路径；React 把字节留在内存。首条消息照常导入这些文件。 |
| 面板中的网页标签由 Go 的网页视图显示；页面的批注消息由 Go 每 0.2 秒从页面队列取回（标签可见或批注进行中时） | MyGo 的网页通道只留给自有页面；不受信任的网页只能经注入脚本的队列回传批注与快捷键，与 Rust 的消息白名单一致。 |
| 网页标签被面板弹层（批注编辑器）遮挡时，页面只显示在未被遮挡的最大区域 | Rust 用窗口区域挖洞；MyGo 网页视图只能矩形裁剪。 |

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
| C6 | 子代理列表与停止；子代理消息窗口（读取最新一页与更早记录、状态变化时重读、暂停时只读、发送后续消息并重读，草稿按子代理保留） | `subagents`、`agents.go` 的 `agentDialogView` | 测试 `TestSubagentMessagesReadPagesAndFollowUp` | 已验证（2026-11-03 补齐） |
| C7 | 附件：选择（≤16）、拖放（文件夹转 `@路径`）、移除（浏览器批注链修复） | `addPaths`、`DropArea`、`model.RemoveAttachment` | 代码审查 | 待验证 |
| C8 | 附件粘贴（剪贴板文件路径或截图字节，`attachments.upload`） | `paste.go` | `TestPastingFilesAndPicturesAttachesThem`、`TestAPictureForANewChatWaitsInAFileUntilRemoved` | 已验证（物理剪贴板待测） |
| C9 | 浏览器批注随消息发送（`# Browser comments:` 段） | `model.AnnotatedPrompt` | 代码审查 | 待验证 |

## 消息区

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| M1 | 用户气泡、复制/编辑/重试/分支 | `userMessage` | 渲染图 | 部分验证 |
| M2 | 工作过程折叠块（思考、旁白、工具、操作）、显示推理开关；工具行按 Cindy 动词命名，可打开本次读写的文件快照 | `workProcess`、`entryView`、`tools.go`、`chat/model/tools.go` | `TestToolRowsNameTheirWorkAndOpenFileSnapshots`、`TestToolsAreDescribedAsCindyNamesThem`、`TestFileSnapshotsKeepWhatTheToolSaw`；渲染图 items-dialog | 已验证 |
| M3 | 回答 Markdown（流式）、模型来源 | `answerView` | 渲染图 | 部分验证（Mermaid、公式、表格适配待与结果窗共用渲染器） |
| M4 | 计划卡；运行中计划卡滚出视野时在输入框上方显示当前步骤，可展开、可关闭 | `planCard`、`plan_reminder.go` | `TestOnlyAFinishedPlanCallPublishesAPlan`、`TestThePlanFloatsAboveTheInputWhileItsCardIsOutOfView`；渲染图 items-plan | 已验证 |
| M5 | 自动跟随到底部，上滚取消，靠近底部 80 DIP 恢复；滚动导航（顶部、上一轮、下一轮、底部） | `transcriptView`、`scrollnav.go` | `TestScrollControlsStepThroughRoundsAndFollowTheEnd` | 已验证（实窗滚动待测） |
| M6 | 生成文件 chip、本轮改动卡（撤销本轮、查看改动）、回答中的文件链接 chip | `files.go`、`links.go`、`chat/model/paths.go` | `TestTheTranscriptEndsWithTheConversationsFilesAndChanges`、`TestAnswersOfferTheFilesTheyName`、`TestLinksResolveToTheFilesTheyName`；渲染图 items-end | 已验证 |
| M7 | Bot 模式（OneBot 队友对话的时间线、引用、语音） | 暂在 OneBot 网页视图中由 React Chat 提供 | — | 原生未开始 |

## 其余区域

| 编号 | 内容 | 状态 |
| --- | --- | --- |
| P1 | Chat 设置（常规、资源、MCP、自动化连接）：原生 `kit.Dialog`（`pages/chat/settings.go`） | 已验证（`TestSettingsDialogIsNativeAndSavesTheDraft`） |
| P2 | 右侧工作面板：React `ChatSidePanel` 整体在网页视图中运行（文件库、资源、文件预览/编辑、PDF、Review、终端、浏览器与批注），可停靠右侧或下方、拖动调整大小；网页标签由 Go 网页视图叠放在面板指定位置 | 已实现；实窗探针：文件库经 Host 列出目录，网页标签加载页面并把标题回传面板 |
| P3 | 自动化整页：React `SchedulerHost` 在网页视图中运行 | 已实现；实窗探针渲染通过 |
| P4 | OneBot：整页在网页视图中运行，队友对话暂为 React Chat（[OneBot 对照](onebot.md)） | 已实现；实窗探针渲染首屏 |
| P5 | 其他页面打开某个会话（可带文件或文件夹）或 Chat 设置（`takeIntents`） | 已验证（`TestLinksFromOtherPagesOpenTheirConversationAndFile`） |

## 网页视图承载（P1–P3）

- **页面**：`apps/desktop/frontend/chat-embed.html`（`src/chat-embed-entry.tsx`）。`npm run build:mygo` 用 `vite.mygo.config.ts` 只构建这一页，输出到 `apps/desktop-mygo/webui/dist`（被忽略）；`build/build-desktop-mygo.ps1` 先构建它，再以 `webui` 构建标签嵌入 WinOne.exe。
- **桥**：包 `apps/desktop-mygo/webui` 绑定 MyGo 服务 `App`，注入脚本把 `window.go.main.App` 与 `window.runtime.EventsOn` 映射到它，React 代码沿用原桥。可调用的 Host 方法与 Rust 桥白名单一致（`webui/allow.go` 由 `component_bridge.rs` 生成）；导出仍只由原生页处理。
- **回调**：面板的关闭、停靠、引用、Review 评论、浏览器批注、拖放与导航经 `App.Embed` 交给原生页（`pages/chat/panel_host.go`）。
- **实窗探针**：`apps/desktop-mygo/cmd/webuiprobe`（`webui` 标签）由 `tests/desktop-mygo-webui/run-private.ps1` 在私有桌面运行，网页数据目录放在输出目录内。2026-11-03 结果：面板、网页标签、自动化与设置均渲染通过。

## 未验证范围

- 真实 Host 与真实模型未联调；以上测试都用假 Host。
- 物理中文输入法、真实拖放、粘贴未测。
- 网页视图：真实 Host 下的编辑器保存、PDF、终端、Review 操作与浏览器批注截图未逐项实窗验证；深浅色切换已接入、未截图核对。

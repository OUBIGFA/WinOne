# Chat 界面复用对照

## 2026-10-18 Cindy 提交链路与紧凑两级右栏

本节覆盖前序发送、插话、重试、右栏与背板说明。Cindy 提交链路基准为 `77e2c32d0b028920116bfac744a04c6b6876257a`（Apache-2.0），源码、版权、许可摘要和适配范围见 `third_party/cindy/chat-interaction-source.json`。直接复用 Enter 意图解析、发送快照和自动跟随的纯函数；现有 Cindy 输入、发送按钮、编辑框及右栏组件继续使用，WinOne 只适配 Host、Pi Durable、草稿存储与用户指定的分区。

| 上游行为 / 本次要求 | WinOne 接入与可观察结果 | 验证 |
| --- | --- | --- |
| 普通发送与快捷键 | `ChatComposer` 复用 `resolveComposerEnterIntent`；Enter 发送、Shift+Enter 换行，IME 和长按不误提交；运行中 Ctrl+Enter 直接插入，普通发送保留继续编辑／排队／直接插入三选确认 | `cindy/composerEnterIntent.test.ts`、`chat-followup.test.tsx`；原生 Enter 和 Ctrl+Enter |
| Send / Stop 两个槽位 | 有草稿时发送始终最右，运行中停止在其左；无草稿或发送事务中停止占主槽位，不被发送锁禁止 | `chat-send-cindy.test.tsx`；原生输入下一条后实际点击停止 |
| 发送快照与失败草稿 | 点击时冻结本条，清空后允许写下一条；迟到回执不清空新草稿；拒绝时合并保留两份文字和附件 | `composerSendSnapshot.ts`、`chat-send-cindy.test.tsx` |
| 排队与同轮插入 | 共用附件校验、请求收据和官方 Durable inbox；插入到下一工具边界，排队到下一轮，不通过中止重发伪装插话；相同文字也不交换收据 | `worker-followup.test.mjs`；原生模型第 2 次请求只含插入，第 3 次才含排队 |
| 已受理与未发送草稿分开 | Durable 已落盘受理后，Host 仅确认对应 PendingID；重开不把已入队内容塞回输入框，不清除较新的草稿 | `internal/chat/send_retry_windows_test.go`；原生读取 `draft.get` |
| 重试／编辑的连续渲染 | Worker 先发送分支快照，再发送新回复；历史导入不再伪造助手 start；前端不以迟到 state 覆盖流式内容；旧答案保存在原分支 | `worker-followup.test.mjs`、Host 收据回归；原生连续 DOM 观察、消息可见性与窗口截图 |
| 编辑重发与文件恢复 | Cindy 内联编辑、焦点、IME、防双击和失败内容保留；文件变化预览慢或失败不阻止重发，保留明确的文件恢复选择和原附件 | `chat-send-cindy.test.tsx`、原生编辑重发 |
| 附件与停止 | 无正文附件可提交；停止真正取消上游流，不发送或清除正在写的下一条 | 原生文件读取、7 次请求（含 1 次主动停止）、服务端断开及重开草稿 |
| 阅读位置 | 直接复用 `autoFollowIntent.ts`；明确上滚解除跟随，迟到受理不会抢回底部 | `chat-send-cindy.test.tsx`、`chat-followup.test.tsx` |
| 两级右栏 | 「文件与资料」固定含文件／资料；「标签页」保留文档、编辑／预览、浏览器、终端、审查和完整标签管理；旧状态迁移保留顺序、固定和选择，资料点击转入文档标签 | `chat-panel-state.test.ts`、`chat-side-panel.test.tsx`；浏览器与原生资料预览／切换 |
| 导航、背板与伸缩 | 导航独立占 38px，不盖住正文、滚动条或输入区；背板左右／底部收紧为 3px，圆角与输入框同心；增高输入区不改变正文视口和滚动位置 | 浏览器 13 场景；原生深浅色 1280×940 / 800×600，工具无裁切、页面无横向溢出 |
| 工作过程 | 继续保留 Cindy 最近五项、展开全部／收回、长过程收起定位与短过程不跳动；正式回复不收进过程 | `chat-process-segments.test.tsx`、浏览器实际展开／收起 |

原生入口为 `apps/desktop/frontend/tests/chat-send-retry-native-check.mjs`，通过 `build/test-desktop-components.ps1 -FullBundle` 使用关闭全部模块、快捷键、自启和后台界面的隔离配置运行。浏览器入口为 `build/test-chat-repairs.mjs`。本轮使用生产内嵌桌面资源和真实 Host → Magpie → Pi，仅模型为本地受控服务；没有把 jsdom 或最终快照当作流式实窗验证。证据及组件快照边界见 `docs/development.md` 最新同名记录。

## 2026-10-16 稳定性与 Cindy 交互修订

本节覆盖下方历史说明中的右栏组件来源、运行中强制展开与背景规则。右栏、文件浏览、预览和工作过程按既有 Cindy 来源清单直接复用，Host 适配保留所有入口和未保存保护。来源分别记录在 `third_party/cindy/{renderer,file-browser,file-view,review,chat-repair}-source.json`；不再采用历史 ChatGPT-Desktop-UI 右栏布局。

| 用户要求 | 当前接入与完整行为 | 验证入口 |
| --- | --- | --- |
| 连续追加与完成通知 | 中间一轮结束不阻塞 Durable 事件消费；执行空闲后等待最后一条已提交消息完成投影，才发出已完成，避免后续回复晚到。保留回执去重、重试分支与重开历史 | `modules/chat-pi/worker-followup.test.mjs`，实际 SSE 模型与持久化；续接验收重复 5 轮，并用受控第二轮响应的回归重复 3 轮 |
| 文件标签不再大量无法读取或 `busy` | 元数据通过 `file.stat` 按 64 个合并；复用同路径请求，后台读取最多并发 6 个；点击文件才读取预览。缺失历史文件省略，权限及传输错误仍可见并可重试 | `chat-file-access.test.tsx`、`chat-read-queue.test.ts`、`internal/chat/file_stat_test.go`、原生驱动 |
| 文件与目录识别 | 仅对明确的文件/目录候选做存在检查，普通标识符保持代码；目录打开文件浏览器；本轮新增/修改文件仍独立位于全部消息尾部 | `chat-file-access.test.tsx`、`chat.test.tsx`、浏览器驱动 |
| 资源标签可用 | 资源与正文使用同一 Open 屏障；未配置模型时仍检查本地 AGENTS、技能与插件清单，不执行扩展或启动 MCP；保留资源权限边界 | `internal/chat/resources_windows_test.go`、原生驱动先读 resources 再打开会话 |
| 文件标签翻译、筛选和搜索 | `files.list/index/search/cancel` 贯通 Rust → Host；“选择文件”使用正确翻译键；按锁定清单打包 `rg.exe`，不依赖开发机 PATH | `chat-file-browser.test.tsx`、`internal/chat/file_search_windows_test.go`、原生驱动 |
| 预览、悬停提示与真实深色窗口 | Cindy HSL 三元值统一适配为 WinOne 完整颜色值；预览使用实体表面，提示使用深底白字。Chat 主题优先读取真实应用的 `data-theme`，保留 class-only 嵌入兼容；浏览器夹具移除额外 `.dark` 类，避免掩盖真实窗口的白底浅字问题 | `chat-desktop-kit.test.tsx`、`build/test-chat-repairs.mjs`、原生驱动计算样式及截图 |
| 改动摘要与窄窗审查 | 保留每轮快照、撤销/重新应用、查看、文件菜单、提交与推送；弱化字色/图标/结构线，工具栏换行且只读提示独立成行 | 同上及 `chat-turn-changes.test.tsx` |
| 工作过程完整复用 Cindy | 运行中默认展示最近 5 项；有更多内容时可展开全部并收回最近 5 项；完成后默认收起。公开正文始终独立，工具详情完整保留，不使用 `forceExpanded` | `chat-workflow.test.tsx`、浏览器驱动运行中展开/收起 |
| 收起后的阅读位置 | 短内容收起保持滚动位置；只有超出可视阅读区的长过程收起或收纳时定位到组头 | 浏览器驱动分别实测两条路径 |
| 背景、输入与左栏 | Chat 画布跟随 WinOne；把手和占位文字弱化；输入框背板左右/底部各超出 6px，顶部项目栏 38px；左栏拖动停在 224px，手动收起仍可用 | 浏览器驱动双主题、1280/800 宽度与连续拖窄 |
| 导航不挡滚动条 | 导航按钮与垂直滚动条命中区域之间留出可操作间隔，保留顶部/底部与按轮导航 | `chat-scroll-controls.test.tsx`、浏览器几何检查 |

浏览器交互入口为 `build/test-chat-repairs.mjs`；真实链路入口为 `apps/desktop/frontend/tests/chat-native-repairs-check.mjs`，由 `build/test-desktop-components.ps1 -FullBundle` 使用仓库内新配置及专用工作目录运行。最终通过结果和未覆盖范围以 `docs/development.md` 为准。

## 2026-10-14 对话展示修订

本节覆盖下方历史记录中的正文排版、工作文字分组和修改摘要位置。Cindy 源码基准为 `69661422beeb85d692fa3704df6d2b42884f4c4b`，Apache-2.0；来源与适配范围见 `third_party/cindy/chat-repair-source.json`。右栏保留已有 ChatGPT-Desktop-UI 组件与完整操作。

| 对照项 | 当前接入与可观察结果 | 验证入口 |
| --- | --- | --- |
| 开头、中途、最终正式回复 | 共用 `ChatMessageView`，独立 Markdown、复制、分支、来源；文字不进入折叠组 | `chat.test.tsx`、`chat-process-segments.test.tsx`、`chat-workflow.test.tsx` |
| 思考、工具、工作状态 | 复用 Cindy `groupWorkRuns` / `WorkGroupBlock`；文字隔开动作段，运行中保持展开，完成后动作可折叠，工具详情完整保留 | 同上；完整窗口检查运行中/完成态 |
| 正文、标题、列表、行内代码 | 直接适配 Cindy `CHAT_BODY_CLASS` 与 `MarkdownRenderer`：正文 15px/24px，标题 20/18/16px；保留文件链接、代码复制、表格与长内容换行 | `build/test-chat-rendering-layout.mjs` |
| 文件变化数量与增删行数 | 保留 Cindy 修改卡的真实快照、总数、文件行与展开；压缩头部和行高，不使用演示计数 | `chat-turn-changes.test.tsx`、`chat-rendering.test.tsx` |
| 变化卡位置与多轮对话 | 各轮卡片集中在消息列表末尾，新增用户消息与回复仍排在它们前面；卡片仍绑定原轮次 | `chat.test.tsx`、完整窗口实际发送第二条消息 |
| 查看、文件菜单、撤销/重新应用 | 保留原 Host 回调、冲突/部分捕获提示、运行中禁用；底部固定留白使最后一行可滚动到输入区上方 | 修改卡单测、完整窗口点击与 Go Chat 测试 |
| 右栏空白与收纳 | 外壳在 Chat 取消额外滚动条预留；使用现有分栏组件收纳，并在重新打开时恢复宽度；未保存文件时保护内容 | `build/test-chat-cindy-alignment.mjs`、`build/test-chat-panel-layout.mjs` |
| 标题失败兜底 | 自动标题用当前输入，手动 AI 重命名读取已保存的活动分支；失败时保存前 40 个字符，保留错误提示并保护较新的手动名称 | Host/Store 标题测试、`chat-management.test.tsx` |
| 标题生成模型 | 每次打开设置读取 AI 服务的最新已勾选模型；读取期间禁用旧列表，完成后更新现有选择，不必切换供应商 | `chat.test.tsx`；完整窗口首次打开 90 个模型并滚动到首尾项 |

完整窗口脚本使用生产 `AppShell` 与 `ChatPage`，覆盖 800/1440 宽度、浅色/深色、运行中/完成态。数据来自隔离夹具，不连接真实供应商或修改用户文件。编译和正式安装器状态以 `docs/development.md` 最新记录为准。

## 历史对照

2026-10-13 补充：用户要求增大 Chat 默认宽度。正文与输入区共用 960px 最大内容宽度，窗口不足时随可视区域收缩；保留上游字体、字号、行高及段落间距。`build/test-chat-interactions.mjs` 同时检查宽窗口的实际宽度及窄窗口适配。

来源：[ChatGPT-Desktop-UI](https://github.com/liuminxin45/ChatGPT-Desktop-UI)。实际依赖为发布包 `chatgpt-desktop-kit@0.6.0`，MIT，版本与完整性摘要固定在前端锁文件中。公开组件从发布包直接导入；包外的示例布局样式和右栏结构保留来源注释。2026-10-13 重新读取实际 GitHub 源码，HEAD 为 `b5ea2bd312e9e9c007b84859d550314ddae4acb5`，同时核查了本地安装包的 `src/ai.css`、`src/components/feedback/index.tsx` 和 `src/components/conversation/ai-response.tsx`。

上游是界面库，没有真实文件、终端、Git 或 AI 服务后端。本次复用限于用户指定的 Chat 内容区、右栏和输入框项目栏；已有 WinOne 操作仍通过原 Host 执行。

| 能力 | WinOne 接入 | 验证 |
| --- | --- | --- |
| 正式回复的字体、字号、行高、段落、标题与列表间距 | `ChatMessageView` 直接使用 `ConversationMessage`、`AIResponse`；后者直接调用上游 `Markdown variant="ai"`。`client-thread--chat` 沿用示例布局 | `test-chat-rendering-layout.mjs` 对照直接渲染的上游组件，逐项比较字体、字号、字重、行高、外边距和列表缩进；当前正文为系统字体、16 px、26.4 px 行高 |
| 标题、段落、强调、删除线、引用、普通/有序/嵌套/任务列表、链接、图片、分隔线、表格、代码块与复制反馈 | 用户消息、过程正文和 Markdown 文件预览共用 `ChatMarkdown`；保留上游 GFM 解析及代码语言栏、复制成功/失败反馈 | 同一浏览器入口覆盖长链接、长行内代码、长代码块和多列表格，核对原始完整代码复制结果 |
| 对话内容禁止横向滚动 | 在 `chat-markdown.css` 只适配内容换行：代码保留空白并换行，表格按可用宽度分列，单元格和长词换行。字体与段落规则继续由上游提供 | 320、360、720 DIP，明暗主题，用户与助手两种角色共 12 组；检查各可见元素实际 `scrollWidth`，无横向溢出 |
| 主题、字号令牌、中英文提示 | `ChatDesktopSurface` 使用 `DesktopProvider`、`UIStringsProvider`，继承 WinOne 主题与语言 | 明暗、中英、窄宽窗口 |
| 工作区、页栏、工具栏、按钮与提示 | 右栏使用 `WorkbenchPage`、`PageBar`、`PageToolbar` 和共享按钮；标签增加上游键盘导航算法 | 尺寸、标签切换、关闭保护 |
| 输入框项目背景、选项按钮、图标与分支对齐 | 使用上游 `client-composer-context`、`client-context-select` 和 `client-composer` 组合；保留项目菜单与 Git 刷新 | 单行省略、项目切换、焦点重复读取回归 |
| 文件列表、目录导航、资源与拖拽引用 | 保留真实 Host 路径；嵌入状态也提供刷新按钮 | 隔离目录文件浏览 |
| 文本与代码编辑、行号、搜索、折叠、跳行、保存、冲突保护 | 保留 CodeMirror；ShadowRoot 隔离页面样式并使用构造样式表，兼容正式 CSP | 正式 CSP 下编辑保存；Host 文件写回 |
| Markdown/HTML 源码与预览切换 | 保留切换入口；Markdown 共用上游排版；HTML 保留沙箱 | 源码/预览、HTML 隔离策略 |
| 图片缩放、旋转、适合窗口；PDF、音视频 | 保留全部类型和操作；放行 Host 本机能力 URL；PDF 不再错误套用 HTML 沙箱 | 图片、类型判断、PDF 响应策略 |
| 复制路径、系统打开、资源管理器定位、不支持格式提示 | 原入口与 Host 方法保留 | 操作路径审查 |
| 标签新建、关闭、重开、排序、固定、全屏、底部停靠 | 原状态与操作保留，补回资源标签重建入口 | 原生工作台与浏览器检查 |
| 浏览器、批注、终端 | 原实现与真实执行路径保留 | 原生工作台回归 |
| 用户和助手重试 | `ChatMessageView.onRetry` 接通调用方；用户附件消息和只有错误信息的助手回复也保留重试入口 | `chat-rendering.test.tsx` 覆盖三种消息；完整重试流程由 Chat 主流程测试覆盖 |

## 文件改动与审查的实际边界

指定上游没有 Diff/Git 审查后端或完整差异组件。不能把演示数据描述为真实审查能力，也不能以替换演示外观为由删掉 WinOne 已有审查操作。

| 上游能力与实际源码 | WinOne 入口与实现 | 验证 |
| --- | --- | --- |
| `examples/demo/ClientPane.tsx:128` 的项目菜单 `Changes` 只调用 `actions.panel()` | 项目上下文的改动入口打开真实审查面板 | 保留现有项目上下文接入；本次检查调用路径 |
| `examples/demo/ClientPane.tsx:334` 的工具 `Changes`、`Ctrl+Shift+G`；点击执行 `unavailable()` | `UpstreamNewTab` 的文件改动按钮打开 `ChatReviewView`，快捷键由 Chat 侧栏接入 | 本次读取源码并核对入口，不复制上游不可用提示作为执行结果 |
| `examples/demo/ClientDemo.tsx:1120` 的改动摘要是固定 `Updated 3 files / +42 / −16`；`View changes` 只调用 `setPanel(true)` | 保留摘要外观和 `client.changes.open` 操作。成功的写入/编辑工具记录使正文显示“查看文件改动”，`onOpenReview` 打开真实面板 | `chat-rendering.test.tsx` 核查点击回调及没有编造文件数量；当前 Host 没有按轮次统计文件数量，因此不显示演示数字 |
| 上游 `EmptyState`、`ErrorState`、`PageToolbar` | 审查加载、空状态和错误状态复用状态组件；文件路径和操作使用工具栏，避免文字与边界贴合 | 浏览器额外测量 320、720 DIP 审查空状态左右及顶部留白；快照读取失败单测确认不会同时显示“没有文件改动” |
| 上游没有的真实 Git 能力，由既有 Cindy 解析/行配对与 WinOne Host 执行保留 | `chat-review.tsx` → `review.branches/commits/snapshot/diff/mutate`；保留未暂存、已暂存、分支、提交范围，基准选择、筛选、统一/并排差异、换行开关、差异块及全部折叠、文件与差异块暂存/取消暂存、全部暂存/取消暂存、还原确认、复制补丁、打开文件/行、评论、提交说明、包含未暂存选项、提交与推送确认 | 本次组件测试覆盖差异呈现、折叠/展开、打开文件、复制入口、暂存请求及快照 revision；真实 Git 既有测试入口 `internal/chat/review_windows_test.go`，本次未重新执行提交/推送/还原 |

本次修复的根因：上游代码块为 `white-space:pre; overflow-x:auto`，表格为最大内容宽度及可横向滚动；直接接入后与用户的完整换行要求冲突。上游双类选择器也要求适配规则有足够优先级。旧审查空状态为无内边距的普通段落；快照失败时还会把“未读到快照”误判为“没有文件改动”。现在使用上游状态组件，并区分读取失败、尚未选择基准、加载中和真实空快照。

## 输入与验证范围

已有对话的输入框底边固定，向上增高，可遮盖正文；正文滚动区及末尾固定留白不随拖动变化。欢迎页初始以 Logo 和输入区的组合居中，上方拖动把手使输入区上下边缘连续展开；下边缘抵达窗口边界后继续向上增高。分支读取只在首次打开、项目变化和手动刷新发生，窗口焦点变化不触发读取。

2026-10-13 本次正文与审查验证：`chat-rendering.test.tsx` 与 `chat-message-collapse.test.tsx` 共 8 项通过；`build/test-chat-rendering-layout.mjs` 共 14 组通过，无浏览器脚本错误，正文排版对照一致，无横向溢出，代码复制完整。审查空状态截图已目视核查。浏览器测试使用隔离的 Host 数据边界，不等同于已验证真实 Git 提交、推送或还原。

其余可复现检查入口为 `build/test-chat-ui-rebuild.mjs`、`build/test-chat-panel-layout.mjs`、`build/test-chat-reading-layout.mjs`，以及前端 Chat 单元测试、`internal/chat` 文件预览测试和原生工作台检查。主流程、完整前端套件和正式交付结果写入 `docs/development.md`。

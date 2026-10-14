# Chat 界面复用对照

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

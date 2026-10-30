# Chat：Pi 内核与 Cindy 界面

用户已确定：Chat 嵌入 WinOne 主窗口，使用 Pi 执行代理能力；UI、布局、输入框和权限交互参考 Cindy。原有打开文件夹、新建项目、无项目对话自选位置、AI 修改文件和执行命令要求继续适用。

## 2026-10-26 自动化与 OneBot 接入

自动化当前直接同步 Cindy `b7e46bcb` 的 scheduler renderer、引擎与相关实现（108 文件），由宿主适配复用，不再维护旧 `chat-automations-*` 页面。新建不显示权限选择，使用完全访问；绑定会话继承其权限；旧任务保留原权限和原定时刻；“一次”沿用 `recurring=false`，表示下一次触发。模板、前置检查、项目配置、历史与未读操作见 `modules/chat-pi/AUTOMATIONS.md`。

OneBot 使用同一 Pi/Host 基座但独立本地内容 worker。档案/版本、记忆/FTS、技能与待办已接入宿主接口；主会话、三层身份提示词和会话绑定的 Pi 工具已通过隔离验证。**OneBot 页面与 IM 实用回路尚未接通，整体未完成**。完整剩余清单与验证边界见 [OneBot](OneBot.md)。本轮没有更新正式安装器。

## 固定参考

- Pi：`@earendil-works/pi-coding-agent` 1.4.0；[源码](https://github.com/earendil-works/pi/tree/4a080252f06caf63a525d7873c5d94d3c23887e0)，MIT。依赖锁位于 `modules/chat-pi/package-lock.json`。
- Cindy：[源码](https://github.com/makecindy/cindy/tree/a152497ac24d155dd46142b73dda4fa10ad08ad0)，Apache-2.0，Copyright 2026 XD Inc. 直接改编代码时保留 LICENSE 与 NOTICE，并记录改编范围。
- Cindy 的具体界面入口：`components/new-chat/ChatInput.tsx`、`PermissionSelector.tsx`、`PermissionPrompt.tsx`；`features/cc-agent/SessionContentHeader.tsx`；`features/cc-agent/sidebar/sections/{ProjectsSection,ProjectNode,DialogueSection}.tsx`。
- Cindy 权限入口：`packages/maker-core/src/agents/pi/auto-review-policy.ts`、`agents/shared/auto-review.ts`、`auto-review-decision.ts`。其 Pi 采用 Ask、Auto-review、Full access；Auto-review 不等于自动允许所有命令。

## 接入边界

WinOne 保有主窗口、项目索引、草稿和 AI 服务配置。Pi 保有工具、执行循环、压缩及会话历史。每个活跃会话通过专属回环地址和临时密钥调用 WinOne 的 Magpie 适配器；供应商密钥不进入 Pi 进程或前端。公开本地网关的设置不受 Chat 影响。Chat 的协议调用计入 WinOne 用量。

Pi 与命令子进程归 Windows Job 管理。关闭 Chat 页面或主窗口时，空闲进程退出；运行中的任务完成后退出。程序整体退出时先停止 Chat，再关闭其他模块。停止失败须返回错误，不能仅移除进程记录后宣称退出。

## 界面实施要求

- Chat 位于 AI 服务上方，使用 Bot 图标，两者之间有细分隔线。
- 新对话欢迎图使用用户提供的 `dist/logo/winone-icon-logo.png`，源码副本为 `internal/branding/chat-welcome.png`；保留原图比例。欢迎文字为 `WinOne Set Your Creativity Free`，不显示下方说明。会话侧栏使用 ChevronsRightLeft / ChevronsLeftRight 收起、展开；底部直接提供归档图标。侧栏项目入口统一为“打开或新建项目”，新对话输入框上方提供项目选择及真实 Git 分支。
- 会话区采用 Cindy 的项目与独立对话分组、对话标题和工具活动呈现；内容独立滚动，输入区固定在底部。
- 输入区采用 Cindy 的圆角边界、紧凑工具入口、模型与权限胶囊、发送/停止位置和窄窗口适配。使用 WinOne 的本地图标、主题和双语资源。
- 权限请求在输入区显示标题、动作说明、可核查的命令或文件变更、拒绝、允许一次，以及有明确范围时的本会话授权。保留原草稿。自动审查失败要明确交给用户，不暗中放行。
- 文件夹、项目与会话操作须真实接入本地存储。移除项目只改变索引，不删除项目文件。输入确认、错误、停止、分支、重开和待授权状态要完整呈现。
- 模型入口同时显示供应商和模型名称。每次对话均可指定独立保存位置，项目目录与会话目录分开；已有会话可以转存，保留原副本，不覆盖目标位置的不同数据。
- Chat 与原生结果页采用 Cindy 的思考状态：生成中及完成后默认折叠，用户展开状态不被流式更新重置；正文仅解析加重与行内代码，不套用完整 Markdown。计时记录真实接收阶段，旧记录不补造耗时。完成图标按用户要求用 Layers，生成中保留 Sparkles。展开内容与后续回答之间另留 16 DIP 间距。

## 当前状态（2026-10-10，本地包已验证）

已接入 Pi 会话适配、项目/草稿存储、Windows 进程管理、专属 AI 接口、Host 的 Chat 模块、Rust/Wails 桌面调用桥和主窗口 Chat 页面。权限采用 Ask / Auto-review / Full access，自动审查使用 Cindy 分类器与当前模型；模型故障明确转交用户确认。Cindy 直接复用文件及来源校验记录见 `third_party/cindy/ui-source.json`；WinOne 适配器在 `apps/desktop/frontend/src/features/cindy/`。

已验证真实 Pi 授权写文件、会话恢复、跨重开去重、空闲进程回收、模型移除后的历史读取、分支保留源历史、独立保存与冲突保护、思考时间记录随 Pi 历史保存。Go 与前端使用同一组思考文本夹具；原生结果页的展开状态和小窗口布局回归通过。源代码与单文件 Pi bundle 均通过真实进程测试，构建入口为 `build/build-chat.ps1`。

本轮已完成 AGENTS.md、自定义技能与插件目录、MCP、附件、会话管理、左右侧栏、项目选择及 Cindy 自动化接线。自动化能力对照见 `modules/chat-pi/AUTOMATIONS.md`；本次总包验证见 [Chat 补齐验收](2026-10-10-Chat补齐验收.md)。重新生成了包含最终绑定模型选项修复的 worker、scheduler 与 script-runner，不再使用此前候选的摘要。

本地回归覆盖真实 Pi、Host 脚本及本地 MCP、进程回收、配置兼容、3529 个载荷文件、隔离安装/覆盖/回滚/修复/卸载。浏览器布局检查使用隔离接口夹具；真实供应商账号、CSV 原生另存为对话框、物理鼠标与多屏场景仍需实机验收。按既有要求跳过第三方卸载软件交接，未对外发布。

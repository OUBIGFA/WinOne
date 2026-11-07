# Cindy 本地插件平台原生接入对照

更新：2026-11-07。固定上游 `2d8d3c3605ee9073613dcd4069bc59f799be61ae`；不是最新 Cindy main 的全量同步声明。当前 MyGo `v0.8.1`、Pi `1.5.0`。

## 边界与验收口径

复用原 GhostManager、Runtime、SDK、各 slot、任务/目标/Orca、MCP Apps、连接及邮件规则。WinOne 仅适配真实 Host、SQLite、进程、原生窗口和目录；不制造受理/发送成功、不以假队列或重放业务调用替代原链。只有插件自有网页、MCP App 内容与邮件 HTML 文档使用隔离 WebView，表单、确认、权限、账号密码和导航由原生控件提供。

用户明确排除 Cindy 云端服务、远程电脑、Chat Server 与非 Pi 执行端。直连 OAuth、本机浏览器扩展、邮件草稿、MCP Apps 与本地 handoff 均在范围内。真实账号、生产 NativeMessaging 登记、物理 IME/音频设备与系统挂起未自动实测；本机夹具和原生 Windows 测试不能冒称这些外部环境通过。

## 完整能力对照

| 原能力 | WinOne 入口/实际实现 | 验证证据与范围 |
| --- | --- | --- |
| 包、namespace、声明、启停/安装/更新/卸载、崩溃恢复、按需/常驻 | `ghost/host.mjs`、原 Manager/Runtime、原生 `pluginchecks`、独立 `WinOne.Plugin.exe` | 真实 `.cindy` 与 SDK 生命周期测试；最终安装/全进程退出按当前候选另验 |
| tools、命令、manual、静态/动态 recommendations、目录可用性 | 原 MCP/tool catalog、validation/store；`platform.mjs` | SDK 原调用与动态推荐存储/快照回归，不用原型 JSON 假受理 |
| locales、app-context、模型上下文、sessionContext | 原 locale/derive context；Host 重铸身份/只读状态 | 真实沙箱语言切换及伪造上下文字段拒绝；wire 不含路径/凭证 |
| 图像/视频生成编辑、文本、embedding、媒体存储/释放/配额 | 原 CindySlot/ledger/ingest，Host Magpie 网关 | 当前 provider HTTP 夹具、改图/视频下载/向量排序/大媒体与持久台账；不修改真实模型设置 |
| 四边 panel、main/task/settings 页、导航开关、多实例、独立窗口/位置/尺寸 | 原偏好/页面元数据，`globalpages`、`pluginpage`、Shell 动态导航 | 当前真实四边/独窗/高度/位置/实例矩阵通过；OS 焦点/鼠标/F8、3 秒过期、外链取消与 ledger→原草稿/附件→用户确认→ACK 通过，DOM focus 不当权限票 |
| 普通卡片、子卡、派生卡、完整字段/控件、动作/变化、model-select | 原 card services、`cards.go`/`cards_form.go`、共享模型菜单 | 真实 SDK 原卡/表单及原生组件；保留失败、只读、草稿和重复回执边界 |
| 消息/工具/权限/回合事件，逐消息覆写/block，调度/例行前置检查 | 原 gateway/hooks，`platform-events`/publisher，Host 持久投影 | 原始输入和消息身份、重复事件、持久覆写、allow/skip/block 原链回归 |
| 公共网络、secret bindings、OAuth/client overrides、动态 connections | 原 network/SSRF/Secrets/OAuth/setup coordinator/authorization service | 单跳固定 DNS/current install 检查；原生 Password 私有提交，普通插件 pipe/模型不可见 token |
| Node 主/额外 entry、JSON-RPC/MCP、子进程、超时/取消/退出 | 原 NodeRuntimeBroker/Process，真实 Bun 子进程 | SDK 真实进程/子进程重启与退出；browser policy 首次 send/wake 等待真实 SDK ready |
| preview、pick、多文件/目录/过滤、确认、notify | 原 slot 和真实原生 dialog，Host capability preview | 仅当前 owner/install 的受理；取消不产生结果、通知/确认限速；不打开未授权路径 |
| skill 随包快照、停用/升级安全 | 原 snapshot verification，`agent-skills` | approved immutable snapshot，不读取已变的插件目录冒充批准内容 |
| fs 私有数据/workdir/save deposit、Library 文件/SQLite/流/暂存/复制/迁移 | 原 FsSlot、LibrarySlot/Vault/SQL/Binding/Downloader；Host library read bridge | CRUD/SQL rollback、流 ACK、长路径与 canonical guard；模型只读库映射、撤销和 Legacy Plan gate |
| 当前 Agent continue/new/fork、errand、workspace draft 与 schedule draft | 原 Agent/Errand/Workspace/TaskView，Host legacy adapters、真实 PiInbox | 原 owner 意图/权限/目录、幂等 clientId、持久映射失败回滚、复制/重开去重；草稿不启动 Pi |
| tasks/hidden task、参数、setModel、队列/steer、run/query/cancel/archive、隔离 worktree | 原 PluginTaskService/Store 与 Host/Pi/Durable | 忙时当前回合旧模型、排队新模型；真实 staged/dirty/new 文件归档恢复和安全补偿 |
| requestWriteAccess、目标 project/task/bot、delegation、observe | 原 write gate、target/delegation/observe，Host 原始限制投影 | 拒绝状态跨模式不重问；Auto 只审原主人限制，不能由插件续接升级授权 |
| companions、interactions/watch、Orca 21 工具/Worker/team | 原 Hub、Orca services/store，真实 Host 归属与 Pi Worker | 正确 provider/model/effort、Lead 队列身份、拒伪造完成、物理工作树快照/恢复 |
| routine sources/events/publish、scheduler eligibility | 原 RoutineSlot 和 Cindy Scheduler/OneBot engine | 注册/状态/幂等、运行前 allow/skip/block；没有额外无人值守生产任务 |
| 未读/淘汰熄灭/seenAt | 原 GhostUnreadStore 和 Host 页面 snapshot | 跨重启持久化、陈旧检查和淘汰广播，不用 `kv.badge` 原型替代 |
| MCP Apps model/app audience、官方 bridge、工具/资源、progress/cancel、Range/download、文件 handoff | 原 MCP Apps/AppBridge + 官方 `ext-apps@1.7.5`，`mcpapp` 原生 Page/SetupPrompt、Host 正常 execution lease | 真实普通业务调用后 publish，一次性页票仍经 Pi 批准；根 `_meta` 不进模型；旧 View/owner/revision 失效拒绝，业务不重放 |
| App message send | 原 authored message/escaping + Main-only live guard，真实 Host/Durable inbox | `onPayload` 必经准入、每个工具真实 block；关闭/重启后的队列失效，新主人输入不继承 App 授权 |
| 连接续接/文件 handoff | 原 coordinator/authorization service/store，Host 私有 delegated continuation | 幂等且保留原 owner 限制；文件只接真实 ledger URI，复制到任务附件，不接受任意本地路径 |
| 邮件 draft/HTML/CID/附件/autosave/merge/10 秒/Undo/now/继续/删除/Reader | 原 MailCardService/Reader/runtime、原 sanitizer/serializer，原生 `mail*.go` | 真 SDK/provider 10 项、原生 WebView2 三组合；长 Reader 保留原 1 MiB，compose 限额不变；私有文件借运不截断 |
| 浏览器 extension protocol-v3、pairing/native binding、signed relay、page jobs | 原 BrowserExtensionSlot/Bridge/NativeBinding，当前 Host constructor seam | 真实 Host→SDK main.js 4 项；所有 registry 操作仅隔离 fake run。已交付/unknown 工作不重投，token 不进公共状态 |
| 提示/警告关闭 | 原生 kit Notice/InlineError/Status/Toast、结果窗反馈；保留网页 Notice/rich status | 中英键盘与内容身份；只收起提示，不清草稿、不解除权限/validation，不绕过未保存确认 |

## 当前可复核记录

证据根为 `_temp/plugin-task-close/resume-20261106/`，临时日志/截图不入库，正式测试在源码内：

- `build/test-native-ghost-checks.ps1 -RunSystemChecks`：`ghost-system-mail-current.log.utf8` **139 通过、0 失败、无跳过，129.24s**；原 SDK/HTTP/存储/真实独立 WebView2 逻辑进程。
- `mail/mail-backend-serial-full/tests.utf8.log`：**10/0** 原邮件 provider 全量；`mail/browser-source-stable-full/tests.utf8.log`：**4/0** 本机 browser bridge。
- `mail/native-mail-standards-final/tests.utf8.log`：3 项真实 Windows 原生/邮件 HTML 组合；`mail-reader-large/tests.log`：真实 Go→Bun 只读长正文/原净化/私有暂存清理。
- `mcp-host-authority-current/tests.log`：真实 Host/Pi 主人限制与失效 App 输入准入；`mcpapps-capability-audit.md` 保存标准 App/WebView 深层导航和 handoff 的逐项过程证据。
- `ui/rich-v08-final1/result.json`：**216 项**真实原生富内容窗口/绑定动作；`native-image-host-current.log`：生产 Host peer/image-preview、真实 PNG、草稿及跨任务边界。
- `performance-and-notices-evidence.md`：真实配置投影/RPC 保存测量、首帧红→绿和提示关闭。

- `ui-final-fresh/{main-1,geometry-1,settings-3,gesture-3,media-1}/`：当前 v0.8.1 实例/高度/位置/焦点/OS 手势/3 秒过期/外链与媒体草稿 ACK；前台组分别20.47/19.55秒回原桌面。
- `ui-final-fresh/{remote-images-1,automations-5,owned-ui-1}/`：真实 HTTP 图片/大合法 PNG、绑定 Pi 终态/通知失败/删除与 paused 恢复/草稿保护，157 个默认原生回归通过（17 项显式系统 skips 分开列出）。
- `mcp-com/{native-frame-081-ready,native-official-app-081-current,unit-081-current}/`：当前 COM ABI/checkptr、Frame2/Frame7 深层安全与官方 App 实窗链、普通/生产 vet。
- `native-ports-accepted-fixed.log.utf8`：18 个 native 模块、9 份 sync 规格的固定2d strict 检查通过，原功能 pending 依据组合行为证据核销。

最终构建/退出/隔离安装、安全与资源记录见 [本次验收](../2026-11-07-native-local-delivery.md)。用户2026-11-07准许本次先交付已验收功能：新进程重开805ms P95尚未达到300ms，明确保留为后续优化，不冒称性能全达标。正式固定文件及Git交付以 `dist/release/release-info.json` 的最终记录为准；不把候选编译当成交付。

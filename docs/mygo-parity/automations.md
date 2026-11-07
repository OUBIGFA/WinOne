# 自动化 MyGo 原生迁移

更新：2026-11-07。原生主体及真实窗口/Host的自动化验收通过；模板、精确项目预览批准、通知失败/绑定、删除/paused恢复、未保存保护和窄窗详情均已核对。见 [本次验收](../2026-11-07-native-local-delivery.md)。下方早期局部待验表保留历史，不据此重复实现。

## 实现与来源

- 原生代码：`apps/desktop-mygo/chat/automations/`。
- 实际入口：`apps/desktop-mygo/pages/chat/panel_host.go` 的 `automationsView` 已调用原生 `automations.Page`，不再创建 `View: "automations"` 的 WebView。
- 后端：继续调用 `chat.automations.*`，由既有 Host、Cindy Scheduler、项目配置适配和脚本执行器处理；没有另写调度引擎。
- 来源：Cindy `2d8d3c3605ee9073613dcd4069bc59f799be61ae` 的 SchedulerPage、TaskListPane、RunHistoryPane/Card、ScheduleFormDialog、TemplateGallery、删除保护及表单/cron 语义。对应来源与 SHA-256 登记为 `third_party/cindy/native-ports.json` 的 `chat-automations`。
- `chat-automations` 的旧 pending 已在实际 Host/原生窗口核对后清除；18 个模块和9份规格的固定2d strict检查通过，没有直接accept到新的main。
- 最新 `ui-final-fresh/automations-5/` 通过：真正Pi绑定owner执行终态、未配置通知保留/禁用、原通知失败history、删除排除手工owner、恢复严格paused且保留历史/错误/通知/绑定、真实保存/继续编辑/放弃保护。默认选中任务在560窄窗的详情点击原失败已红→绿，不重复读取历史或改变owner。
- 原模板/项目精确preview/configHash批准的 `TestRealHostNativeTemplatesAndExactProjectImport` 未受本次选择修复影响，保留实际执行证据，不以界面字段存在代替。

## 早期功能对照（历史待验状态；当前结果见上文）

| 上游行为 | 原生实现 | 证据 / 当前状态 |
| --- | --- | --- |
| 按状态/最近执行排序，筛选与项目分组 | `model.go`、`list.go`；暂停在后、一次任务不单独沉底 | 原生单元与视图测试；中英/深浅 1000/560 渲染 |
| 项目组折叠、列表调宽与筛选记忆 | `list.go`；保存于 `desktop-automations-view.json` | 已实现；持久化与手动拖动尚需实窗复核 |
| Host 事件刷新；隐藏页不持续查询 | `page.go`、`events.go` | 项目预览事件不重复触发 pendingImports，避免读→发布→再读循环的回归测试 |
| 运行历史、按持续会话分组、未读状态、费用与检查输出 | `history.go`、`page.go` | 已实现并原生渲染；长历史与费用组合仍需全矩阵核验 |
| 新建、编辑、立即运行、暂停/恢复、删除/恢复 | `page.go`、`history.go` | 真实命名管道下创建/保存失败/删除保护回归；后端执行测试通过 |
| 每次新建、持续会话、绑定已有会话 | `form.go` | 已实现；绑定不可用与归档会话的完整交互仍需核验 |
| Cron 常用频率、时间/星期/月日；相对间隔；手动；指定日期 | `cron.go`、`form.go` | 可逆 Cron 预设测试；旧特殊表达式保留为自定义，不擅自重写；间隔与过期 onceAt 的只改名称回归 |
| 模型、供应商、思考档位、快速模式 | 共享 `widgets.ModelField` / `modules/nativeui/modelmenu` | 档位取供应商的 efforts/levels，未提供的旧值保留；后端去掉固定档位白名单并验证 `balanced` 保存/执行 |
| 脚本执行与能力授权 | `form.go`；能力目录来自后端原版 SCRIPT_CAPABILITIES | 已实现；具体连接器与脚本运行仍需真实 Host 逐项验收 |
| 前置检查脚本、超时、自测、AI 生成自测 | `form.go` | 原生表单已有，继续使用现有 Host 生成/自测接口 |
| 插件前置检查 | 真实 Cindy Ghost 核心、Host 桥、原生共享选择/管理入口 | 核心已接通：真实插件 allow/skip/block 与真实 Scheduler 无空任务/无模型调用/通知不重复验证通过。完整本地 SDK、设置页和产品组合验收仍未完成，见 [续接核查](chat-onebot-audit.md) |
| 桌面/飞书/企业微信通知与静默 | `form.go` | 字段已接入；渠道可用性、配置跳转和实际投递需补齐核验 |
| 模板目录、参数、应用、保存和删除 | `templates.go` | 必填参数验证；应用不能覆盖未保存草稿；原生表单与后端模板接口测试 |
| 项目配置转入/复制/移出与变更确认 | `page.go`、`templates.go` | 按精确 configHash 确认；需真实 Host + 实窗验证；项目转换保留 preRunPlugin |
| 删除时保留/归档/删除生成会话 | `model.go`、`page.go` | 手动绑定的原会话排除，删除保护测试；部分失败仅重试剩余会话，不重新删除自动化 |
| 未保存保护、失败保留草稿、旧权限/未知字段保留 | `Draft.patch`、原生 `kit.Dialog`、`Chat.leaveAutomation` | 新增 Chat 侧栏真实点击/输入回归；只发送已改字段，不设自动化权限选择器 |
| 运行任务跳转 | `Options.OpenSession` → `Env.Open` → 原生 Chat | 已接线；来源/草稿与运行历史跳转待实窗组合验收 |

## 验证记录

- 统一入口：`build/test-native-ui.ps1 -Modules desktop-mygo`；本轮现有 MyGo 全套通过。
- 原生模块局部入口：`build/test-native-ui.ps1 -Modules desktop-mygo -Packages ./chat/automations`。
- Chat 集成：`TestAutomationsOpenNativelyFromTheChatSidebar`，在没有 WebView 服务的测试窗口中渲染真实自动化列表与历史，输入表单形成草稿，切回 Chat 经过未保存确认。
- 后端构建和运行：`build/build-chat.ps1 -SkipInstall -Test ...`，Chat/自动化 93 通过，OneBot 119 通过，暂停恢复 2 通过，`internal/chat` 通过；日志 `_temp/cindy-sync-2d8d3c3/chat-build-native-automation.txt`。
- 原生渲染图：`_temp/cindy-sync-2d8d3c3/native-automation-shots/`。这些是生产 MyGo 组件的 ui.Tester 渲染，不是 Windows 实窗或生产账号验收。
- 当前最终构建、许可、正常退出和5863文件隔离安装矩阵通过；Git及唯一固定安装器的实际交付以 `dist/release/release-info.json` 为准，不自动更新真实安装。

## 原生引擎接入结果（取代早期选型记录）

终端已直接复用 MyGo 官方 `plugins/terminal` 与原 ConPTY 所有权，实际 VT DLL、manifest 和主题摘要按 `third_party/native-terminal.json` 校验；MyGo 0.8 更新未改变这组摘要，不重复下载。

PDF 已采用原生 PDFium，保留文本/查找/链接/表单与文档级权限；音视频复用原生有声播放器而非 MyGo 的无声视频组件或“打开系统应用”。原生工作面板与浏览器批注已有隔离实窗证据。当前 v0.8.1 的 PDF 并发关闭/父上下文取消真实回归已通过，只由 CommandContext 终止进程，不提前关闭 stdin 制造双终止竞态；完整 native 测试和普通/生产 vet、EXE与许可通过。资源/原始许可均在独立native清单，源码只在仓库管理副本准备。

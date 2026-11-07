# Chat / 自动化 / OneBot 原生迁移续接核查

更新：2026-11-07。原生主体、本地 SDK、邮件/browser桥、真实前台手势/媒体 ACK、typed COM/官方 App 和最终候选构建/退出/隔离安装检查已通过；Git及固定安装器最后交付以 [本次验收](../2026-11-07-native-local-delivery.md) 和 `dist/release/release-info.json` 为准。用户明确准许本次先交付已验收功能，300ms启动目标仍未达标且保留后续优化。

最新能力对照见 [本地插件平台](local-plugins.md)。本文件下方 2026-11-05 的过程记录保留用于追溯；其中旧“targets/setModel/消息/媒体台账/卡片/MCP/邮件缺接线”状态已由本轮原实现和实际回归取代，不作为再次重写的依据。证据根为 `_temp/plugin-task-close/resume-20261106/`：当前专用 SDK **139/0、无跳过**；真实 mail provider **10/0**、browser bridge **4/0**、原生邮件三组合；rich 原生窗口 **216 项**。真实生产账号/物理硬件仍未自动实测。

用户新增的设置保存、闪聊首帧和提示关闭均在本轮先处理、后交付：配置投影均值 **69.948→16.147 ms**、实际 Host 保存/回读 **158.434→39.453 ms**（隔离同负载）；首帧不再依赖前帧 Bounds；关闭只收起反馈，不清草稿或权限。最终候选实窗/进程及安装回归完成后才更新正式产物。

## 已实现，不重复改写

- Chat、设置、自动化主体、工作面板和 OneBot/Bot 模式已有 MyGo 原生实现；MyGo 生产构建不再承载 React。真正的网页内容和插件自有页面使用隔离 WebView。
- 工作面板包含原生文件库、编辑器、审查、终端、PDF 与音视频；OneBot 的原生设置、私聊、群聊、话题、语音链及后台接线保留。
- `cindy-ui`、Mermaid 与 MathJax 公式实现已接入；本轮发现的公式缺陷见下文。实现存在不代表整个功能矩阵验收结束。
- 本地插件平台复用 Cindy `2d8d3c3` 的实际管理、协议、回执与 slot；完整本地适配和真实行为/原生组合对照见本轮记录，云端/远程/非Pi仍为明确排除范围。

## 本轮完成的修复与证据

| 修复 | 验证 |
| --- | --- |
| 安装启动探针认识新的只读 `chat.ghosts.pending`，仍拒绝安装/审批/执行 | `app/probe_windows_test.go`；`build-resume-final.log` 中真实 `internal/startupprobe` 通过 |
| 插件 UI 关闭后停止轮询、移除窗口注册，过期请求从界面撤下；重复答案不阻塞 Host | `chat/pluginhost/pluginhost_test.go`，受影响原生包通过 |
| 插件设置点击返回时的同帧空指针崩溃 | 实窗栈确认根因；`chat/pluginchecks/navigation_test.go`；实窗回到列表并打开设置页复验 |
| BroadcastChannel 排除发送者回声，避免重复投递和插件自激循环 | 原版页面 shim 的失败→修复测试；真实独立逻辑进程/面板通信回归及截图 |
| 停用插件真正等待其 Node 工作进程退出，重新启用可重新调用 | 真实 `.cindy` 包、真实 Bun Node 工作进程测试；无 mock 退出回执 |
| 模型选择器使用正确上游 `request/resolve`，补齐 `effort/fastMode`，核对供应商和模型 | 真实插件请求测试；原生页面共用 `widgets.ModelField`，不再自建模型列表 |
| 插件安装权限说明在手动/Agent 安装路径共用完整插值与未知能力保护 | `pluginchecks.PermissionDetails` / `PermissionsKnown` 与原生包测试 |
| 插件公网请求使用实际 Cindy/OpenClaw SSRF 单跳实现、固定 DNS 结果并在发包前重验权限 | `ghost/outbound.test.mjs`；同期保留原始 MIT/Apache 许可与完整来源摘要 |
| Host 给 Runner 传 `[]byte` 导致参数变 Base64、误报插件不存在 | `chat_ghosts_windows_test.go` 真实 Go→Bun 回归；实窗完成 Pi 权限批准→插件自身确认→工具结果→原生卡片 |
| 群聊 PromptPanel 没有运行完整 Chat.View，因而未打开被等待的会话 | `TestPromptPanelOpensSpeakerTaskAndAnswersItsPermission`，从实际控制器打开并向正确会话回复 |
| 异步行内公式完成后 Markdown 仍缓存旧 TeX | MyGo 原版段落缓存增加显式 revision 回调，保留文档身份/复制选区；`third_party/mygo-winone/inline_math_test.go`；来源与许可记录同步 |
| MathJax 默认拆分行内 SVG，原适配只取第一个，漏掉等号和后续项 | 关闭浏览器式行内拆分；6 项公式测试通过。已重建真实 worker，实窗确认 `E=mc²`、`α+β` 完整显示（中文浅色、英文深色窄窗），不再保留“未实窗”状态 |

实窗证据在 `_temp/cindy-sync-2d8d3c3/e2e/shots-latest/`：

- `plugin-install-confirm.png`、`plugin-installed.png`：真实原生文件选择器与安装确认/回执。
- `plugin-settings-physical.png`：插件自有设置页读取了自己的 `/kv`。
- `plugin-panel-physical.png`：面板收到独立逻辑进程的 `PANEL` 回答。
- `plugin-confirm-current.png`、`plugin-call-complete.png`、`plugin-card-visible.png`：真实 Host/Pi 调用、两级许可和原生卡片。测试模型只用于产生固定请求，不冒充生产账号验收。
- `formula-inline-fixed-light.png`、`formula-inline-fixed-dark-en.png`：中/英、浅/深和窄窗；这些截图同时暴露了最后的行内多项公式缺字，不能当作其最终修复截图。

## 字体许可与隔离安装续验（已完成，不重做）

- **保留 NewCM 4.1.3**，没有换字体或重写公式。原包虽然漏带 LICENSE，但全部 105 个 WOFF2 的 OpenType name 记录保留了 Antonis Tsolomitis 的原始版权和 GUST 声明，部分另带 MathJax 2024 版权。`build/record-math-font-notices.py` 可重现抽取；证据 JSON 使用固定 LF，避免 Git 检出后哈希变化。
- 已保存原始 GUST 1.0、其引用的 LPPL 1.3c、MathJax 同版本项目 Apache 许可（提交 `0bfa580685ca72e4a3a37657ccd754a65c1a42c6`）及原包声明；明确区分包代码与底层字体许可，不用任意 Apache 文本替代字体授权。完整未修改 npm 字体源归档随包分发，含来源、变更/使用说明和 SHA-256、npm lock SHA-512。
- `third_party/chat-math-notices.json` 独立于历史分享清单；`build/verify-chat-math-notices.mjs` 核对原始许可、105 份元数据、源归档和精确运行清单，并按真实 Bun metafile 核对全部 3 个实际依赖（286 模块），不把未打包的菜单/语音依赖算入公式 worker。空许可、未解决审查、错误摘要、遗漏运行资源继续阻断构建。
- 源码许可回归 **6 项**、公式 **6 项**、Chat/Pi **94 项**、OneBot **120 项**、暂停恢复 **2 项**、`internal/chat`、`internal/distribution` 全包和分发 vet 通过；真实 Ghost 专用入口 **20 项通过、0 跳过**。普通 Chat 构建跳过的真实沙箱用例由该专用入口覆盖，不把跳过当通过。
- 沿用已完成编译的候选，只重建受影响 Chat 和 Host；`build/verify-built-bundle.ps1 -RunStartupProbes` 续验通过。清理的是已确认与 AI runtime 字节相同的旧候选重复 Photon 文件，移到 `_temp/font-install-close/old-build-files/`，未扩大运行清单放行重复文件。
- 最终候选包 **5,840 文件**；**16 次隔离进程操作**通过：两处失败回滚、中断安装拒启/错误摘要拒绝/修复失败保留/成功修复、正常覆盖、历史恢复日志保留、自包含残缺安装修复、两处卸载中断恢复，以及独立正常首次安装/卸载。逐文件哈希、测试注册表、快捷方式、配置及额外文件保留均核对；测试登记、快捷方式和进程已清理。
- 安装包载荷、维护/更新/首次提示帮助程序、测试安装器及导出卸载器的 Defender 静态扫描均明确未检出。未修改安全设置、真实安装、真实配置或真实 ProgramData。

本轮证据统一位于 `_temp/font-install-close/`：

- `chat-build-fixed.log`、`distribution-host.log`、`ghost-system.log`：构建及测试。
- `final-install-build.log`、`final-installer-matrix.log`：最终源码许可/启动/包校验、安装编译与矩阵。
- `install-final/recovery-matrix-result.json`、`install-final/result.json`：恢复矩阵和正常首次安装/卸载回执。
- `installer-final/security-payload/result.json`、`installer-final/security-installers/result.json`：当前测试产物的扫描记录。
- `shots/formula-final-zh-light.png`（1606×1052）、`shots/formula-final-en-dark-narrow.png`（798×818）：实窗公式最终效果，无黑边。实窗关闭后 UI 进程退出；本次测试剩余 Host/子进程按隔离路径确认后强制清理，不冒充新增的正常托盘退出证据。
- `installer-final/WinOne-0.0.1-windows-x64-test-setup.exe` 仅供隔离测试，SHA-256 `af034877933c26161dc9a69bbbd68436e118dd39235bec9f10c07650d0822ef4`；不是正式交付包。未签名、未覆盖干净机器/真实账号与硬件，也不代表所有界面交互已完成。

## 当前验证状态

- `fullbuild-resume2.log`：Chat/Pi、OneBot、Math/插件的默认测试、Go 主模块、Rust/结果窗等阶段通过；WinTray 的 `TestDCBackedLaunchPreservesNativeTrayRestore/class-dc/manual` 曾返回 `started_only`。**未把这次统一命令说成通过。**
- 从失败阶段续接 `build-wintray.ps1`，WinTray 全模块及真实 Worker 回归通过；然后 MyGo 全模块、普通/生产 vet、构建和真实安装启动探针通过。证据 `build-resume-final.log`。
- 先前的最终包失败已解决：原始字体许可与源归档补全后才增加独立 Math 运行清单；当前完整候选已通过清单、哈希和隔离安装校验。历史失败日志保留，不冒称此前统一构建命令通过。
- `onebot-resume-full/runtime.log`：120 项通过。较早 Chat 构建中市场导入曾出现一次 `IMPORT_ITEM_FAILED`，随后单项和整套复验通过；原因未定，不假称修复。
- `close-resume/tests.log`：7 种隔离实窗生命周期场景通过，覆盖旧常驻字段、退出、未保存、进行中及慢释放；关闭到进程结束记录为 11–29 ms。
- 插件真实沙箱、Node、MCP、跨页通信、卡片动作、模型选择、SSRF 与关闭测试通过的具体范围见 `build/test-native-ghost-checks.ps1`；没有用这些测试推断未实现 API 可用。

## 持续执行中的新增进展（不作为结束条件）

完整执行顺序与最新过程证据：`_temp/plugin-task-close/plan.md`。已继续接入原版任务服务/收据库、真实目标投递、伙伴交互与路由、持久媒体授权/配额、批准技能快照；实际沙箱→Host→Pi的任务及项目/普通目标投递通过。还修复了技能快照 BigInt IPC 与合法大媒体被 1 MiB 传输上限截断的问题。这里是进度记录，尚未逐项验收的配置、协作/委派、完整卡片、模型媒体能力等必须继续实施，不能据此结束工作。

## 最后验收核销（2026-11-07）

1. **原生页面最终组合通过**：四边/独窗、固定/自适应高度、重开位置尺寸/停用释放/单多实例；真实焦点/OS鼠标/F8/3秒过期/外链Gate，以及准确媒体账本→预览→用户确认→原附件/草稿合并→ACK均有实际终态。前台组看门狗30秒，已返回原桌面。证据 `ui-final-fresh/`。
2. **COM/PDF与当前构建通过**：typed QueryInterface/Callback/vtable、checkptr=2、普通/生产vet、真实Frame2/Frame7与官方App通过；PDF只由CommandContext终止，真实并发取消/Close回归通过。完整native包以有界测试并发复验，保持原退出预算。证据 `mcp-com/`、`desktop-final-bounded-081.log`、`bundle-final-081-startup-verified.log`。
3. **候选与交付检查**：当前5863文件隔离安装/回滚/修复/覆盖/自包含残缺修复/卸载恢复及配置保护通过；完整进程资源、UI正常退出、Defender保持实时保护下静态扫描通过。Git/正式安装器同步最后执行，结果以 [本次验收](../2026-11-07-native-local-delivery.md) 和固定release-info为准。21次新进程重开805ms P95未达300ms，用户明确准许作为后续优化，不删预算或伪称通过。

旧“targets/参数/伙伴/routine/台账/事件/卡片/MCP/邮件还未接通”的清单已由本轮原服务及真实测试取代；逐能力及准确边界见 [本地插件平台](local-plugins.md)，不再按旧清单重复实现。真实账号、实体麦克风/输入法/挂起等外部条件未被夹具冒称通过。

## 上游追踪

MyGo 本轮升级并核对最新正式版 **v0.8.1**（原始提交/许可与源码适配已记录），Pi **1.5.0**。Cindy 导入基线仍为 `2d8d3c3605ee9073613dcd4069bc59f799be61ae`，不是宣称最新 main 全量同步。

当前18个原生模块、9份同步规格；SDK339文件、原生panel/MCP/mail16项显示源已补登记。固定 `--strict --ref 2d8d3c3605ee9073613dcd4069bc59f799be61ae` 检查通过。默认main报告只观察后续变化，不自动扩大升级范围；旧两个pending已按真实终态核销。

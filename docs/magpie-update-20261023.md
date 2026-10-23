# AI 基座与插件浏览器续接（2026-10-23）

本次 Magpie 锁定 50f52b540b915bdf5912ddebc48c1fa46eef3025，社区插件锁定 652f06dc4d5bea1472be05eaa4fe1381c03bf109（37 包）。来源、原始许可证、依赖摘要和运行清单已同步；不在构建时自动追随 HEAD。下表覆盖从 bac5be0 起的全部 27 个提交。

## 本次增量对照

| Magpie 提交与变化 | AI 服务所属功能及接入位置 | 验证 |
| --- | --- | --- |
| 65e8040、3dc25a6：插件请求测试按测试实例计数 | 上游 gateway 测试夹具，生产行为不变 | 保留完整源码 |
| a1cce7f：意图判断超时可调 | 高级设置 → 模型设置；MagpieSettings.classifyTimeout → 上游 ClassifyWait | 保存重开、0/1/23/60 秒适配器、上游 classifier/decision 回归 |
| be002ac、19f9a80、da6fd87、e6bed2f、eee9ece：Kimi 合作方、官网名称、图标与赞助排版 | 自定义供应商目录；accounts.catalog 读取 PartnersNow，保留官网、密钥入口、语言、介绍和赞助标识；图标走既有本地头像通道 | 来源许可校验、动态目录缓存回归；README 网站排版不嵌入 WinOne |
| 68fb6b6、56418ac：Command Code 0.1.15、DeepSeek off 档位 | 内置账号插件、模型与网关思考设置 | 37 包清单验证、TestOffAsNone、TestAskedAtOff |
| 97902e9：启动时更新合作方、每小时刷新 | 账号目录按需启动短进程，等待上游目录；设置窗口目录缓存一小时后重取 | TestPartnersFetchedOnStart、前端缓存过期回归 |
| 85534cd：Codex 历史线程迁回 | Magpie 外部客户端管理，不属于 WinOne 账号、模型或路由入口；保持既有不修改外部客户端的边界 | 上游 TestCodexThreadsComeBackOffMagpie 与线程迁移回归；不自动操作用户 Codex |
| db48697、c8c6e7e：弹窗分段按钮保持位置、测量时机 | 插件与路由复用资源使用最新 app.js；WinOne 自有表单沿用统一 Dialog | 生产构建与设置页回归 |
| 673651e、0c679c6：Codex 外部应用重启、DeepSeek Hermes 识别 | 上游外部客户端管理；不新增控制外部程序的 WinOne 入口 | 上游 CodexAppReopen、DshHermesFound/Windows/Wired 夹具回归 |
| 34ad18e：用量会话行 helper 迁移 | 用量统计适配器使用最新上游资源与会话数据 | 用量前端和 Go 测试 |
| 44d8c74：局域网共享指南路径 | 上游文档修正；WinOne 仍使用本地网关既有入口 | 无运行时变化 |
| 2435944：JSON 转义后字段脱敏保持有效 JSON | 高级隐私与共享隐私服务、gateway | TestMaskJSONKeepsArgumentsJSON、ArgumentsEscapes、RestoreArgumentsFromArguments、ArgumentsCutShort |
| 8ef9fc5、3b8bcc3：ChatGPT 重发移除 status、提前过期时刷新一次 | 账号供应商和实际模型请求，直接编译上游 provider/gateway | CodexInputDropsStatusWithID、Expired401/UsageExpired 及非过期 401 回归 |
| 0815c91：Claude 文字形式的工具调用转为真实调用 | 账号供应商、网关、Chat 的统一 AI 请求 | 六项 ClaudeTextToolCalls/Prompt 回归 |
| 45fa299、a77b5b4：清晨已签到结果重查、手动签到显示具体结果 | 账号页增加立即签到，按保存的账号范围调用；自动维护走 soon=false 的到期判断；显示领取、已签到、无活动、资格、验证等结果 | WorkBuddy/PluginCheckinEarlyDoneAskedAgain；本地 keyless 插件真实签到；前端按钮与结果 |
| e96d577：隐私记录卡重绘 | 高级设置 → 隐私 → 脱敏记录，完整 masking.html 资源 | 构建确认 106 条共享样式适配；原始记录能力保留 |
| bf2e067：Codex 额度边缘仍有 credits 时再试 | 网关与账号额度路由 | CodexCreditsAskedAgainAtTheEdge/NoLongRest/FirstOfTheSpent、routes 回归 |
| 50f52b5：跨请求已发送文本保持相同脱敏以保留缓存 | 共享隐私服务、默认 AI 和网关 | TestSentTextKeepsItsMasking、TestMaskedConversationKeepsItsCachedPrefix |

路由页仅承载路由；供应商、账号、默认设置、网关、用量和隐私仍在各自入口。外部客户端管理维持原有产品边界，表中明确列出，未当作已加入 WinOne 的功能。

## 插件浏览器修复

作者 magpie-chatgptweb2api 默认是有界面的 keep-open Chromium，连接使用本机 CDP。Magpie 没有替所有第三方浏览器隐藏窗口的通用实现。原 WinOne 每秒隐藏窗口存在闪现间隙。

现在登录仍在当前桌面可见。授权完成后，仅对本次登录 Job 内的浏览器发送正常关闭消息，等待档案保存。账号读取与模型请求在独立 Windows desktop 启动有界面的 Chromium；不需要把专用浏览器放在前台，也不关闭 Chromium 沙箱。新桌面复制当前桌面的 DACL 与完整性标签，以兼容 Chromium AppContainer 网络进程。

后台引导进程先等 Host 发送首个请求，确保子进程继承已分配的 Job。Windows 回调用整数编号查找状态，避免 Go 对象指针经整数往返。生成、模型读取、网关及维护均同步账号后端，Host 退出回收保留的浏览器。

普通权限最终 AI 程序已通过作者插件与本地网站的登录、冷请求、连续回复、Host 生命周期重启、流式取消、后台不可见和退出无残留。Windows 管理员提权请求被取消，本轮管理员复测未完成。真实供应商账号、反机器人验证与真实模型响应未验证。

测试期间整个 AI、Bun 与 Chromium 进程组峰值 18 个进程，私有提交内存约 1.08 GiB。该值是本地网站测试的峰值，不是空闲常驻或真实供应商预算；Windows 工作集求和会重复计算共享页。记录见 browser-final-normal-verified-resources.json。

## 当前证据

所有本轮过程证据位于 _temp/resume-plugin-gesture-20261023/：

- upstream-tests.log：27 提交中的 40 个新增测试入口通过（含子用例 61 项）。
- ai-ui-fixed.log：账号与设置页定向 28 项通过。
- final-build.log：前端全量 157 文件、862 项通过；首次构建在 Go vet 指针检查处中止，该处已修正。
- plugin-methods-fixed.log：37 包账号方式与本地无密钥签到通过；无密钥方式不再被旧测试错误地要求输入个人密钥。
- browser-final-normal-verified.log、资源 JSON：最终 AI 程序普通权限集成通过。
- stroke-native.log：真实低级鼠标钩子 → SendInput → 自有窗口往返通过。
- gesture-ui.log、gesture-ui/layout：中英、深浅色、520/1000 px、保存重开和录制通过；已查看代表截图。

安装器交付结果在完成隔离安装验证后记录到 development.md；本节不把测试包或构建过程当作交付完成。

## 先前轮次记录（bac5be0）

Magpie 更新至 bac5be067744f1bcf5609d5bb790f41c5dc5c078，社区插件更新至 421117ef7504bfd62de44c95924fbabb53a03e47，Pi 相关包为 1.4.0（4a080252f06caf63a525d7873c5d94d3c23887e0）。这三项是本轮锁定的构建输入；后续构建不得自动改为上游 HEAD。

## 功能对照与验收

| 上游能力 | WinOne 接入位置 | 本轮状态 |
| --- | --- | --- |
| 脱敏词忽略大小写；小数与 UUID 防误匹配 | modules/ai-magpie/settings_test.go、高级隐私和上游 masking | 适配器测试通过 |
| 无密钥登录、单身份账号、启用账号的模型合并、Qoder 当日再次签到、Qoder mover 最低版本 0.2.31 | 37 个账号插件、插件登录状态机和完整 registry | 构建时 registry 校验通过；本地模拟登录普通与管理员通过 |
| 跨 Magpie 进程的登录续期互斥 | 上游 plugin renewal lock | TestRenewOnceAcrossHosts 与失效锁检测通过 |
| 路由最后请求高度恢复、请求表末尾滚动条、请求详情收起、账号名称 | 完整 routing client、请求服务和 WinOne 路由页 | 上游详情回归 TestRoutingRequestOpensItsBodies、编译与前端测试通过 |
| 用量按查看者时区、请求速度与缺失原因、账号展开、首字耗时、账号改名 | 完整 usage 页、Host 时区参数和 usage.Zone | 上游 GUI 时区路由、Host 日期筛选、热力图和会话费用跨午夜回归通过 |
| 思考参数、schema、流式拒绝、Ark 月额度、慢首字休息、Lite 压缩、fake-IP、预算去重 | 原始 gateway、budget、netproxy 与默认 AI 和本地网关 | 构建及既有适配器测试通过 |
| 上下文覆盖与已下线模型的输出上限保存 | 自定义供应商和模型元数据保存流程 | 既有回归通过 |
| GitHub 大备份分块 | 保留 WinOne 备份边界，不引入 Magpie 外壳、外部客户端、Library 或云备份产品 | 边界保留 |
| Pi 检查点、工具恢复、压缩、Code Mode 与 Windows shell | 官方 Pi 1.4、checkpointWhen 与迁移备份 | 81 项 Chat 测试、2 项暂停恢复测试及 Go Chat 集成通过 |

## 插件运行环境

用户选择按 Magpie 使用真实 Windows 用户目录。只有插件子进程恢复 USERPROFILE；WinOne 的 XDG 配置、认证记录、APPDATA 和 LOCALAPPDATA 浏览器档案继续使用私有目录。第三方插件因此可能读取 Windows 用户目录中的外部客户端账号，这是已接受的兼容边界。

专用浏览器登录使用最终构建的 WinOne.AI.exe 在普通权限与管理员权限分别冷启动验证。测试使用全新复制的 Chromium 和本地模拟网站，确认等待授权时窗口可见、授权后转入后台、新 Worker 读取到账号记录并返回中文流式回复；未运行真实供应商账号登录。

## 验证证据

- 构建：_temp/continue-elevated-20261023/final-build-third.log
- 最终载荷：_temp/continue-elevated-20261023/final/build
- 普通登录：browser-final-normal-gated.log 与对应 result.json
- 管理员登录：browser-final-elevated-gated.log 与对应 result.json
- 上游增量回归：_temp/continue-elevated-20261023/upstream-regression/

## 交付状态

统一构建、包校验、正式三件套扫描和隔离安装矩阵均通过。固定安装器已更新为 dist/release/WinOne-setup.exe，详见 [开发状态](development.md) 的 2026-10-23 条目。未提交、未推送、未签名、未发布，也未安装到用户真实环境。

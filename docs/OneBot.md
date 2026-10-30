# OneBot：Cindy 本地伙伴能力接入

## 2026-10-30 最终接入状态

固定 Cindy `b7e46bcbbe41eeee2f16381fef00df61a31a3b8b` 的本地伙伴能力已完成 WinOne 接入并进入固定安装器。原版页面、群聊/分工、六类 IM 的本地队列与主人来源、市场/导入、语音、学习和工作台均复用上游实现；WinOne 只保留 Host、Rust 方法表、AI 和本地文件/配置适配。完整构建与许可证证据见 `_temp/deliver-20261030/` 和 `docs/development.md`。

最终未覆盖范围只有需要人工账号或实体设备的验收：真实 IM 登录收发、实体麦克风、Windows UAC 提权，以及真实供应商账号、物理输入法和长期稳定性。没有把隔离模拟回路当成这些项目的人工验收。

## 范围与来源

用户于 2026-10-26 指定：OneBot 的导航入口和页面标识使用官方 Lucide `Panda`（`panda`）图标。

用户于 2026-10-25 确定：Cindy「伙伴」改名 **OneBot**，独立入口最终放在 Chat 上方；仅使用 Pi。剔除 Cindy 云端伙伴、设备互联/远程主机、手机端、Chat Server 群组/邀请、官方 X 与账单/账号服务。**保留个人微信、Telegram、飞书/Lark、企业微信、钉钉和 Discord**。开发过程不使用子代理。

来源固定为 Cindy `b7e46bcbbe41eeee2f16381fef00df61a31a3b8b`，不因恢复任务不断追逐新 HEAD。Apache-2.0 原始 LICENSE/NOTICE 位于 `third_party/cindy/`。`build/sync-cindy.mjs` 根据 `third_party/cindy/onebot.sync.json` 机械同步，宿主适配单独存放；上游实现不手改。依赖全文及摘要由 `third_party/onebot-notices.json` 与 `build/verify-onebot-notices.mjs` 校验。

## 最新续接状态（2026-10-29）

原始合并清单与本轮断点见 `_temp/finish-all-20261029/plan.md`；历史表中较早的“仍待接”应结合本节核对，不重复已经实现的工作。

- 自主跟进、工作台/待办工具、逐轮学习复核、工作记忆、回复学习回执和 `@伙伴` 已有实现。此次全 Host 真实回路补验了学习辅助调用；其测试夹具此前与对话共用回复 FIFO，现用独立辅助模型保持真实执行并避免争用，未关闭学习。
- 本机群聊原版 Worker 服务已接入 Go 专线/附件/项目目录/专线关闭与 Rust 方法表。新增 4 项 Host 检查，真实 Pi/Magpie 回路通过文件工具、附件、草稿保留、同输入重试与删除归档。附件重复导入导致回执内容冲突的问题已先失败后修复，重开保留首次字节与原名。
- **群聊尚未全量完成**：原版页面、分工工作树及产品实窗未接齐；`onebotStaged.tsx` 仍存在，不能生成正式交付包。完整个人微信/持久保护消息、市场/导入、语音、生成草稿/图像及原合并任务验收继续保留。
- 证据为 `_temp/finish-all-20261029/`：Chat 90 + OneBot 106 + 暂停恢复 2 通过；Host 全套默认检查/vet 通过；Rust 86 通过/49 显式忽略；来源声明复验通过。未提交、推送或更新固定安装器，未修改真实安装/配置。

## 能力对照与剩余范围

以下是接入状态，不是产品已全量完成的声明。基础服务通过测试不代表界面、执行器或真实渠道已连接。

| 上游能力 | WinOne 接入点 / 依赖 | 当前状态 |
| --- | --- | --- |
| 自动化页面、模板、项目配置、前置检查、历史与未读 | `features/cindy/scheduler/`、`modules/chat-pi/automations.mjs`；见 `AUTOMATIONS.md` | 主体已在前序完成；本轮只因共享构建执行回归，不重新实现 |
| 伙伴档案与版本 | `onebot/profile-store.mjs` 适配 SQLite；复用 `botProfileVersioning`、能力选择、模型路线、默认身份语义 | 本地创建/读取/更新、版本冲突、冻结版本、Home 写入恢复日志已接通；头像/颜色持久化及原版自动保存接线已测，完整页面、图片头像和创建引导待接 |
| 伙伴 Home、SOUL、USER、提示词附加内容 | `onebot/content-service.mjs` → 上游 `botProfileFolder` | 真实读写、外部编辑对账、原文空白保留、路径检查已验证；文件不能修改权限配置 |
| 记忆、索引、全文检索、设置页编辑冲突与导入 | 同一 `MakerMemoryStore`、FTS、`botMemoryService` | 本地接口、真实 Bun SQLite、Pi 记忆工具及原版记忆页组件已接通；已测搜索/删除/编辑冲突/保存失败草稿。压缩记忆和完整产品导航仍待接 |
| 自有技能读写、启停、完整源文件、检索分页与按需挂载 | 上游 `botSkillStore`、`botSkillQueryIndex`、`botSkillRuntimeProjection` | 文件服务、挂载投影及会话绑定的 Pi 读写工具已接通；完整页面与安全边界全流程待接 |
| 技能/MCP/工具集选择 | 原版 `BotCapabilitySettings`、Host 能力目录及 Pi 资源加载器 | 能力目录、原版选择组件、加载限制及模型失效后的资源浏览已接；外部技能须明确加入，个人技能独立保留。已验证旧工具/旧回合结束后排队输入切换模型、身份和技能，重复通知合并，刷新失败不使用旧配置发出后续请求；完整模型链及产品入口继续接入 |
| 待办、来源去重、版本保护、行动回执 | 上游 `botTodoStore` 与 `teammateTodo` | 读写/去重/回执已接通；执行待办仍待绑定真实任务。回执成功不伪装为事项已完成 |
| Pi 主会话、隐藏渠道专线、话题/历史、模型路线切换 | `botProfileRuntime`、canonical session、现有 Chat/Pi service | 主会话、原版三层身份提示词、受控工具及模型链顺序切换已在真实回路验证。切换沿用 Pi 重试，保留工具结果，Host 记录实际路线并在重开时保留；后台 Pi 子任务参数隔离已测。原版模型设置组件及共用选择器已完成隔离保存/双语主题布局验证；完整产品导航、渠道专线管理和独立伙伴任务的真实执行仍待接 |
| 独立伙伴任务、任务专用模型与完成回执 | 原版 `botDelegationService`、`onebot/task-host.mjs`、`task-client.mjs`、普通 Chat 任务登记 | 正式 Worker → Host → 普通 Chat/Pi 已接通，按执行身份结算并回传原伙伴；原生时间线元数据持久保存。真实回路已测指定任务模型、写文件、等待期间草稿保留、协调进程重开接回原执行及重复结果不重发。**完整任务卡渲染、任务控制/代答、工作树、原始用户请求与隐藏续接的呈现适配、未受理任务/整机重开恢复仍待完成**；不等同于 Pi 子代理 |
| 生命周期与失败恢复 | 上游暂停、恢复、同主会话重启、删除；归档是删除内部步骤 | Host → Worker → Pi 已验证暂停/恢复/重启、删除后历史和草稿保留、旧授权撤销及 Home 原生回收；清理失败持久回执/重开重试与原版页面组件已测，产品实窗流程继续 |
| 关联项目 | 原版 `botLinkedProjectAccess`、`workbench.json`、`BotProjectsSettings` 与 Pi 项目工具 | 原版页面组件、Host 登记目录边界、关联/移除/失效目录保留、变更通知及主会话 Pi 工具已接通；8 组浏览器场景和真实 Host→Pi 回路通过。关联不增加任务责任或工具权限，完整产品导航仍待接 |
| 例行任务、自主跟进与任务工作台 | Cindy `RoutineEngine`/`RoutineFileStore`、WinOne 现有 Scheduler、Host canonical Pi 派发；`onebot/routines.mjs` | 伙伴例行任务已保存、重开、暂停/恢复、手动执行、一次性触发、执行前检查、执行历史、取消和删除；执行固定当前 canonical，不接受页面传入另一会话。自主跟进、待办巡检与任务工作台仍待接，不能把普通例行任务当成自主接管 |
| 夜间学习与学习复核 | Cindy `botNightlyLearning*`、`botNightlyLearningCounter`、`nightlyLearningRoutine`；`onebot/nightly-learning.mjs`、`learning-host/data/scan` | 夜间学习已接入 Host AI 辅助模型、频率/开关、主人轮次计数、自动输入排除、重开去重、原文证据校验、记忆/Skill 落盘及逐条撤销/恢复；原版学习页面已通过真实原生操作。学习复核、工作记忆和 IM 主人来源投影仍待接 |
| IM 六类渠道 | Cindy 的个人微信/iLink、Telegram、Feishu/Lark、WeCom、DingTalk、Discord 及逐轮主人授权/专线归属 | Worker 已接五类连接适配、加密 KV、私有 Host 派发与 Pi 回合；文字/卡片确认和逐轮授权回执持久恢复已做隔离测试。真实公网账号、个人微信业务编排、持久队列/受保护内容和完整管理流程仍待完成 |
| 技能/伙伴预设市场与本地包 | 上游 bundled packages、市场页面、本地导入/导出 | **待接**；不得用空页面替代 |
| Hermes/OpenClaw 导入与自动化接管 | 上游 bot-import、companionImport；已接入的文档导入可作为底层存储 | **完整流程待接**；单独 memory.importDocument 不代表导入功能完成 |
| 语音输入、输出和会话操作 | 上游 bots/voice 与 WinOne 的共享媒体能力 | **待接** |
| OneBot 导航、列表、会话和完整设置界面 | 原版 bots renderer + 本地宿主适配；遵循 WinOne 双语/主题/未保存保护 | 原版页面壳、伙伴列表、会话、设置抽屉、例行任务和夜间学习页面已接入；真实原生检查覆盖中英、深浅色、320/520/1000 宽度、保存、手动执行、撤销/恢复及无横向滚动。群聊入口仍由 `onebotStaged.tsx` 占位，不能视为群聊可用 |
| 整体交付 | 全流程、资源/退出、隔离安装矩阵、全工作树提交推送、固定安装器 | **未完成**：群聊、完整个人微信/iLink 编排、市场/导入、语音、自主跟进/学习复核及其真实公网/安装验收仍未完成；不得发布当前中间状态 |

## 当前宿主链路

```text
桌面 Chat("onebot.<method>", params)
  → Rust/Wails 明确方法表与修改保护
  → Go chatOneBot（按需创建，复用 Windows Job）
  → onebot.js（私有 JSON-lines 协议）
  → runtime.mjs（单目录单写者、档案写入恢复）
  → profile-store.mjs / content-service.mjs
  → Cindy 原始 Home / Memory / Skill / Todo 服务
```

- 数据根只能由 Host 的私有 `init` 指定，位于所选 WinOne 数据目录的 `onebot/`；无真实用户目录默认值，也不读取 Cindy 的用户数据。
- `profiles.db` 保存档案、版本和待写 Home 内容；`bots/<id>/` 保存用户可见内容。`config.json` 即使被外部修改，也不成为宿主权限来源。
- 单目录独占采用 SQLite 的 OS 锁，而非可复用 PID。内存中的 SQLite 实例按需创建，空闲实例预算为 8，不限制伙伴或记忆数量。关闭先排空已接受请求，再关闭数据库和后台进程。
- Cindy FTS 使用 `prepare/transaction` 接口；Bun 宿主通过缓存 `query` 适配 `prepare`，确保关闭时立即释放语句与 Windows FTS/WAL 文件句柄，不依赖 GC。
- 索引/技能生成目录拒绝已有 junction/symlink 和硬链接文件。它不是抵御外部程序并发改目录的 OS 沙箱；Pi 侧最终仍必须仅授予 workspace 原始文件写入，Home 内容通过类型化工具管理。
- `init`、云端连接和待办派发回执不在桌面方法表里。Pi 的记忆/技能工具通过单会话私有桥绑定 Bot id，不能在参数中选择另一位伙伴；系统 digest 类型不开放给模型。渠道逐轮主人授权、保护消息的临时会话和完整工具集仍待接入，不能直接暴露全部桌面方法。
- `runtime-refresh-requested` 由 Host 通知既有 Worker，在当前回合及后台任务结束的边界应用；Durable 保留工具、输入回执和队列，资源应用回执返回后才更新 Host 元数据。空闲通知和发送共用一个刷新任务。主会话已通过 `session.open` 创建并持久绑定；尚无正式 OneBot 页面，未做这一路的实窗流程验证。
- `im-service.mjs` 已由 worker 初始化；Host 传入 DPAPI 保护的 master key 和私有派发桥，init 后恢复保存的渠道。现有隔离回路与原版 SDK 测试不代表真实公网账号、个人微信或完整 IM 产品流程已经验收。

## 构建与验证入口

```powershell
. ./build/use-local-resources.ps1
node build/test-onebot.mjs --bun $WinOneBuildParameters.BunExe --output _temp/<任务>/onebot-tests
./build/build-chat.ps1 -BunExe $WinOneBuildParameters.BunExe -GoExe $WinOneBuildParameters.GoExe -CargoExe $WinOneBuildParameters.CargoExe -OutputDirectory _temp/<任务>/chat -SkipInstall -Test
```

`--runtime-only` 只跑 Bun 接入测试；不带此参数同时执行已同步的上游 Vitest 测试。`WINONE_ONEBOT_WORKER` 可指定待验收的 bundle，不指定时测源码 worker。测试输出目录必须是新的、Git 忽略的 `_temp/<任务>/`，HOME/TEMP/TMP 均隔离。

正式 Chat 构建生成 `onebot.js`、记录 `runtime.json.onebotSHA256`，并打包全部 OneBot 许可。`chat_onebot_runtime_paths.txt` 与许可清单即时互校；任何 OneBot 资源出现都要求完整载荷，但旧已验证包仍允许没有 OneBot，保留回滚兼容。

### 独立任务真实回路续接（2026-10-27）

- `task-client.mjs` 通过私有桥调用 Host；Host 用 `AdmitTaskConversation` 在普通 Chat 登记协调器生成的 ID、模型、权限和原伙伴归属。重试不覆盖用户后改的标题、归档及草稿，不另建执行器。
- Pi `winone.timeline` 是纯呈现记录，不进入模型上下文。已修复首次用户消息前自定义记录不落盘的问题：通过 Pi 公共 `setSessionFile` API 初始化独占空文件，不伪造消息。重复时间线 ID 内容冲突时拒绝写入。
- `input_settled` 取自 Pi Durable 的逐输入提交及精确 answer entry；它与会话整体 `settled` 分开。排队输入仍运行时，前一输入也能返回自己的终态，后续用户回合不能替换独立任务结果。共享自动化读取器同步支持这一回执。
- 已受理执行用 `instanceId + generation + requestId` 持久定位。回执读取及落库失败仅重试该回执，不重发模型输入；OneBot 协调进程重开后被动接回已受理任务，已完成但未送达的结果复用原版 `resumeCompletionDelivery`。这不是未受理任务、整机重开或完整任务控制已验收的声明。
- 证据：`_temp/onebot-native-task-resume/`。`final-runtime/` 为上游 **932 通过 / 4 条件跳过**及宿主 **67 通过**；`build-4.log` 为普通 Chat/自动化 **89 通过**、暂停恢复 **2 通过**及 `internal/chat` 通过；最后自动化接收器补齐后重新构建，`full-host-2/` 的 Host 默认全套（含真实 Worker）通过。Go vet 与 `internal/distribution` 通过。来源仍固定 Cindy `b7e46bcb`，211 后端文件、36 任务文件与 92 项依赖声明校验通过。
- 首轮全 Host 检查的两项 AI 失败使用了归档/媒体功能前的 `ai-network-final/`；按前序交接改用 `ai-media-policy/` 后通过，未修改对应生产代码或断言。自动化结果读取器的回执遗漏则已实际修复并复验。
- 可复用真实 Host 测试入口新增 `build/test-onebot-host.ps1`，显式传入 Bun、Chat bundle、AI 可执行文件和隔离输出目录。脚本保留 Go 依赖缓存路径，隔离 HOME/USERPROFILE/TEMP/TMP，并恢复进程环境。
- 尚未新增完整产品导航或原版任务卡，不把纯时间线元数据当作已完成界面。未执行本阶段产品实窗、完整资源与安装矩阵；未提交/推送、未更新固定安装器、未修改真实安装与配置。继续按上表和合并清单完成后统一收尾。

### 2026-10-26 已有证据

- 上游基础：7 个 Vitest 文件，111 通过、4 跳过（原有平台/符号链接条件）。`_temp/cindy-onebot-20261025/host-runtime-1/result.json`。
- 当前 Bun 接入：19 通过，含真实 SQLite、中文检索、编辑冲突、精确导入、技能资源保留、自动发现待办归属、目录链接防护、独占/重开、协议错误码及 EOF 排空。`_temp/cindy-onebot-20261025/final-runtime/`。
- 共享 Chat 构建及 89 项回归 + 2 项暂停恢复通过；`internal/chat` 进程测试通过。最后 OneBot bundle 为 `host-bundle-final/onebot.js`。
- Go `apps/host` 与 `internal/distribution` 全套默认测试和 vet 通过；最后 bundle 的 Host 真进程调用/退出/数据库重命名复验通过。全套 JSON 证据在 `host-checks-2/go-tests.jsonl`；需要真实账号/系统交互的测试仍按原条件跳过。
- Rust `component_bridge::tests::onebot_content_routes_preserve_close_guards_and_refuse_private_controls` 通过。43 个 OneBot 同步文件、13 项 npm 依赖通知及现有 9 imports/303 adaptations 校验通过。
- 首次 Go 测试唯一失败来自“普通目录”夹具位于仓库 `_temp` 内，Git 向上找到了 WinOne；夹具增加无效仓库发现边界，保留非仓库断言后复验通过，未改生产探测语义。

## 依赖声明沿用 Cindy（2026-10-26 用户裁决）

用户在企业微信 SDK 缺独立 LICENSE 的问题上明确要求「直接按照 Cindy 的方式做」。已核对锁定版本的 `scripts/generate-third-party-notices.mjs`、`forge-third-party-notices.ts` 与 `docs/legal/notices/desktop-win.txt`：Cindy 将这些条目放在 `Packages without a standalone license file (license per package.json)` 附录。

WinOne 保留相同依赖，不另换协议实现或删除渠道。`@wecom/aibot-node-sdk@1.0.7`、`@sapphire/async-queue@1.5.5`、`@sapphire/snowflake@3.5.5` 保存包内原始 package.json/README 和缺少独立全文的事实、Cindy 原文件 SHA-256/提交及声明行，不伪造许可证。只有名称/版本/许可与锁定 Cindy 附录逐项匹配才采用此声明路径；其他包继续保存原始全文。`dingtalk-stream` 原包写 `v2.1.5`、npm 锁写 `2.1.5`，两者分别记录并校验，未改包源码。

### 本次续接验证（2026-10-26）

- 原版基础 + IM/iLink：62 个 Vitest 文件，**900 通过、4 条件跳过**；`im-upstream-tests/result.json`。
- 当前宿主接入：**23 项 Bun 测试通过**，包括真实 Pi 接收三层身份提示词、经批准写记忆/技能；加密 KV、连接记录/失败回滚、模拟传输器的真实派发结果及同 lane 保序；`im-host-tests/`。模拟传输器测试不是公网账号测试。
- `pi-im-candidate/` Chat 构建及随包许可验证通过，普通 Chat 回归 89 项、暂停恢复 2 项、internal/chat 进程测试通过；最新 OneBot bundle 约 0.44 MB，尚未包含未接入的 IM 连接适配器。
- Go OneBot 5 项通过：方法边界、共享日志、主会话持久身份/普通 Chat 隔离、DPAPI、真实 worker 退出。相关 vet 通过。Rust OneBot bridge 测试、前端 TypeScript 和差异检查通过。
- 同期其他目录迁移工作的临时编译缺项随后已补齐；本任务未覆盖或回滚那部分代码。

以上**没有**覆盖 OneBot 实窗、IM 实际派发/登录、语音、完整导入、安装矩阵、长时资源基准。没有提交/推送，也没有替换 `dist/release/WinOne-setup.exe`；应按原要求完成余项后再统一收尾，不能将本次续接视为已满足「直到更新安装器」。

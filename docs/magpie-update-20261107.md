# 2026-11-07 AI 服务升级与原生功能对照

## 来源和交付边界

- Magpie：`22f5cf69e583dab7723523be514cbafc9c870ebd` → **`c39d36644742a10a4730f0ffc5a03d877f185541`**（本任务查询时最新 main，494 个提交）。固定该提交，不在长任务中追逐移动 HEAD。
- 社区插件：`1ed2f5544e19718511dec039e6a6e3805d82dd8b` → **`4200fed0993b1fbbe144b7135724e70f5cfd3205`**。登记 79 个可随包分发插件和 14 个需上游实际安装流程的包；内置账号后端不进入可卸载清单。
- 当前组合使用另一个任务已升级的 **MyGo v0.8.3**；Pi 全套 **1.5.0**（已核对最新正式版）。本任务保留启动任务的已有/未提交工作，不把未验证启动实验自动称为发布结果。
- Go 依赖声明未变化；新 checkout 执行 `go mod download all` 恢复完整校验锁后，与 `modules/ai-magpie/upstream.go.mod/sum` 一致。新版 LICENSE / THIRD_PARTY_NOTICES 保存原文和摘要，并由源码级门禁核对。
- 原生管理页调用同一 Host 管理、能力地址鉴权的 Magpie 路由/插件/隐私运行时，不自研路由、账号、搜索或定价。第三方作者自带网页/面板仍通过 MyGo `ui.WebView` 在原 scripts-only sandbox 和声明权限边界内显示，不用近似原生页面删除作者能力。
- **用户最新交付安排**：本轮先提交、推送代码；另一位 AI 合并各对话改动并统一更新正式安装器。本任务不修改固定正式包。其他对话的未提交启动实验和测试文件留在原工作区，未纳入本轮提交。

## 上游增量 → AI 服务功能 → 验收

| 变化 | WinOne 接入位置 | 当前证据 |
| --- | --- | --- |
| 模型发现、账号池/配额休息、Cline 不再写拒绝的 providerOptions、ZCode Trial 单模型休息、Copilot 缓存计费 | 同一新版 provider/gateway；供应商、账号、模型、本地网关 | 适配器/OAuth/账号快照通过；336 个 provider 增量测试通过 |
| HTTP/1.1、DeepSeek 图片文件、原样错误、优先使用供应商、明确全部模型选择 | `internal/ai/provider_preferences.go`、`preferences.go`、共享 worker DTO | 原生高级选项完整接线；实际 worker 解码使用 upstream Provider；普通配置保存/回读和字段语义测试通过 |
| 跨源请求密钥保护、取消不算失败、原错误类型、thinking/工具协议、搜索结果数量和并行合并 | 同一 gateway、Host 请求路径、`magpie_settings.go`、原生高级设置 | 实际请求/组执行、适配器、搜索字段、取消统计回归通过；大批量网关增量复验记录见下节，不冒称最初失败的整批通过 |
| SearchCount、RetryAfterCap、RouterAllTools、AgentCompacts、CodexSubagentLead、提前重置 3–1440 分钟 | `advanced_latest.go` 与现有额度/模型/搜索/客户端设置 | 原词/单位/默认语义、草稿不自动保存、保存回读测试通过；所有模型下拉仍走共享 modelmenu |
| 路由组与请求记录分开、模板/导入/分享、规则/成员/账号、策略/亲和、实时证据 | `routing*.go`，运行时原 groups/trace/context/recent-call handlers | 实际创建→保存→请求执行→trace/body 读取通过；账号完整 ID、默认权重不被重写、删除成员/规则联动、未保存保护、宽窄布局回归通过 |
| 字节流量/图片文件节省、缓存命中率、取消状态、5/15/30 分钟刷新 | `pages/aiservices/usage`，原用量 ledger/query/定价 | 原生用量套件通过；缓存率复用原 `cacheHitOf`，取消标记由 upstream `Record.Canceled()` 输出，不自猜状态 |
| 插件退出/RPC 取消、跨源请求保护、新版社区包与原生管理 | 实际 plugin/middleware runtime；`plugins_native.go`、`plugins_details.go` | 本地夹具安装→选项保存→启停→回读→README→卸载通过；包源码/运行资源/许可验证通过 |
| 脱敏规则/记录/预览/原值生命周期 | `masking_records.go`，原 masking/maskrec | 实际敏感词被替换且命中清单非空、记录读取和清除通过；查看原值显式请求、关闭/换运行时/清除时取消旧读取并忘记原值 |
| Magpie 自身平台窗口、外部客户端安装和外部会话库接管 | 不属于 WinOne AI 服务的独立入口 | 保留既有私有 home、显式安装和不接管真实外部配置的边界；不把独立 Magpie 应用塞进路由 |

## 路由原生功能核对

来源：`internal/provider/group.go`、`grouprule.go`、`grouptemplate.go`、`groupmatch.go`，`internal/gui/groups.go`、`trace.go`、`context.go`、`providers.go`、`assets/plugins/routing/client.js`。

- 总路由开关走 WinOne 保存事务；关闭 groups 内置功能时给出具体说明和原生一键启用；保留第三方替换页和扩展面板。
- 自动发现/排除供应商/隐藏和恢复自动组；动态完整模板；导入/覆盖、分享、复制、重命名、启停、单/批量删除与排序。
- 成员/嵌套组/图片模型/分类器选择；模型模式和实际匹配数量。预览调用上游 `CleanPatterns`/`PatternHits`，不另写匹配器，不保存草稿。
- 成员排序/启停/权重/快速模式；固定/子集思考档位使用模型实际提供的原词；改标识时同步权重、规则、手动选择和顺序，删除成员时清理失效引用。
- 全部上游策略及已注册 JS 策略；Smart/顺序/轮询/用量/周配速/免费/速度/价格/权重/用完再换/到期免费/手动；保留亲和、限流后置、首内容等待、耐心、窗口/对外档位、Family、仅经组列出模型和限定账号。
- 账号 ID 使用 `/api/groups` 返回的完整 ID，不重复加供应商前缀。
- 有序规则保留上游全部字段：优先/后续/暂停、输出及最低思考档位、Token/图片/压缩、意图/分类器可信度、客户端、目录/排除目录（选择和拖入）、时间/星期/时区。
- 请求/历史/日期与筛选、会话或网关密钥分组、全部展开/收起、刷新/暂停；完整候选/尝试、规则/亲和/嵌套、配额/休息/失败/Token/耗时/流量/缓存/模型来源证据；解除休息、供应商验证链接、原请求/回复/脱敏头、归档、上下文与跳转脱敏记录。
- 供应商账号/密钥池保留实际 route/affinity/sink 操作。原生编辑与统一保存/撤销、导航草稿保护接通；后台刷新不覆盖草稿；缺失必须字段的成功 HTTP 回执不能假装保存成功。

## 插件原生功能核对

来源：`internal/gui/plugins.go`、`assets/plugins.js`、`pluginpage.go`、`pagebridge.js`、实际 `internal/plugin`。

- 已安装清单/状态/具体错误、版本/Git 提交/新版本/自动更新回执、Bun 状态；同时检查 Git heads 和 npm 更新。
- 推荐/npm 搜索/包信息（发布者、许可、版本、下载量、主页/仓库）、GitHub topic 未审查仓库与 README。先显示上游清单，再读取 npm 元数据，不杜撰未知版本。
- npm/Git URL/本地目录安装，原生选择/拖入；单个/全部更新、重新读取、启停和卸载确认。
- 镜像与 registry 使用同一 WinOne 草稿/保存事务；插件 JSON 对象选项、上游示例、错误/保存/撤销与离开保护。
- 供应商/账号连接、冲突与优先选择、工具/MCP、网关/中间件/智能体状态、页面/面板声明读取范围、菜单动作、原作者页面及上下文保留。
- 内置后端不可由第三方卸载操作删除；不隐式安装别的 CLI，不读不属 WinOne 的账号。
- 作者页面继续执行原 SDK：真实 scripts-only frame 内的 `onAction` → 允许的 `usage:summary` read → options.set 已通过。页面原生选择目录/账号桥也保留。

## 脱敏原生功能核对

- 1–30 天汇总，类型/识别来源/客户端/供应商/拦截筛选、分页和详情。
- 每个命中的类型/动作/提示/长度/占位符；原始值仅在显式请求且仍在运行时内存时显示，失效明确说明。
- 已发送正文及可恢复原值；显示单个值不自动改变“已发送正文”的显示模式。
- 64 KiB 预览使用已保存设置，显示实际命中；预览不存入记录。
- 清除确认、跳转设置/对类型恢复脱敏走同一草稿。清除的正向回执必须验证；取消旧详情读取，避免旧结果重新出现。

## 本次验证入口和证据

- `build/build-ai.ps1 -Test`：适配器、OAuth、账号元数据/环境/注册表隔离，当前产物记录 `ai-build-complete.log`。
- `build/test-native-ui.ps1`：Chat/AI 服务/用量/首页/划词/共享控件/外壳/i18n 当前回归通过。
- `build/test-ai-native-windows.ps1`：真实 MyGo v0.8.3 窗口，中文/英文、深浅色、1000/560 宽；内置路由/插件/隐私页面不创建 WebView；作者 SDK 在实际 WebView2 中执行成功。证据 `native-windows-content/`。原生根截图不合成独立 WebView 子窗内容，作者网页另用 WebView 的实际 CapturePage 保存为 `third-party-sdk-content.png`，不把根图里的黑占位当成网页视觉证据。
- `internal/ai` / `apps/host` 正式回归：原字段/单位、单次划词警告/恢复、旧请求不能覆盖较新成功、故障不被操作成功吞掉、统一日志去重。
- 证据根为 `_temp/20261107-chat-ai-native/`，不进 Git；正式测试源码入库。

## 上游测试的实际限制与处理

1. 初次账号请求 404 根因不是模型缺失：新版 `Find`/定价等内部读取改走 `sharedAll`，旧 `All` 接入点被绕过。构建将私有供应商快照接到该共用点，适配器复验通过，未增加错误回退。
2. 旧的 WinOne cgroup `filepath.Dir` 补丁与新版 `path.Dir` 遍历不兼容，造成 Windows 测试永不结束。删除该旧替换，保留当前上游实现。
3. 850 个 gateway 增量测试的五分钟进程预算不足，改用显式十五分钟**测试运行器**预算，产品的等待/重试阈值与测试断言不变。
4. 符号链接测试普通权限失败，经用户明确允许只为仓库隔离测试请求一次 UAC 后，`TestSweepBridgeProjectsTakesOnlyTheBridgesFolders` 通过。没有更改系统开发者模式或真实目录。
5. 原 `TestSearchCountSetting` 等夹具明确采用“无宿主”假插件，但全局强制 Bun 会启动不存在的 npm spec 并占用夹具目录。该单例按原无宿主环境复验通过；整批按无宿主夹具语义复验，真实 Bun 行为单独由实际插件/SDK/适配器及对应有界测试验证，不隐藏最初失败日志。
6. 首轮 GUI/外部会话/proc 增量中的 Windows 路径字面值、外部会话分配预算、系统监听读取限制仍在原日志记录，不冒称这些无关外部客户端/平台案例全部通过。真实供应商账号联网、外部客户端配置和用户实际安装没有被安全夹具冒称验证。

## 代码交付与供统一打包复用的产物

- `build/build.ps1 -SkipInstall` 当前完整构建、普通/生产 vet、源码/产物许可、整包启动/配置/分发检查通过，日志 `unified-build.log`，退出记录为 0。
- `build/package.ps1` 校验并生成 6053 文件候选 ZIP。当前工作区该候选含另一对话仍未提交的启动实验，不能把它冒称本轮独立提交的最终安装器。
- 本轮提交范围（基于已提交 MyGo v0.8.3、不含未提交启动实验）在 `commit-snapshot/` 单独重建来源/cache，复用原项目必要生成资源；受影响原生/真后台测试、生产 EXE、普通/生产 vet 和实际链接许可通过，记录 `commit-scope-native.log`、`commit-scope-build.log`。
- 来源清单中的 MyGo 摘要对应其已提交 `source.json`，不会因本轮提交把其他对话未提交的启动补丁带入；另一任务合并后须按原入口同步摘要再重建、验收安装器。
- 网关正确无宿主环境复验成功；原浏览器/Unix 等条件 skips 不计入已通过，真实 Bun 账号并发用例和获准符号链接用例有各自通过记录。没有安装、关闭或修改用户真实 WinOne 配置。
- 正式安装器、匹配卸载器、SHA-256 与发布元数据由用户指定的另一位 AI 统一维护。本轮留存产物与证据入口见 `dist/reports/chat-ai-native-20261107/README.md`。

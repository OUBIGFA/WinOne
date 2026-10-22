# Magpie 2026-10-19 更新对照

## 来源与范围

- Magpie：`d783df46cbac9021e26a6bd71ca0b55544552eb2` → `c3b2e6109ba9b01ab56b7fe1ce0946ff45ef1c7b`（2026-10-19 查询的最新提交，67 提交、186 个变化文件）。
- 社区插件：`0a998a8a7fea95161a3d13d03f3b4594cf6e2fad` → `ff13aac87dfe1c86b7915badef42cb9046588059`。
- Magpie `go.mod`、`go.sum`、原始 MIT LICENSE 无变化；社区根 LICENSE 无变化，变化包声明 MIT，无新增运行时依赖。来源/原文摘要与产物仍须按现有许可入口核对。
- 更新按 AI 服务的对应功能实施；路由只载入路由插件。WinOne Chat 仍由 Pi 执行，不导入 Magpie 独立应用的客户端配置、外部会话管理和主窗口。
- 下表是实施前建立的完整增量清单，不是完成声明。2026-10-17、2026-10-18 清单的未验证范围仍保留，不能用新版可构建替代实际验收。

## 完整增量 → 功能 → 接入与验证

| 上游提交 | 变化 | AI 服务所属功能与接入位置 | 本轮验证 |
| --- | --- | --- | --- |
| `ccb2eea`、`41f5107` | Zed 模型读取失败说明、优先有套餐的组织 | 账号供应商；完整 Zed 0.1.10 插件、账号目录与资源锁 | 待验 |
| `cc8e8e3` | Grok 无邮箱登录兼容 | 账号供应商；完整 Grok 0.1.17 插件 | 待验 |
| `47eb42f` | WorkBuddy 积分倍率解析 | 账号/模型/用量；WorkBuddy 0.1.25 与上游账号接口 | 待验 |
| `5b267fb` | Command Code Zero Data Retention 登录选项 | 账号供应商；Command Code 0.1.14、现有动态登录问题/输入通道 | 待验 |
| `c3b2e61` | 插件登录成功即移除过期标记，包括空供应商列表 | 账号供应商；上游 provider/pluginsignin，WinOne 私有账号资料 | 待验 |
| `f76201a` | CLI 已打开的授权页不重复打开 | 账号供应商；上游 `SignInState.Opened` → Host `aiSignInState` → `AIAccounts` 自动打开判断；手动重开保留 | 待验 |
| `33dbdcc` | 插件页登录动作主动定位登录区域 | 插件管理 → 账号供应商；新版 `plugins.js`，`plugin-host.js` 交给 WinOne 页面导航与 `ai-account-connection` | 待验 |
| `65f5f01` | 授权输入与提交按钮等高、8px 间距 | 账号供应商；现有 WinOne Input/Button 与登录表单，嵌入页同步最新 app.css | 待验 |
| `c483fea` | OpenCode Zen 解释 API 不提供余额并打开控制台；Go `/inference/go` 套餐识别 | 用量统计/供应商；上游 balance/planquota，`accounts.quota` 原样返回；WinOne 额度区补 `seeAt` 控制台入口与双语说明 | 待验 |
| `11173c8` | Computer Use 长会话每张截图只拟合一次 | 本地网关/默认服务；上游 gateway/codex_images，经 `gateway.go`、`stream.go` | 待验 |
| `ec7f474` | Codex→Copilot 保留 web_search，转译工具参数流式输出 | 本地网关/默认服务；上游 responses/custom_tool，经相同网关执行 | 待验 |
| `24f89cd` | 回复文本用 builder 累积，避免逐 delta 全量复制 | 默认服务/网关；上游 gateway/ir | 待验 |
| `938c314`、`ec6fba4` | 大请求 JSON 校验一次、顶层成员拆分一次 | 默认服务/网关；上游 gateway/fastjson | 待验 |
| `2a27fb8` | 搜索能力检查不复制大请求；原生搜索供应商不重复解析 | 默认服务/高级搜索/网关；上游 gateway/search | 待验 |
| `fc78eb5` | ClinePass 实际供应商从 routing.finalProvider 读取 | 网关/用量；上游 gateway/upstream | 待验 |
| `9491910` | 静默 HTTP/2 连接探活并断开 | 默认服务/网关；上游 netproxy 与 gateway/upstream_transport | 待验 |
| `129d8b0` | Kimi 工具 JSON Schema 的 `$ref` 目标补类型 | 默认服务/网关；上游 gateway/kimi_schema | 待验 |
| `a111818` | 会话亲和结果异步合并落盘，不逐请求写入 | 路由/网关；上游 gateway/affinity，仍使用 WinOne 私有运行目录 | 待验 |
| `a595f15`、`627e8e7` | 路由组成员目录在请求 Hold 内只构建一次 | 路由/模型；上游 provider/group、hold，经 `routing_provider.go.txt` | 待验 |
| `ed3a46d`、`224ea7d` | 目录短时复用；目录重置失效旧快照 | 模型/路由/供应商；上游 provider/hold、catalog | 待验 |
| `a20d869`、`ae019de` | 实际服务记录合并写入；测试同步写入防跨测试残留 | 供应商/路由/用量；上游 provider/served | 待验 |
| `8fd74d1`、`e1ebd17` | 退役账号文件每秒检查；Hold 内缺失文件也记忆 | 账号/供应商/路由；上游 provider/retired、filememo | 待验 |
| `9b72add` | Codex zstd 保留两个编码器，避免按 CPU 核数放大内存 | 账号供应商/默认服务；上游 provider/codex_zstd | 待验 |
| `e3c09f4` | 每账号 provider info 构建一次 | 账号/插件/网关；完整上游 plugin host 与 WinOne bundle 适配 | 待验 |
| `8f37ed5` | plugin-auth.json 按文件版本缓存 | 账号/插件；上游 plugin/host.js，私有账号目录不变 | 待验 |
| `ef75a9a`、`5774c79` | 流式插件 chunk 零额外字节复制、固定格式解析 | 插件/默认服务/网关；完整上游 plugin host Go/JS | 待验 |
| `db61143` | 用量/路由缓存只保留需要的字符串 | 用量统计/路由；上游 usage/request_snapshot，WinOne 元数据账本 | 待验 |
| `e7114d3`、`43bd998`、`34b1757` | 网关会话投影共享、按所需天数读取、切片独立容量 | 用量统计；上游 usage/gateway_sessions，经 `usage.go`；不读取外部会话 | 待验 |
| `6359b2c`、`2fcd4b3` | 单一 Vias 索引回答不同 since，按需复制会话列表 | 用量统计；上游 usage/vias_cache，经 `usage.go` | 待验 |
| `227b864` | Codex effort 只留可匹配的网关调用 | 用量底层；上游 usage/codex_identity；WinOne 禁止外部客户端会话发现的边界保留 | 待验 |
| `da9fc17` | Pi/omp 会话调用记录增量读取 | 用量底层；上游 sessions/calls_pi；WinOne 自己登记的 Pi 会话与主 Chat 不被替换 | 待验 |
| `9893ae8`、`2b2d5fd` | Hermes 单会话变更只重新解析该会话；路径每列表解析一次 | Magpie 外部会话管理，不是 WinOne AI 服务入口；源码保留，不扫描用户外部资料 | 边界回归待验 |
| `38f6bee`、`81029fc` | Manage 只读可见智能体文件；概览统计与会话共用一次扫描 | 同上；WinOne 不载入 Magpie Sessions/Manage 应用页 | 边界回归待验 |
| `d9f6b42`、`b9b4c21`、`e03d495`、`b3642e7`、`1f41180` | Grok、Droid、Kimi、dsh 增量读取与列表缓存 | 同上；源代码保留，私有 AI 用量读取仍固定 SourceGateway | 边界回归待验 |
| `6d3caf6`、`11b42c0`、`ec396be`、`a77f6ae` | Cline manifest 不重读；Kimi/dsh wire 增量；WorkBuddy 零费用开头合并为一次 | 同上；不扩展外部资料读取权限 | 边界回归待验 |
| `c381e4c`、`d7e02ae`、`baef679` | 会话读取不被另一读取刷新阻塞；读缓冲按剩余文件大小、至多 1MiB | 会话读取底层；上游 sessions，WinOne 私有用量接口边界不变 | 边界回归待验 |
| `cfbd321`、`865b618` | 启动预热 Codex effort；网关会话列表不附多余逐日信息 | Magpie 独立 GUI/Sessions；不导入上游启动预热或外部扫描 | 边界回归待验 |
| `95ef45f` | curl 安装的 OpenCode 更新使用已展示版本 | Magpie 外部客户端更新，不是 AI 服务执行；禁止替用户安装/更新客户端 | 不触发外部更新 |
| `1a3b926`、`eb3f174` | Claude Code 主/副模型上下文独立；auto mode classifier 模型配置 | Magpie 外部 Claude Code 客户端配置，不是 WinOne Pi Chat 权限/模型；不写外部客户端配置 | 不改变 Pi 或用户客户端 |
| `ab7ec8c` | Manage agents 名称与图标相邻 | Magpie 外部客户端管理；WinOne 账号选择已有对应紧凑布局，纳入全局布局检查 | 待验 |
| `9fd80b2` | Library 技能支持任意 Git 仓库（含 GitLab） | Magpie 外部资源库，不是 WinOne AI 服务插件安装器；源代码保留，不擅自替换 Pi 技能发现/安装流程 | 不扩大安装范围 |
| `3608ed7`、`2ceff73` | 独立 WebView2 参数传递；焦点/可见性恢复合并读取 | Magpie 应用外壳，不嵌入 WinOne 路由；WinOne 使用自己的宿主与活动页读取生命周期 | 边界回归待验 |
| `ae7dc10` | Docker 镜像基础更新 | 上游部署工具，不属于 Windows WinOne 产物 | 无需应用到 Windows 安装包 |

## 社区包完整增量

除上表账号包外，完整同步 Autoclaw 0.2.2、Cline 0.1.9、Factory 0.1.34、Raccoon 0.1.1。仓库还新增 `middleware-block-patterns` 0.1.1，但上游 `registry.json` 尚未收录，因此不擅自加入默认分发；第三方插件页仍支持通过显式来源安装。所有包的原始入口、选项、页面与资源通过 `build/update-ai-plugins.mjs` 同步到资源锁、运行目录清单及适配器目录；不手工裁剪文件。

## 验证记录

实施中。测试证据写入 `_temp/20261019-stability-ui-recording/`；未更新正式安装器，不操作真实安装或账号配置。

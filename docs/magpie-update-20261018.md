# Magpie 2026-10-18 更新对照

本次固定来源：`7660399a121af67db4efd56ecb6bd4fb04dc53f1` → `d783df46cbac9021e26a6bd71ca0b55544552eb2`（2026-10-18 查询的远端最新提交，共 63 个提交）。社区插件固定至 `0a998a8a7fea95161a3d13d03f3b4594cf6e2fad`，31 → 32 包。

MIT 许可证、Magpie `go.mod/go.sum` 与前版相同；社区包均声明 MIT、没有新增运行时依赖。所有文件/版本通过现有准备入口校验，第三方插件仍须用户显式安装或启用，不借更新自动启用未选择的中间件。

此表是接入与验证清单，不是全量完成声明。此前增量的未验证部分继续见 [2026-10-17 对照](magpie-update-20261017.md)。AI 服务各功能分别接入，路由仍不装载 Magpie 整应用。Chat 使用 Pi，外部客户端的配置、会话发现和迁移不写入用户真实目录。

## 完整增量到功能的映射

| 上游提交及变化 | WinOne 对应功能 / 接入位置 | 验证状态 |
| --- | --- | --- |
| `9f9bfc1` Codex 模型上下文改为账号实际 272K；`8233ead` 任意账号插件 priority 档位支持快速模式 | 账号、模型；`modules/ai-magpie/models.go`、`settings.go`；保留账号模型元数据 | 新来源适配器基线通过；专项待验 |
| `5b12407` Responses 自定义工具兼容；`7ea8aea` DSML 多竖线工具解析；`e183604` Claude 点号工具名恢复 | 默认 AI 服务、网关和实际对话；上游 gateway/claude，经 `gateway.go`、`stream.go` | 源码随新版接入；专项待验 |
| `bf931f8`、`45b8c72` 原生压缩会话只在启用时固定账号，无法服务时给出原因；`717b69f` 路由说明区分原生压缩固定 | 默认服务 / 路由；上游 gateway 和抽取路由客户端 | 新来源适配器基线通过；专项待验 |
| `f550d36` Claude 的 SOCKS 代理转 HTTP；`992f288` 同时拒绝工具/思考的 Chat 中继故障切换；`72ed518` ChatGPT GPT Image 不受错误账号目录拒绝；`6690caa` 自动审核选择实际执行账号 | 供应商、网关；上游 provider/gateway/claude | 源码随新版接入；真实账号与代理网络未测 |
| `2042123` 暂停供应商仍保留自动组；`1e643ba` 模型显示名加入自动组 | 路由 / 模型；上游 provider + `routing_provider.go.txt` | 专项待验 |
| `6b55010` Jev 分类携带前一轮上下文；`48aae83` 上下文按分类器自身权限脱敏 | 路由 / 隐私；上游 gateway | 专项待验 |
| `0eab029` Codex zstd 窗口限为 2 MiB；`b14784c` 有额度的大窗口账号耗尽后尝试小窗口；`7ac2024` 成功大回复不再经过失败正文扫描；`b36819d` 已净化长串不重复扫描 | 网关 / 性能 / 隐私；上游 provider/gateway/redact | 专项待验 |
| `d432920` DeepSeek Search API | 高级设置 → 模型设置 → 搜索 API；动态目录已有，补 `internal/ai/magpie_settings.go` 的保存校验 | 待验证保存、密钥加密和实际受控请求 |
| `5a2414a` 新安装内置能力默认全启用 | 默认/高级功能设置；`builtin_plugins.go` 已以 All 为缺省，显式 Essentials/禁用继续保留 | 基线通过；缺省及旧选择专项待验 |
| `e44b9ae` 每个候选供应商独立 `onProvider` 中间件 | 插件和网关；上游 middleware/gwmw；WinOne 外层路由与内层 transport 不重复运行请求钩子 | 待验证一次调用及不同候选间隔离 |
| `b5c7693` Factory 0.1.33；`be36009` Qoder 0.2.18；`c4aeaec` Cursor 0.2.19 | 账号供应商 / 随包插件；`plugin-catalog.json`、`third_party/ai-plugins.json`、分发路径清单一起生成 | 版本锁已更新，运行回归待验 |
| `0f1666b` 退役账号统一插件化；`21252f6` Zed、`615732a` Kiro、`e405315` Cursor、`761556a` Qoder、`b7ceb53` WorkBuddy、`30fcbf6` MiMo、`09b9ae8` ZCode、`6da19bf` Factory、`2927a16` Grok、`07a445f` Devin、`be62bcf` Command Code；`b98f906` 保留迁移前账号；`e431f71` 删除退役专用路径；`de67171` 失败迁移 10 分钟后再试；`0718beb` 模型获取不等待无关 CLI | 账号供应商；WinOne 私有账号资料与已验证插件后端；`accounts.go`、`plugins.go`、`account_snapshot.go` | 不自动迁移外部软件账号；私有资料兼容、登录形态与旧账号专项待验 |
| `b9f790d` P5 四种插件面板、语言化标题、主题继承、页面接替；`d783df4` P6 供应商/智能体菜单动作、晚注册处理 | 插件、供应商、账号、路由、用量、高级设置；`plugin-host.js`、`prepare-ai-plugin-page.mjs`、WinOne 页面插槽 | 接入与权限/主题/故障隔离验证中 |
| `8e44c2f` 插件更新入口说明并定位可更新项 | 插件管理；抽取新版 `plugins.js`，WinOne 统一导航不添加上游全局侧栏 | 页面回归待验 |
| `85eef5d` 额度刷新不跳位 | 高级额度、账号统计；WinOne 使用既有紧凑组件，不导入上游整体排版；路由账户额度共享新版只读接口 | 相邻页面回归待验 |
| `1acd79c` 压缩用量建议准确标识最省区间 | 用量统计；上游 usage/catalog 经 `usage.go` | 专项待验 |
| `cb05dc2` CodeBuddy 外部会话；`3b3bc35` 增量读 Codex 正在写入的长会话 | Magpie 外部会话管理，不是 WinOne AI 服务入口；`configureUsage` 仍仅允许 WinOne 已登记的 Pi 历史 | 保留来源，禁止发现/删除用户外部会话，不替换 Pi Chat |
| `84cd686` Claude Code 主模型 tier 保留；`252bc61` 外部客户端 caller key 保留；`f191d64` DSH YAML 起始标记；`04e8aa8` Library IME 不跳页；`1755713` Magpie 窄窗标题栏 | 上游独立客户端/资源及外壳；WinOne AI 连接、Pi Chat 和设置壳不使用这些外部配置写入器或上游外壳 | 不扩大为外部客户端自动修改；WinOne 对应功能独立回归 |
| `89521b0` CPU profile 诊断 | WinOne 诊断 / 性能；上游 profiler 保留，不暴露无认证的独立 Magpie CLI 或网关诊断端口 | 安全边界待验，真实 CPU profile 未测 |
| `bad4508` FX 测试、`50a7031` Vision 测试、`3443d18` keychain 测试、`a3b48e0` stream 测试、`d694d9b` address probe 测试时序改进 | 上游测试；不改变 WinOne 功能选项 | 受影响测试待跑 |
| `1357236`、`bd30b06` 退役文档；`5380114` 上游经验记录 | 来源文档，无运行功能 | 已核对变更，不作为 WinOne 指令 |

### 插件接入边界

- P5 `agents/usage/settings/provider` 分别进入账号、用量、插件设置和已保存供应商编辑区；传入上下文仅 `{id,name}`。插件自身页面仍可独立打开。
- 路由/用量存在对应页面，按上游先启用者优先的接替规则保留恢复能力；`sessions/library` 是 Magpie 外部会话/资源页，不接管 Pi Chat，插件仍作为独立页可访问。
- P6 的动作仅把动作 ID 和选中供应商/智能体的公开 ID、名称交给插件页面；不传密钥、请求头、账号邮箱、模型草稿或提示词。保留未保存更改保护。
- 页面/面板保留上游 `sandbox="allow-scripts"`、独立 opaque origin、CSP、声明式只读权限、服务端重复校验、10 秒 hello 超时、局部失败重试和 0–2400px 面板高度边界。

## 验证记录

2026-10-18：使用新版 Magpie 与原 31 包插件的第一轮适配器、OAuth 和账号快照测试通过。新 32 包、新增 UI 和本轮截图/OCR 更改尚须完成对应回归。测试与失败现场保存在 `_temp/ai-capture-latency/`，正式安装器尚未更新。

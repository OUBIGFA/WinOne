# 共享 AI 适配器

截图 OCR、翻译与后续 AI 模块共用 magpie。供应商、账号登录与默认模型集中在 WinOne 独立“AI服务”页；模块不另存凭据或模型配置。

通过 `build/build-ai.ps1` 构建；脚本将入口放入选定的 magpie 源码并运行接口测试，不编译上游界面。

本次验证的完整上游依赖快照保存在 `upstream.go.mod` 和 `upstream.go.sum`；最低 Go 版本为 1.26.8。本次更新没有改变上游 Go 依赖版本。构建测试使用私有账号目录，并复用已配置的 Go 依赖缓存；测试目录位于任务临时目录，不能进入候选包的 `shared/ai`。

构建还需传入 `PluginSourceDirectory` 和 `BunExe`：社区源码锁定 `bcc94e593339539fbe4c074ea666846455b6c167`，Bun 锁定 Windows x64 baseline 1.4.2。来源与逐文件摘要在 `third_party/ai-plugins.json`；脚本核对目录、资源和许可，不运行 npm 安装脚本。产品使用打包的插件和共享 Bun，按供应商的私有目录按需启动；不在打开设置时下载组件。

账号目录包含 19 个内置类型及社区插件支持的供应商，合并为 25 个唯一选项。新选择的连接使用目录推荐的后端；旧配置缺省仍走原内置后端。插件登录支持方法选择、前置问题、设备码显示与复制、API 密钥、浏览器回调及授权码；模型获取与生成调用同一插件。跨安装目录只重新绑定已验证的插件路径，账号与选项保留。

普通账号页按 magpie 的 `pluginSubs` / `plugin.Cached` 流程读取：复用已验证的插件名称和登录方式，每次从当前 `plugin-auth.json` 读取账号，再复用上游账号名称、隐藏状态和过期状态处理。公开资料保存在供应商私有目录的 `winone-account-provider.json`，不保存凭据或账号快照；缓存绑定安装路径、账号类型、插件选项及插件/Bun 的锁定摘要。首次初始化、安装路径或依赖版本变化时重新枚举插件。登录、模型获取及生成仍完整校验资源并启动插件。WinOne 的短命读取进程不启动 magpie 常驻界面的后台刷新。

对应上游依据为本次验证提交 `0bb0781ea065b5236a9a0014bceb84cb84324895` 中的 `internal/gui/plugins.go`、`internal/plugin/providers.go` 和 `internal/provider/plugin_accounts.go`。`plugin_snapshot_test.go` 对比两条路径在登录、换账号、退出后的结果；完整包测试验证移开测试 Bun 后仍能读取账号，而登录仍拒绝缺失的运行时。

服务预设直接读取 magpie 的预设目录并使用 `FromPreset`，保留多协议与模型范围；界面展示 48 个生成服务，纯决策路由器不作为 OCR、翻译或对话供应商。用户修改地址时，其他预设地址不会成为隐式备用目标。

上游：[yetone/magpie](https://github.com/yetone/magpie)，本次验证提交 [`0bb0781ea065b5236a9a0014bceb84cb84324895`](https://github.com/yetone/magpie/commit/0bb0781ea065b5236a9a0014bceb84cb84324895)，MIT。构建读取所选来源的 HEAD，不用固定提交阻塞更新。最新适配及验证见 [Chat 后端与 Magpie 验收](../../docs/2026-10-13-Chat后端与Magpie验收.md)。

每次文本或图像请求使用短命进程；图像为最多 32 MiB 的 PNG，沿用 magpie 的 Chat、Responses、Anthropic 适配。账号目录属于 WinOne；不导入或切换其他客户端账号，不自动安装缺失的官方客户端。

用量页包含概览、请求、上下文和会话四页，默认最近 7 天，也可选择包含起止两天的日期范围，请求和会话列表每页 15 条。账本复用 `internal/usage` 的筛选、分页、速度、价格与 CSV 导出。请求页复用 `HeatmapOf` 的最近 53 周热图，保留当前供应商、来源等筛选，但不被短时间范围截断。CSV 包含全部筛选结果；超过十万条时明确要求缩小范围。共享账本保存供应商与会话标识、模型、Token、耗时、状态，以及上下文分类名称和计数，不保存提示词正文、凭据或上游错误正文。存在请求且价格缓存缺失或过期时，短命读取进程调用 Magpie 的 `catalog.Sync` 更新 models.dev 公开目录，最长等待 8 秒；有效缓存直接复用。更新失败仍返回用量和已有价格估算，并由界面显示重试提示。费用使用公开模型价格，不代表供应商实际账单。

前端 `magpie-ledger-charts.ts`、`magpie-context-graphics.ts`、`magpie-session-calendar.ts`、`magpie-quota-curve.ts` 和 `magpie-usage-heatmap.ts` 仍取自 `ede1e32aec1a4f6bf06aeef6ed2e03d4218db226` 的 `internal/gui/assets/app.js` 与 `context.js`。保留时间堆叠图、速度轨道、Top 7 加其他、排行联动、日期选择、上下文 400 格、请求轨迹、额度曲线和活动日历。概览的来源、模型、账号、供应商均使用上游排行，可展开全部条目；紧凑行保留请求数、费用和份额，完整计时指标保留在可访问描述与提示中。适配部分是 React 生命周期、语言、CSS 变量作用域和 WinOne 数据源。来源与摘要见前端 `ai-usage.source.json`。`context.go` 复用上游分类汇总、评分与标签；移除外部客户端会话名称发现，改用 WinOne 的 Chat 目录。上下文每条会话轨迹保留上游最近 160 点的边界。

读取账本时注入空的 `usage.LogCalls`，阻止 Magpie 在应用筛选前发现外部客户端历史。账号额度只调用已保存 WinOne 账号私有目录的 `LoginUsage(Asked(ctx), agent)`；同一账号仍保持独占，短时竞争最多等待 25 秒并可取消，桌面端首次等待对齐为 30 秒。前端合并在途读取，切换账本筛选不会重复启动额度 worker。失败不补零；Magpie 通过 `asOf` 保留旧额度时，明确显示本次刷新失败及历史记录时间。`chat.usage` 和会话目录经原生 Chat 通道调用，只读取 WinOne 已登记且通过真实路径边界检查的 Pi 文件；历史读取不启动 worker 或扩展。会话的 Token、消息、工具、技能与压缩来自这些文件。费用用私有 Chat 入口设置的会话 ID 关联到账本，未匹配部分标记价格未知；请求和会话统计不能相加。活跃时间按相邻消息间隔累计，忽略超过 5 分钟的空档。

用量回归位于 `modules/ai-magpie/usage_test.go`、`internal/chat/usage_test.go` 和前端 `ai-usage.test.tsx`、`magpie-ledger-charts.test.ts`。网页可视夹具为 `tests/ai-settings-preview.html`，数据由 `tests/ai-usage-fixture.ts` 在内存中生成；页头标明模拟数据，不访问真实服务。

CSV 导出由原生窗口选择路径，再调用 `capture.ai.usage.export.save`。Host 核验调用者确为同安装的 `WinOne.exe`，内部读取完整 CSV，使用带 BOM 的 UTF-8 原子写入，只回传保存状态、路径和条数；大 CSV 不经过 1 MiB 桌面消息。取消对话框不调用保存，只有 `saved` 回执才在界面显示成功。

模型设置保存在 WinOne 的 `Provider.modelMetadata[模型 ID]`；显示名称仍使用 `modelNames`，不改请求路由 ID。`contextWindow`、`maxTokens`、`compactionThreshold` 均为 Token 数，0 使用默认值；压缩阈值交给 Chat 的 Pi 长对话处理。`reasoning`、`vision`、`tools` 用空值表示未知，用布尔值表达明确能力。生成与原始协议请求把输出上限、思考档位及服务档位交给 Magpie 适配；客户端更小的输出上限保留。关闭图像或工具能力后，相关请求会明确拒绝，不静默丢弃输入。

供应商返回的 `rate` 与 `rateWas` 也通过同一份模型元数据保存和传递，分别表示当前及折扣前的额度倍率，不是美元价格。用量、额度和本次上游更新的完整对照及验证范围见 [AI 用量与基座验收](../../docs/2026-10-11-AI用量与基座验收.md)。

请求限制参考上游 `internal/provider/concurrency.go` 和 `internal/gateway/rpm.go` 的每密钥/账号语义。由于 WinOne worker 为短命进程，宿主 `internal/ai/request_limits.go` 在各 worker 之间保留 FIFO、并发和滚动分钟记录。RPM 按 WinOne 调用计数，Magpie 内部重试和辅助请求不重复计数；上游 429 与 `Retry-After` 仍控制重试。队列上限及等待秒数为 0 时不设此项限制；调用方超时与取消继续生效。模型名称、档位和窗口参考上游 `internal/provider/modelprefs.go`、`models.go` 及 `internal/gui/assets/app.js`；独立绿色免费标签沿用该脚本的 `freeBadge` 和 `app.css` 的 `.badge.free`，字号遵守 WinOne 的可读性要求。

回归入口：`internal/ai/model_metadata_windows_test.go` 验证保存与请求快照，`request_limits_test.go` 验证限流、队列上限、取消和等待到期；设置 `WINONE_AI_TEST_EXE` 后，`model_request_integration_windows_test.go` 用本地 HTTP 上游核对完整请求链。前端 `tests/ai-model-settings-check.mjs` 检查中英文、深浅色和窄窗口布局、保存值、免费标签、模型图标间距与侧栏线宽；它只访问内存夹具，不修改真实配置。

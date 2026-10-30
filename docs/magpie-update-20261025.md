# Magpie 固定基准合并与检查（2026-10-25 基准，2026-10-26 续接）

## 范围与结论

本轮沿用中断时已经选定的基准，不再查询或追随更新：

- Magpie：`42ec2700bd2a4cf8d4ceb76a95767ac221e283e5`，相对原 `50f52b540b915bdf5912ddebc48c1fa46eef3025` 共 280 个提交。
- 社区插件：`e5136447b7e5047095c6e9d355e79379736549a9`，41 个打包条目。
- 本地隐私运行时：`pf-v2`；模型文件、privacy-filter.cpp 和 ggml 来源不变，新增上游 `weights-once.patch`；仍由用户主动开启后下载，不随安装器携带或在验证时下载。

隔离工作区的本轮差异已经合回主工作区，保留原有模型档位修复、其他 AI 的未提交改动和旧资源目录。`build/use-local-resources.ps1`、来源锁、插件清单、运行文件清单和许可证记录使用同一基准。

**本轮完成的是固定基准合并及既有接入路径的检查，不宣称 Magpie 的所有新增产品入口已完整接入。** 下方明确区分已接入、上游同版失败和剩余入口。按原会话用户要求，不提交、不推送、不更新固定安装器，也不操作真实安装、配置或账号。

## 功能增量对照

| 上游变化与代表提交 | WinOne 对应入口／实现 | 本轮证据与边界 |
| --- | --- | --- |
| 模型原生档位、提供档位与请求档位分离；代理模型别名识别（`edff2f81`、`ea33a78a`、`4a3e07f1`） | 模型设置、Host `internal/ai`、`modules/ai-magpie/models.go` / `settings.go`；Chat 沿用 Magpie 的 Pi 映射 | 原 MiMo `light/balanced/deep` 保存修复保留；Host/适配器、前端及两种 Chat 会话模型测试通过；没有真实插件账号验证 |
| Codex WebSocket 优先、拒绝或过大后 HTTP 回退（`a2e98cd7`、`3e9efe1d`） | 账号供应商和共享 AI 请求，直接编译上游 gateway | 调整原 HTTP 专用测试夹具，显式拒绝 WebSocket 后检查原 HTTP 请求；适配器测试和上游对应 socket 测试通过 |
| 额度刷新、续期锁、账户回退、Google schema、Claude 搜索引用与工具调用（`85cde4a4`、`be7f1746`、`b28e5ffd`、`1d6b72ad`、`9a210622`） | 账号、网关、共享 provider/gateway；插件条目随锁定包更新 | 增量 provider/gateway 测试执行；不是实际供应商网络或登录验收 |
| Command Code、ZCode、Qoder、Cursor、WorkBuddy 等 mover 最低版本，以及新增社区包 | `third_party/ai-plugins.json`、`modules/ai-magpie/plugin-catalog.json`、插件打包与原管理页 | 41 条目准备通过；目录与运行时文件清单同步；不自动安装外部客户端 |
| 插件自有页面保存 hook 选项、发现页交互和 Bun 不可启动错误（`2c7f0ac1`、`63c0c354`、`5b32b7ea`） | 完整第三方插件管理抽取、registry 和上游 plugin | 抽取、构建、10 项 plugin 增量选择及适配器插件隔离测试；真实第三方登录未重跑 |
| 路由规则多模型、时区、拖动排序、策略切换、Try now、局部行和曲线更新（`f99d27ab`、`c888ef27`、`b5a78fd8`、`b9950956`、`67361b54`、`3173440c`） | WinOne 路由页完整上游 routing 插件、原 groups/trace API | 8 组空态和 8 组有数据布局，真实选模型／保存／未保存取消、中文及页面隔离检查通过；新规则组合并未逐一真实供应商执行 |
| 隐私页从 app.js 移入 redact 插件（`0df6a3ab`、`e698a688`、`9b57261b`） | 高级设置 → 隐私；`build/prepare-ai-masking.mjs` 抽取 app.js 依赖并保留插件 client/page/CSS/i18n | 16 组空态／真实请求记录布局；原文、发送内容、还原信息、精确记录深链和失败重试通过 |
| 按类型／发现方式／阻断筛选、请求内容与还原次数（`51667a34`、`9b57261b`） | 同一隐私页，原 `maskingRoutes`、`maskrec` 和 gateway | 原 API 与页面完整保留；合成请求的脱敏、恢复、阻断及记录详情真实执行；没有遍历全部筛选组合 |
| 保留密钥格式、中文密码／密钥识别、防误脱敏、预览虚构密钥（`bea9f905`、`714ab8e8`、`ee715c2e`、`611b9e17`、`642180a8`） | 隐私设置增加 `redactShape` 原字段；Host 保存与快照、原上游 masker | 保存重开、运行时字段、真实 loopback 虚构密钥与回复还原通过；redact 31 项增量测试通过 |
| pf-v2 GPU 模型只加载一次、关闭 Metal heartbeat（`642d430e`、`5df62c5c`、`d2e0ea39`） | 既有可选本地隐私模型管理与 pii | 下载来源／摘要／许可校验通过，pii 增量测试通过；未下载模型或运行 GPU 推理，不声称性能验收 |
| 用量分块、内存映射、缓存复用、跨供应商无缓存命中率、套餐文本（`115ca343`、`a6399dd4`、`3a47e377`、`1dfa2848` 等） | `prepare-ai-usage.ps1`、Host 用量适配与既有用量页面 | usage 增量运行无失败；前端用量 20 项通过；新上游折线图形态没有因此自动进入 WinOne 自有图表 |
| 内存释放、请求体就地读取、进程枚举与代理读取限时（`20d63621`、`106a2455`、`fc9430e1`、`4725c772` 等） | 共享 gateway/provider/proc/netproxy 实现 | 上游相应增量测试；没有以构建结果代替整进程组资源测量 |
| 合作方目录／赞助、站点和上游外壳导航变化 | 原供应商目录继续读取上游；README／站点／Magpie 外壳不嵌入 WinOne | 不向路由页塞其他功能，不新增外部应用导航 |
| 外部客户端导入、会话接续、自动更新、Mac 托盘与客户端配置（AionUi、Lody、agy、Codex 等） | 维持 WinOne 不接管外部客户端和不嵌入 Magpie 应用的原产品边界 | 相关源码保留；不声称在 WinOne 暴露这些入口；部分平台测试见下方 |

## 本轮修复的嵌入故障

1. `AIPrivacyRecords` 原来创建 `masking.html` 后又改为 `plugins.html`，现始终打开隐私文档。
2. `plugin-bridge.js` 已拥有 `params`、`view`、`mode`、`state`；不再重复声明，避免整组后续脚本停止执行。
3. 记录键继续使用 Host 既有的十进制字符串 DTO。新上游 client 与地址参数里的 `+key` 转换会丢失 UnixNano 精度，现保留字符串；Go 磁盘格式不变。
4. 上游返回栏向外扩展 16px 是为其有内边距的应用外壳设计的；独立文档取消这段外扩，不靠隐藏滚动条裁切。
5. 摘要读取失败明确显示原错误和重试。原 status helper 假设存在 app modal，而且 `ms=0` 会立即清空；抽取时对这两处宿主边界适配。重试按钮按原始英文键随语言切换。
6. 父页面握手完成后重新发布高度，避免初次 ResizeObserver 通知早于父页面确认时，记录区停在 320px。
7. “改为脱敏”交给 WinOne 草稿事务，保留其他类型选择、保存／撤销和未保存保护；不会在 Magpie 另一份设置文件里直接保存，操作后也不谎报已保存。跨来源消息和锁定状态被拒绝。
8. 前端全套检查暴露用量测试过早同步寻找下拉选项，改为等待真实选项挂载；没有修改产品下拉逻辑。

## 验证结果

证据集中于 `_temp/magpie-sync/`：

- `final-ai-build.log`：最终来源许可、41 条目、路由／插件／隐私抽取、完整 AI adapter 测试与构建通过。候选 `merged-ai-build/WinOne.AI.exe` 的 SHA-256 为 `b9ddab7c195d67695d9e13e8acdaa60187c7f8a0bcaf40a4c6b1a8705245a112`；测试后该目录无 AI Worker 残留。
- `final-exact-binary/report.json`（此前同样通过于 `privacy-final-verified/`）：真实 AI Worker + 本地 echo 供应商，16 组中英 × 深浅 × 520/1280 × 空态/记录；脱敏、回复还原、字符串记录深链、密钥格式、阻断不出站、草稿动作桥接、摘要失败重试通过。测试没有真实凭据；截图中的密钥完全是合成夹具。代表截图已查看。
- `routing-merged.log`：路由 8 组空态、8 组有数据、真实编辑与保存；0 浏览器错误。
- `frontend-targeted.log`：隐私文档、设置、模型共 21 项通过；`frontend-full-final.log`：主工作区全套 179 文件／1174 项通过（包含其他 AI 同时保留的工作，不全是本轮新增）。
- `frontend-build-merged.log`：TypeScript 与生产构建通过；保留依赖的 `use client` 构建告警，没有称为零警告。
- `host-ai.log`：`internal/ai`、`apps/host` 测试通过；对应 `go vet` 通过。
- `chat-model-final.log`：普通／Durable Chat 34 项通过，包含供应商自定义思考档位不被强写。
- `verify-import-notices.mjs`：9 imports / 303 adaptations；前端依赖通知 432 项；AI 来源与下载摘要校验通过。

### 上游增量并非全绿

`build/test-ai-upstream.ps1` 按本次增量的新增／修改测试文件选择测试。实际日志中 712 个顶层结果：682 通过、13 跳过、17 失败。平台未编译的名字不算通过。详情为 `upstream-fixed-baseline/`；不要只引用脚本选择的名字数量充当通过数量。

已使用 **同一 `42ec2700` 未打 WinOne 补丁的源码归档** 复查：

- provider 8 项 Codex keychain 测试：Windows 不读取该密钥存储，干净上游同样失败。
- gateway 2 项：套接字交接时 Windows 仍有 I/O；WSL IPv6 loopback 探测失败，干净上游同样失败。
- gui 4 项：otel 正在使用的断言，以及 Lody/AionUi 测试缺 `%AppData%`，干净上游同样失败。
- sessions 2 项：agy 测试对 POSIX 路径的预期与 Windows 路径不同，干净上游同样失败。
- gateway `TestMaskedRecordKeepsSentAndBack`：`Took > 0` 偶发失败；单次干净上游通过，重复 10 次时同样出现 `Took:0`。脱敏、回填和记录存在并不因此失效。证据 `timer-check/clean.log`，未通过钳制假耗时或改断言隐藏它。

`clean-failures/` 保存前 16 项对照；这些是已知限制，不是“全部检查通过”。没有修改上游平台测试、系统网络或安全设置来凑通过。

## 仍未完成／不在本轮合并验收中的内容

以下来自更大的“全量同步”目标，本轮按最后的“沿用当前基准完成合并和检查”要求收口，**没有默默标记为已接入**：

- 新的独立用量归并别名 `UsageAliases`、上游新增折线视图；WinOne 自有用量页面仍是原入口。
- 供应商 `LimitRestOff` 以及代理用户名／密码分字段的新入口；当前供应商配置并未完整增加这些字段。
- 本地请求正文归档开关／容量和新图像 MCP 服务入口。正文落盘还涉及 WinOne 既有隐私边界，未默认开启；不能以编入上游代码等同于 Host 已公开这些入口。
- 真实 Our Free Model 登录与推理、真实 WinOne 原生窗口的本轮完整隐私验收、pf-v2 GPU／整进程资源与长期稳定性、正式安装生命周期，均未验证。本轮浏览器矩阵不是原生窗口验收的替代品。
- 原模型默认思考档位覆盖客户端选择、Anthropic 默认档触发 adaptive 的上游语义保持不变，仍按先前记录保留。

闪聊、文件互传、保存按钮与标签保护已有的完成记录没有重复执行；其真实异地网络等原未测边界也没有因本轮 AI 检查变成通过。

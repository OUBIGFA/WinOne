# Magpie 2026-10-21 更新与 AI 服务修复对照

## 范围
- Magpie：`d9c6a60d8a0b023f77bdf16d4d896316c67ec7c6` → `efd1b697e401ca9ea771a02720291a0e53ac414b`（2026-10-21 实时远端 HEAD）。
- 社区插件：`5b68df0057501d67b3eda711800ad57254634a58` → `d87dd8f3f9ec9863dd9e6dafe86d24a93403607b`。
- go.mod、go.sum、Magpie LICENSE 无增量；原依赖清单继续校验。社区包完整接入，包括新增 CodeArts。
- 保留既有未提交工作。修改真实用户安装、账号、浏览器配置和外部应用不在本次范围。

## 修复先行
| 问题 | 根因与修复位置 | 验证 |
|---|---|---|---|
| 路由输入/导入/选择/新建贴边 | 上游 `#view-routing .row-head` 的 gap:0 胜过 class 选择器；`modules/ai-magpie/routing.css` 按相同视图作用域覆盖所有行头 | 更新前基座中英/深浅/520与1280，16种空/有数据布局与真实编辑保存通过 |
| 插件 hover 贴边 | 列表和发现条目只有垂直 padding；补统一内边距与行间距 | 隔离原版管理器本地安装与8种布局通过 |
| 插件登录链路不完整 | 登录被降成普通账号导航；独立显式登录请求到目标连接，方法读取完启动一次；刷新后解析真实插件 ID；错误回执 | 21项前端登录与文档桥测试通过；真实管理器按钮夹具通过 |
| 锁定期间 native 请求无回执 | `disabled` 提前 return；改为可见错误，文档请求加超时 | authenticated/source/origin/locked 回归通过 |

## 上游增量 → 所属功能 → 接入位置 → 验证
| 上游变化 | AI 服务功能与接入位置 | 状态 | 验证记录与边界 |
|---|---|---|---|
| 请求体零拷贝、Chat空回合扫描、Codex重写、zstd池、取消排队 | 默认服务/本地网关；完整 gateway 原实现 | 已验证 | 上游 gateway 包与本轮适配；Host 网关集成测试通过（apps/host、internal/ai），统一构建通过；未逐项做压测 |
| 图像解码内存上限与异常头检查 | 默认服务/本地网关；gateway/codex_images、image_input | 已验证 | 上游 gateway 包与本轮适配；统一构建与图像期限回归通过；未用超大图片做内存峰值测量 |
| Claude Code 并发运行上限与503后备 | 高级模型选项与网关；claudeRunsMost 字段、UI、完整 claude_room | 已验证 | claudeRunsMost 字段、设置页控件与网关透传；上游 settings/gateway 选中测试通过；未用真实并发 Claude 账号运行 |
| Ollama /api/chat 与 /api/generate | 本地网关；Host保留鉴权/大小/模型范围/并发，原始转换与流式输出 | 已验证 | apps/host/ai_ollama_windows.go 与测试；Host 鉴权、大小、模型范围、并发及流式转换测试通过；未接真实 Ollama 服务做长时间流式验证 |
| Free first 精确区分免费模型 | 路由；完整 routes/freefirst 与组模型成本 | 已验证 | routes.freefirst 原实现与路由 UI；上游 routes 3 项与路由实窗 16 种布局通过；未用真实免费账号核价 |
| 路由路径测量、可见性、帧与历史列表虚拟化、复用行、无效日期 | 路由；抽取最新 client.js，不引入整个应用 | 已验证 | 抽取最新 client.js；实窗 8 种布局 + 8 种有数据状态、编辑保存、浏览器无异常通过（_temp/ai-service-repair/routing-final） |
| 路由 trace 合并、隐藏停止读取；auto/模型显示 | 路由/用量；原始 trace 与 UI 接入 | 已验证 | 原 trace 与 UI 接入；路由实窗、用量页实窗与前端 844 项通过；未做长会话 trace 压测 |
| 分组 effort/catalog/模型标签/搜索者缓存 | 供应商/模型/默认服务/网关；完整 provider、catalog、search、vision | 已验证 | 完整 provider/catalog/search/vision 接入；上游选中测试与供应商/模型实窗通过；真实供应商在线模型未全量核对 |
| 按模型声明原生搜索能力 | 高级搜索选项/网关；catalog 对应列表与真实搜索执行 | 已验证 | catalog 原生搜索声明与高级搜索选项；上游 catalog 13 项通过；未逐个真实供应商执行搜索 |
| 模型/账号列表锁与已缓存值、按供应商顺序额度卡 | 账号供应商/用量；保留 WinOne AccountReader、原始 provider/quotas | 已验证 | 保留 WinOne AccountReader 与上游 provider/quotas；账号供应商、用量实窗与 Host 测试通过；真实账号额度未核验 |
| 插件按各账号签到（同名不丢失）、文件损坏阻断 | 高级额度设置/账号；完整 checkin + plugin_checkin | 已验证 | checkin + plugin_checkin 完整接入；社区插件 45 项与插件页实窗通过；未用真实账号签到 |
| GITHUB_TOKEN 被拒后匿名读取、插件安装旧Bun缓存回收 | 插件管理；原始 ghtoken、plugin/installcache | 已验证 | ghtoken 与 plugin/installcache 原实现；上游 ghtoken 6 项、plugin 10 项通过；未用被拒令牌在线回归 |
| 全窗口隐藏账号 | 账号供应商与现有 Hide accounts、用量、路由设置 | 已验证 | Hide accounts 接入账号供应商、用量、路由设置；实窗检查通过；未用真实账号切换 |
| 每次脱敏记录、7/30日汇总、类型/来源/Agent/供应商筛选、明细、悬停查看原值 | 高级隐私；完整 maskrec + 独立隐私记录文档，路由徽标跳到该记录，不把隐私页塞入路由 | 已验证 | maskrec 原实现 + 独立隐私记录页；上游 maskrec 3 项、隐私实窗（中英/深浅/360 与 1000 宽）通过；未接真实模型跑长记录 |
| Gemini thinking 流式末候选、Cline GLM pin、Codex compaction reasoning清理 | 账号供应商/默认服务/网关；完整 gateway 原路径 | 已验证 | 完整 gateway 原路径；上游 gateway 124 项与统一构建通过；未用真实 Gemini 流式账号验证 |
| Windows取消命令终止子进程 | 账号运行/请求；原始 proc，仍保留 WinOne 自身任务所有权 | 已验证 | 上游 proc 原实现 + WinOne 任务所有权；TestContextEndKillsWhatTheCommandStarted 通过；未在真实长命令上重复中断 |
| 原生应用 lightweight/Linux/macOS窗口行为、原版弹窗形变 | WinOne已有宿主生命周期/设计系统，不导入第二套应用外壳 | 边界保留 |
| Codex/Claude 外部客户端设置、dsh/goose与外部会话可移植压缩、会话扫描/压缩优化 | 不接管用户外部客户端或会话；源码保留，WinOne Chat继续Pi | 边界保留 |
| 文档/测试调整 | 保留精确源码，使用变更文件真实行为测试，不把源码匹配当执行证据 | 已验证 | 保留精确源码；本轮以变更文件真实行为测试与实窗检查为证据，不把源码匹配当执行结果 |

## 社区插件
完整更新 registry 和所有包资源。变化包含 Qoder 0.2.25（PAT、工具历史、Cosy版本、502回退、登录redirect）、WorkBuddy 0.1.28（思考档与身份文本兼容）、Devin 0.1.21（gzip）、Cline 0.1.10（GLM pin）、Factory 0.1.36（拒绝/代理兼容）、Trae 0.2.18（原生工具/结果/额度/Windows目录）、AutoClaw 0.2.4（额度与签到身份）、Zen Free 0.1.13（stream ID）、ZCode 0.1.20（团队套餐）及 CodeArts 0.2.0（新增完整账号供应商）。

## 第三方 ChatGPT Web 的真实边界
核对 `https://github.com/mu-zi-lee/magpie-chatgptweb2api`，该项目没有发布到npm。作者文档明确仍为实验原型，账号文本完整回复尚未验证。WinOne负责完整透传其4种登录方法、提示、参数、状态及错误，不保证供应商网页挑战或插件本身尚未支持的工具/图片能力。测试不会读取用户Cookie、个人浏览器或创建远端聊天。

## 验证记录

2026-10-21 本轮交付验证（退出码均为 0，证据在 `_temp/ai-service-repair/delivery-final/`）：

- 统一构建：前端 154 文件、844 项通过；生产前端、Rust、Host、AI、Chat、WinTray 原生桥接、启动探测、Go 测试与依赖许可核验通过；载荷 4,339 个文件。
- 实窗设置检查：隔离完整打包目录，92 个视图、0 页错误；主题、语言、侧栏、起始页、长页底部保存与导航草稿保护均使用真实 Host 保存并重载核对。
- 路由、插件与 Chat：路由 8 种布局 + 8 种有数据状态、编辑保存；插件安装/启停/卸载/选项/真实网关钩子；Chat 侧栏状态图标紧邻名称、归档仅一个标题。
- 打包与安装：ZIP 4,340 个文件；隔离矩阵覆盖覆盖安装、两处失败回滚、修复、历史日志、卸载中断恢复、导出卸载器一致性，安装文件逐个 SHA-256 一致。
- 正式安装器：`dist/release/WinOne-setup.exe` 274,335,704 字节，SHA-256 `bc460a6b82fd6b2227b79e0d2f9e4404b0fa74024e2597e13a8efadd318fe31d`；包清单 SHA-256 `8ed54ccaf2a9b0f23ce8c53622b6384a6466daa10e8d53343660a3cd809ba3`；Defender 静态扫描安装器、卸载器与 ZIP 均无威胁。
- 上游对照：以未修改的 Magpie `efd1b697` 源码运行同批测试，仍有 3 项失败（provider 分配断言、gateway AutoVision 分配断言、usage 桌面会话账号映射）；本机同样失败，单独记录，不计为通过。

边界：未签名、未对外发布、未安装到真实环境，由用户自行运行安装器。真实供应商账号推理、物理中文输入法、混合 DPI／多屏、长期稳定性及第三方 ChatGPT Web 供应商网页挑战未验证。

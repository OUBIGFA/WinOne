# Magpie 2026-10-20 更新对照

## 来源、边界与交付条件

- Magpie：`c3b2e6109ba9b01ab56b7fe1ce0946ff45ef1c7b` → `d9c6a60d8a0b023f77bdf16d4d896316c67ec7c6`。本次 GitHub 查询的最新提交，共 42 提交、235 个变化文件。
- 社区插件：`ff13aac87dfe1c86b7915badef42cb9046588059` → `5b68df0057501d67b3eda711800ad57254634a58`，4 提交、9 文件；完整保留注册的 32 包及资源。
- 实施前核对表如下。状态为「待验」的不代表已完成；2026-10-19 及更早的真实账号限制继续保留。
- 每个变化落在 AI 服务所属功能，不把 Magpie 整个应用放进路由。Chat 仍用 Pi；外部客户端管理、自启、上游应用更新不是 WinOne AI 服务入口，不写用户真实客户端配置。
- 所有测试、下载资料和候选包放 `_temp/20261020-ui-magpie-resume/`；源依赖复用 `dist/resources/`。未得到本轮验证前不替换正式安装器。

## 上游增量 → AI 服务功能 → 接入 → 验证

| 提交 | 上游变化 | AI 服务所属功能与接入位置 | 本轮验证 |
| --- | --- | --- | --- |
| `ffa292c7d`、`1f98f5ba6`、`cac6e358a` | pf-serve 原生模型运行时、Vulkan 构建与头文件 | 高级设置／隐私；按上游 SHA 按需下载，保留 CPU/GPU 回退与许可，不打包 1.6GB 模型 | 待验 |
| `c0dad80da`、`07ecdda10`、`2fd552947`、`3806014ac` | 模型辅助脱敏、启停/下载/取消/删除/状态，中文人名补规则；普通保存不清空本机模型选择 | 高级设置／隐私；原生 pii/redact/masking + 私有共享服务；Host 鉴权，失败阻断上游请求；WinOne 现成控件与双语 | 待验 |
| `b749cffbb` | 回复预算超过剩余上下文时以实际剩余量重试 | 默认服务／网关；完整 gateway/maxoutput、reply_room | 待验 |
| `b641868eb`、`2c3c90397` | 时区/RPM 回归使用受控时钟，减少负载相关测试抖动 | 网关；更新上游原始测试，不放宽 WinOne 验收 | 待验 |
| `d44d81d9a` | Qoder 最低迁移版本 0.2.19，模型目录失败可回退 | 账号供应商；完整 Qoder 插件、资源锁及 provider/migrate_qoder | 待验 |
| `dd70c5e6a` | ClinePass GLM 的 Upstream 选项也固定 Z.ai | 账号供应商／网关；完整 provider/cline、gateway/cline_pin | 待验 |
| `8febe3a50` | 插件目录短时缺项后恢复已勾选模型；全不选清除恢复意图 | 账号供应商／模型；provider 重列逻辑及 WinOne 模型草稿的 delisted 保存、重开与恢复 | 待验 |
| `197849ad4` | 图像生成超时 1–60 分钟，预设 5/10/15/30/60 | 默认服务／模型选项／网关；MagpieSettings、原有设置表单、Host 超时、gateway/draw | 待验 |
| `be6a74f13` | 路由组按用户明确输入的最大成员窗口选路 | 路由／网关；完整 provider/group 与请求路由 | 待验 |
| `f73378365` | 离开请求明细时还原点击图表前的筛选 | 用量统计；WinOne AIUsage 的临时明细筛选生命周期 | 待验 |
| `a77ecbf8a` | 同供应商同名模型显示最短可区分 ID 片段 | 自定义/账号模型、默认模型、网关、Chat/结果窗；原始 modelTells 与 Go 适配 | 待验 |
| `084c2275d` | 模型选中/取消不改变控件大小 | 模型列表、路由 picker；现有固定复选框尺寸及上游样式 | 待验 |
| `d5bc9f000`、`afa5f74b9` | 普通 403 不使账号休眠到窗口重置，相同请求跨账号同类 403 不休眠；Factory 最低 0.1.34 | 账号／网关／路由；完整 gateway/fallback、plain403 与 Factory 插件 | 待验 |
| `b0b076421` | 路由规则更多菜单常显；空条件保存错误附定位与添加/移除说明 | 路由；完整最新 routing/client/i18n，WinOne 视觉适配保留所有操作 | 待验 |
| `c86390433` | Command Code ZDR 拒绝不休眠，改走下个成员 | 账号／网关／路由；完整 plugin_zdr 路径 | 待验 |
| `94df23926` | Ollama `/api/version`、`/api/tags`、`/api/show`，让 Copilot 发现模型 | 本地网关；复用原始路由并贯通 Host，只公开网关已允许的模型，不绕过密钥/CORS/请求大小 | 待验 |
| `ef655acb4` | 路由追踪从真正服务请求的进程读取 | 路由／网关；共享 RoutingProcess、上游 trace peer，Host 私有能力地址 | 待验 |
| `81a24cf8f` | 自动分组使用 `auto/<供应商原模型ID>` | 路由／模型／网关；完整上游 autoref，Host 分组权限映射同步支持新 ID | 待验 |
| `d155ab161` | Responses 项保持初始 ID，避免重复显示 | 默认服务／网关／Pi Chat 协议；完整 gateway/itemids | 待验 |
| `e5b57794e` | 路由 Agent 选择可输入任意 User-Agent，并列出已出现客户端 | 路由；原始编辑器、i18n、执行与保存全部保留 | 待验 |
| `9588501c0` | Ollama／Ollama Cloud 决策模型经 System One 分类 | 路由；完整 provider/ollama_decide、decide_group | 待验 |
| `fb101b0d5` | GitHub 插件列表失败显示原因与重试，不消失 | 插件管理；完整 plugin/github 与原始 plugins.js 管理器，WinOne 样式适配 | 待验 |
| `16327872c` | Antigravity 退役 2.5 Flash 恢复模型本名 | 账号／模型目录；完整 antigravity_models 与脱敏夹具 | 待验 |
| `5f73537f5` | 单个 ChatGPT 账号 Codex 过载先同账号重试，再后备 | 账号／默认服务／网关；完整 codex_backend | 待验 |
| `b10788745`、`b9d1cd36f`、`e4ac5e871`、`822f2b304` | Claude Desktop 语言、Hermes `${VAR}` 密钥、Zcode 搜索及取消勾选、dsh 输出预算 | Magpie 外部客户端配置；源码保留，但不导入外部客户端管理、不改 Pi、不读取真实 `.env` | 既有隔离边界待验 |
| `40dee5175` | Magpie 自更新等待杀软释放下载、避免重复试启动 | 上游独立应用更新器；WinOne 继续自己的安装/更新事务，不调用 Magpie 自更新 | 边界待验 |
| `5e30d5b5a` | macOS 登录项系统开关状态 | 上游 macOS 外壳；Windows WinOne 不注册其自启 | 不适用于本安装包 |
| `791604559` | 外部 Library 的 RTK/Grok 提示和 issue 链接 | 外部客户端资源库；不替换 Pi 技能或 WinOne 插件管理入口 | 不扩大管理范围 |
| `c12ba4fbc` | README 赞助商 logo 间距 | 上游文档；保留来源，不引入产品 UI | 不适用于本安装包 |
| `8716659ca`、`40d166185`、`d9c6a60d8` | 上游分支合并 | 使用精确合并结果，其功能变化均在以上对应项核对 | 待验 |

## 社区插件

| 包 | 版本与变化 | 接入 | 验证 |
| --- | --- | --- | --- |
| Qoder | 0.2.19；列表读取失败 502 可重试／回退，失效登录 401 | 完整原包与私有账号运行时 | 待验 |
| Raccoon | 0.1.2；扩大额度账单历史读取范围 | 完整原包，额度接口原样返回 | 待验 |
| Zen Free | 0.1.12；流式 function name/arguments 为 null 不再整轮失败 | 完整原包与流式 gateway | 待验 |
| 其余注册包／registry | 没有删减；更新 registry 和所有锁定资源 | build/update-ai-plugins.mjs | 待验 |

## 本地隐私模型接入说明

模型动作是本机即时操作，单独持久化在 WinOne 私有 AI 运行目录，和路由偏好一样不覆盖尚未保存的供应商/高级设置草稿。下载完成才启用；关闭取消下载并停止进程，取消保留断点，删除只清理这份私有模型缓存。默认不下载、不常驻。共享进程提供仅供 Host/已拥有 Worker 使用的鉴权脱敏通道，短任务不重复加载模型；请求文本不落日志。上游判断是否启用规则/个人信息的语义不变，模型不是百分之百防泄露保证。

## 本轮证据

实施中；后续按当前代码实测填写。没有真实账号的供应商线上调用、未下载的完整模型 CPU/GPU 推理，不可由 mock、源码测试或构建替代。

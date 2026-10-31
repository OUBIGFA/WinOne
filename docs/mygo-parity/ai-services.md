# AI 服务页（ai-services）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/features/ai-settings.tsx` 及其挂载的 `ai-*` 组件（账号、供应商、密钥、请求头、头像、模型、模型设置、预设、智能体选择、网关、路由、隐私记录、第三方插件、插件扩展、Magpie 高级设置、代理、隐私模型与规则、内置插件），以及 `magpie-curl.js`、`magpie-proxy-fields.ts`、`magpie-headers.ts`、`magpie-model-tells.js`。MyGo 实现：`apps/desktop-mygo/pages/aiservices`；用量统计在 `pages/aiservices/usage`（单独对照：`ai-usage.md`）；Host 提供的网页由 `apps/desktop-mygo/web` 嵌入。

状态：**已验证** = 有单元测试或渲染截图；**待验证** = 已实现，还缺证据。渲染截图保存在本地 `_temp/mygo-frontend-20261030/ai-shots/`（不入库）。

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| 高级设置的修改直接编辑同一份草稿 | React 用 `advancedDraft` 引用暂存高级设置：先改供应商、再改高级设置时，会以旧副本为基础覆盖掉供应商修改。MyGo 只有一份草稿，不会丢修改。 |
| 数字输入框留空表示“默认/不限制”，输入不在范围内时标红且不生效，离开输入框恢复为当前值 | React 的 `type=number` 会把越界值写入草稿，保存时被 Host 拒绝。 |
| “移除此服务”放在供应商设置底部 | React 的保存栏只把保存/撤销移到右侧栏，删除按钮留在页面内；MyGo 同样留在页面内，位置固定在当前供应商下方。 |
| 路由页的“开启路由组”在页面锁定时回复错误 | React 不回复，网页里的请求会一直等待。 |
| 页面使用宽版布局（最大 1200） | 8 个一级标签在常规宽度下完整显示；模型列表、用量表格需要宽度。窄窗口下标签按约定省略。 |
| Host 网页（路由、插件管理、脱敏记录、插件面板）在 MyGo WebView 中显示 | 用户决定（2026-10-31）。网页原有 postMessage 协议不变，由注入脚本经 `winone-web` 协议转给 Go；网页内链接在浏览器打开，不能跳到其他来源。 |
| 已移除供应商在概览中显示为不可点 | 与 React 相同；保存后消失。 |

## 页面级

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| A1 | 读取 `ai.configuration.get`；保存 `ai.configuration.save`（含 `keyUpdates`）；未声明字段原样保留 | `page.load` / `page.save`，`core.Preserved` | 测试 `TestSavingKeepsFieldsThePageDoesNotEdit` | 已验证 |
| A2 | 一级标签：自定义供应商、账号供应商、默认 AI 服务、本地网关、路由、用量统计、插件、高级设置；切换经未保存保护；保存、账号登录、模型读取、头像上传期间锁定 | `page.View`、`navigationLocked` | 测试 `TestTabsAskTheGuard`；截图 ai-custom | 已验证 |
| A3 | 已启用供应商概览：数量、模型数，点击跳到该供应商的模型（同一草稿内） | `page.overview` / `openProvider` | 截图 ai-custom；测试 `TestRemovingAProviderClearsItsTasks`（已移除者保留） | 已验证 |
| A4 | 保存栏：用量统计、插件标签仅在有修改时显示；网关自有保存 | `page.rail` | 测试（网关单独保存） | 已验证 |
| A5 | 插件扩展租约 `ai.routing.extensions.open/close`（viewer），保存或插件变化后重开 | `extensions.Follow` / `Reload` | 代码审查 | 待验证 |

## 自定义供应商 / 账号供应商

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| P1 | 供应商选择与改名同一字段；Enter/离开完成，Esc 还原 | `editor.nameField` | 代码审查 | 待验证 |
| P2 | 添加自定义供应商（上限 32）；从服务预设添加（筛选语言、仅推理服务、合作伙伴说明与链接；空白新连接被替换） | `page.add` / `addPreset` / `presetPicker` | 代码审查 | 待验证 |
| P3 | 添加账号先选智能体，取消不留空连接；按类型命名并编号去重 | `agentPicker` / `uniqueName` | 测试 `TestAccountSignInRefreshesTheAccounts` | 已验证 |
| P4 | 连接：协议、预设地区（切换时重置模型与地址）、地址（HTTP 提示、本地服务提示）、粘贴 cURL 导入（地址、协议、模型、密钥；占位符不保存） | `editor.connection` / `importCurl` / `curlOf` | 测试 `TestCurlExampleFillsTheDraftWithoutSaving`、`TestCurlReadsVendorExamples` | 已验证 |
| P5 | 多密钥：添加（≤16）、显示/隐藏（已保存的经 `ai.keys.reveal`，不进草稿）、启用、删除、轮换策略、失败切换、尝试次数、冷却 | `editor.keys` / `keyRow` / `policy` | 测试 `TestNewKeysTravelApartAndSavedKeysAreRevealedOnRequest` | 已验证 |
| P6 | 高级：头像（文件、网站、内置、地址、重置、拖入；分块上传 `ai.icons.store`）、Responses 延迟工具、请求头（单行/JSON） | `editor.avatarView` / `advanced` / `headersView` | 测试 `TestHeadersFollowMagpie` | 部分验证 |
| P7 | 配额或限流时暂停请求开关 | `editor.View` | 截图 ai-custom | 已验证 |
| P8 | 模型：获取（替换目录、暂停的账号模型、清除失效任务）、测试（全部/单个/以 Codex 或 Claude Code 身份）、搜索与手动添加、全选/全不选、右键菜单、复制 ID、列表高度可拖动并记住 | `modelsPanel` | 测试 `TestDiscoveryReplacesTheCatalogAndClearsTasksOfRemovedModels`、`TestAnUnsavedConnectionIsSentWithDiscovery`；截图 ai-models | 已验证 |
| P9 | 模型设置：显示名称、请求名称、映射、API、提供的思考档位、价格（含长上下文）、恢复默认、上下文窗口/最大输出/压缩阈值（含快捷值与旧阈值）、思考档位、服务档位、能力 | `modelsPanel.settings` | 测试 `TestModelSettingsKeepTheRoutingID`；截图 ai-model-settings | 已验证 |
| P10 | 请求限制（并发、RPM、排队数量、排队等待），有值时默认展开 | `modelsPanel.limits` | 代码审查 | 待验证 |
| P11 | 账号登录：方式、账号列表（可隐藏）、登录/重新登录、刷新、签到及结果、设备码、登录链接、粘贴回调/授权码/密钥、插件提问、打开页面、取消；离开时取消 | `accountPanel` | 测试 `TestAccountSignInRefreshesTheAccounts`、`TestPluginManagerOpensItsAccountType` | 已验证 |
| P12 | 插件的供应商/智能体操作菜单（按钮与右键） | `extensions.providerMenu` | 代码审查 | 待验证 |
| P13 | 插件面板：provider、agents 插槽 | `extensions.Slot` | 代码审查 | 待验证 |
| P14 | 移除服务（清除引用它的任务与新密钥） | `page.remove` | 测试 `TestRemovingAProviderClearsItsTasks` | 已验证 |

## 默认 AI 服务、本地网关、路由、插件

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 默认服务（供应商与模型）；切换翻译模型后立即翻译；显示思考过程 | `page.defaultsView` | 代码审查 | 待验证 |
| G1 | 网关：启用、端口（1024–65535）、状态与刷新、地址、访问密钥显示/复制/重置、模型（按供应商分组，可搜索、全选）、支持的接口；单独保存与撤销 | `gatewayPanel` | 测试 `TestGatewayValidatesItsPortAndSavesApart`；截图 ai-gateway | 已验证 |
| R1 | 路由：启用开关（草稿）；Magpie 路由页（WebView），主题同步、高度、查看脱敏记录、开启路由组；保存后重开；插件页可替换 | `routingPanel` | 测试 `TestRoutingEnablesGroupsWithoutSavingOtherDrafts` | 部分验证（WebView 未在真实窗口验证） |
| L1 | 第三方插件管理（WebView）：选择文件夹、打开账号类型（可直接登录）、变化后刷新账号目录；锁定时回复错误；操作菜单打开指定页面 | `pluginsPanel` | 测试 `TestPluginManagerOpensItsAccountType` | 部分验证 |

## 高级设置

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| V1 | 额度：显示方式、读取、币种、单位；使用与余额提醒、重置前提醒；托盘额度与刷新间隔；预热（重置后、每日多时段、Codex 各账号独立时间/自动重置/积分）、每日签到（含插件） | `advancedPanel.quota` / `warmup` | 测试 `TestAdvancedEditsAreDraftsOfTheConfiguration`；截图 ai-advanced | 部分验证 |
| V2 | 模型：代理（自动/直连/自定义，拆分登录，应用到草稿，未应用时离开会询问）、识图与生图模型、生图超时、MCP 地址复制、网页搜索（搜索模型、搜索 API 增改排序启停取 Key、优先顺序）、回复设置、客户端模型 | `advancedPanel.modelOptions` / `proxyView` / `searchAPIs` | 测试 `TestProxyLoginRoundTrips` | 部分验证 |
| V3 | 隐私：脱敏记录（WebView，暂存动作并跳到表单）、脱敏密钥/格式/个人信息及类型、本地隐私模型（下载、进度、删除确认、GPU 回退）、隐藏账号、自定义词、规则（前缀/正则，编辑删除）、按类型处理、始终原样发送、统计上报、请求存档、内网下载、账号读取 | `advancedPanel.privacy` / `privacyModel` / `rules` / `actions` | 测试 `TestMaskingRecordsStageTheirKinds`、`TestRuleKindsNormalizeAsMagpie` | 部分验证 |
| V4 | 内置功能：预设、恢复为预设、每个内置插件（启用、正在使用、按 schema 的选项、详情）；内置扩展（启用、类型与账号、包名、JSON 选项，无效时禁止保存） | `advancedPanel.features` | 代码审查 | 待验证 |
| V5 | 插件面板（settings 插槽） | `extensions.Slot` | 代码审查 | 待验证 |

## 未验证范围

- WebView 嵌入（路由、插件管理、脱敏记录、插件面板）只做了桥接单元测试（`web/web_test.go`）；真实窗口中的主题同步、高度和原生请求尚待在用户不使用电脑时验证。
- 账号登录与模型测试只用假 Host 验证；真实供应商未测。

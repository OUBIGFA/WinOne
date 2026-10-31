# AI 服务 · 用量统计（usage）功能对照

更新：2026-11-01。React 来源：`apps/desktop/frontend/src/features/` 下的 `ai-usage.tsx`、`ai-usage-chart.tsx`、`ai-usage-breakdowns.tsx`、`ai-usage-heatmap.tsx`、`ai-usage-period.tsx`、`ai-usage-quota-curve.tsx`、`ai-usage-sessions.tsx`、`ai-usage-session-charts.tsx`、`ai-usage-context.tsx`、`ai-usage-aliases.tsx`、`ai-usage-types.ts`、`ai-request-archive.tsx`、`ai-request-speed.ts`、`magpie-*.ts`、`magpie-ledger-lines.css`、`ai-usage.css`，挂载方式见 `ai-settings.tsx` 第 231 行。MyGo 实现：`apps/desktop-mygo/pages/aiservices/usage`（`package usage`，由 AI 服务页通过 `usage.New`、`View`、`Close` 嵌入）。插件对用量页的替换和 `slot="usage"` 扩展面板由 AI 服务页处理，不在本表。

状态：**已验证** = 有单元测试或 `ui.Tester` 渲染截图证据；**待验证** = 已实现，还缺证据。测试在 `apps/desktop-mygo/pages/aiservices/usage/*_test.go`。截图由 `WINONE_USAGE_SHOTS=<目录> go test -run TestScreenshots ./pages/aiservices/usage/` 生成，本次 35 张保存在本地 `_temp/mygo-test-installer/usage-shots/`（不入库）：中英、深浅色、1100/700/520 宽，含图表与热图悬停。

当前已挂载到 MyGo 测试安装器。父页 `TestUsageIsMountedAndAliasesSaveThroughThePage` 覆盖读取、模型归并草稿、统一保存与撤销；`TestTopLevelTabAppliesAfterTheUnsavedGuardAnswers` 覆盖延后确认的标签切换。完整包、原生启动与隔离安装证据在 `_temp/mygo-test-installer/`；本次用量截图和请求测试仍是正式组件加私有 Host 夹具，不等同于真实供应商账号或已暂缓的 Chat 页面验收。

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| 图表旁的排行中，排在图表前 7 组之后的分组使用“其他”的颜色；React 按排名循环使用 7 种颜色 | 与图表一致：图表把第 8 组起并入“其他”，循环颜色会让两组共用一种颜色。概览分布排行没有图表，仍按排名配色 |
| 图表的 `chartBy`（选中某天时的整段图例）也按供应商、模型显示名称；“模型 · 供应商”分组显示为“模型名 · 供应商名”；网关密钥分组显示密钥名称，无密钥显示“未使用网关密钥” | React 只给 `by` 命名，图表提示和这两种分组显示内部标识 |
| 分组名称每帧按当前语言和供应商生成 | 切换语言或修改供应商后立即生效，与 React 渲染时命名一致 |
| 请求明细每条请求一行卡片：时间（展开）与状态在首行，其余字段带名称按宽度换行 | React 在容器窄于 850 px 时已改用同样的分栏卡片；设置页列宽很少超过该值，MyGo 不提供横向滚动的表格 |
| CSV 导出失败单独显示在导出按钮下方，“重试”重新导出 | React 与读取错误共用提示，“重试”只会重新读取统计 |
| 保存对话框中输入的文件名没有扩展名时补上 `.csv`/`.json`；补上后的文件已存在时拒绝并提示重新选择 | MyGo 的保存对话框没有默认扩展名，不能替用户确认覆盖（与配置迁移导出一致） |
| 请求热图随宽度缩小，不横向滚动；因此不需要上游“保持最新一周可见”的滚动处理 | 全局禁止横向滚动；React 的 WinOne 样式同样让网格缩放 |
| 会话活动日历按上游每周最多 20 DIP 布局；自选日期范围显示该范围的每一天 | React 的 `min-width:220px` 让 7 天日历的格子变成约 90 px；React 对自选范围只显示今天一格 |
| “用量趋势”显示为可见标题；速度柱的底轨使用较深的灰色；悬停列使用悬停灰 | 层级更清楚；原底轨 `#f2f2f2` 在浅色背景上几乎不可见。界面其余部分为中性黑白灰，图表分组颜色保留 React 的低饱和配色（数据编码） |
| 图表指标、图表样式、热图指标保存在配置目录 `desktop-usage-view.json` | React 保存在 WebView 的 localStorage；它们是查看偏好，不是 AI 设置 |
| 额度趋势范围和曲线放大状态在切换视图后保留；额度数据仍在回到概览时重新读取 | React 卸载组件后这些选择会重置 |
| 搜索框使用普通输入框，不显示占位文字 | MyGo 0.7.3 搜索框的内置占位文字只有英文（“Search”）；React 原输入框也没有占位文字 |
| 上下文评分的名称列等宽对齐；未知的上下文标签显示其原始键名 | 新版 Magpie 可能新增标签；React 会显示 `usage.tag…` 键 |
| 归档请求头按键名排序显示 | Go 的映射没有原始顺序 |
| 账号行在没有套餐名时不显示末尾的“ · ”；额度窗口说明在没有显示文字时不以“ · ”开头 | React 会留下孤立的分隔符 |
| 不发送 `tz` 参数 | Magpie 的 `usage.Zone("")` 等于本机时区（`time.Local`）；React 传浏览器时区，在同一台电脑上结果相同 |

## 已知限制（需要外壳或接口支持）

| 项 | 现状 |
| --- | --- |
| “打开会话”（会话详情、上下文会话） | React 打开 Chat 并选中该会话；现在只切到 Chat 页，因为 `Options` 和 `core.Env` 没有“打开指定会话”的入口 |
| 日期选择器的按钮名称（“Previous month”等） | MyGo 0.7.3 只带英文控件文案，影响自选日期弹出日历的可访问名称 |

## 页面级

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| U1 | 说明文字在视图标签上方；视图：概览、请求明细、上下文、会话、模型归并（有 `onPreferencesChange` 时才有归并） | `Usage.View`、`tabs` | 测试 `TestAliasesHideWithoutADraft`，截图 | 已验证 |
| U2 | 视图标签切换经页面未保存保护 | `kit.SubTabs(..., env.RequestChange)` | 测试 `TestViewTabsAskTheGuard` | 已验证 |
| U3 | 非归并视图显示时间范围、自动刷新（手动/15/30/60 秒）、刷新按钮（读取中忙碌并禁用） | `controls` | 截图，测试 `TestAutomaticRefreshCountsOnlyWhileShown` | 已验证 |
| U4 | 时间范围：今天、最近 7 天、最近 30 天、全部、自选日期；自选日期点“应用”才生效，倒序日期自动排序；默认 7 天 | `periodPicker` | 测试 `TestChosenDatesApplyAsOneInclusiveRange`、`TestDefaultsToSevenDaysAndPagesFifteenRequests` | 已验证 |
| U5 | 自动刷新只在标签显示、不是归并视图、间隔非 0 时计时；切换视图或间隔重新计时；隐藏窗口不刷新 | `Usage.sync` 中 `c.After` | 测试 `TestAutomaticRefreshCountsOnlyWhileShown` | 已验证 |
| U6 | 标签不显示时不读取、不计时；点击图表/排行进入请求明细后，离开请求明细或标签隐藏时恢复原筛选（含之前的手动修改） | `Options.Active`、元素本地标记检测未显示的帧、`deactivate`、`leave` | 测试 `TestInactiveReadsNothingAndADrillDownGivesBackItsFilters`、`TestLeavingTheRequestsGivesBackTheFiltersADrillDownReplaced` | 已验证 |
| U7 | 已保存配置的 revision 变化后重新读取统计 | `ledgerKey.settings` | 测试 `TestASavedConfigurationReadsAgainAndPicksAreSaid` | 已验证 |
| U8 | 迟到的旧筛选结果不能覆盖新结果 | `ledgerRead.generation` | 测试 `TestALateAnswerForAnOldFilterDoesNotReplaceTheCurrentOne` | 已验证 |
| U9 | 页面关闭：清除计时器与归并输入的活动报告 | `Usage.Close` | 测试 `TestClosingForgetsTheFormAndDropsLateAnswers` | 已验证 |
| U10 | 窄窗口不溢出、不横向滚动 | 换行布局 | 测试 `TestNarrowWindowsKeepEveryTextInside`（中英，640/480 宽，五个视图），截图 520/700 宽 | 已验证 |

## 概览与请求明细共用

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| L1 | 读取 `capture.ai.usage.get`（筛选：period、provider、source、failed、offset、day、model、account、purpose、query、callerKey） | `readLedger` | 测试 | 已验证 |
| L2 | 筛选：供应商（带图标）、来源（全部/WinOne 功能/本地网关）、搜索；仅显示失败请求；有筛选时显示“清除筛选”（保留时间范围）；修改筛选回到第一页 | `ledgerView`、`pickSelect`、`filter.narrowed` | 测试 `TestALateAnswer…`、`TestGatewayKeyFilterIncludesCallsWithoutAKey` | 已验证 |
| L3 | 选中日期/模型时显示“日期 模型名” | `ledgerView` | 测试 `TestASavedConfigurationReadsAgainAndPicksAreSaid` | 已验证 |
| L4 | 读取中状态、导出成功提示、读取错误与“重试”、价格更新失败提示 | `ledgerView` | 测试 `TestReadFailureOffersRetry`、`TestPriceWarningKeepsTotals…` | 已验证 |
| L5 | 合计：请求数（失败数）、输入（缓存）、输出、费用（USD）、套餐积分（有记录时）、输出速度（实测/下限分开、说明提示）、首字延迟、缓存命中率（排除不统计缓存的请求并说明）；未计价请求说明 | `usageTotals` | 测试 `TestTotalsLeaveUnknownCacheOutAndKeepSpeedsApart`、`TestPriceWarningKeepsTotalsAndCreditsShowWithFirstTokenWait` | 已验证 |

## 用量趋势图

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| C1 | 指标（总 Token/费用/请求数/速度）与样式（柱状/折线）可选并记住；分组（模型、模型 · 供应商、供应商、来源、有网关密钥时的网关密钥） | `chartView.view`、`viewPrefs` | 测试 `TestChartStyleAndMetricAreRemembered` | 已验证 |
| C2 | Magpie 柱状图：前 7 组按排名配色叠放，余量为“其他”；坐标轴取整、左边距按最宽刻度；X 轴刻度按宽度间隔 | `chartPlan.segments`、`paint` | 测试 `TestColumnsStackAndLinesBandEachGroup`、`TestEmptyChartsSayWhy`，截图 | 已验证 |
| C3 | 速度：每列是该时段速度的底轨，各组为标记，超出底轨的标记有引线；不叠加 | `segments` | 测试 `TestSpeedLinesBreakAtUnmeasuredRepliesWithoutStacking` | 已验证 |
| C4 | 折线：叠放折线带半透明面积，下层不被上层覆盖；速度折线在无测速处断开，单点为圆点，时段速度为虚线 | `chartPlan.lines`、`ledCurve` | 测试同上 | 已验证 |
| C5 | 无数据/无价格/无测速时显示原因 | `chartPlan.none` | 测试 `TestEmptyChartsSayWhy` | 已验证 |
| C6 | 悬停显示时段提示：时间、各组数值（降序）、合计与请求数；速度提示分列显示首字延迟；提示保持在图内 | `chartView.tooltip` | 截图 `*-overview-hover.png` | 已验证 |
| C7 | 按天的图可点击或用键盘选择某天（再次点击取消），选中的天高亮，其他天的柱变淡 | 每列一个切换按钮（一个 Tab 停靠点，方向键切换） | 测试 `TestChartDaysPickTheirRequests` | 已验证 |
| C8 | 旁边的排行：全部分组，显示数值、条形、请求数/Token/费用/积分/速度/首字/命中率/占比、失败与循环数（红色）；点击进入该组请求明细；悬停时图中突出该组 | `rankRows`、`rankList`、`chartView.emphasis` | 测试 `TestRankingsKeepMagpiesTopSevenAndOther`、`TestInactiveReadsNothing…`（点击模型），截图 | 已验证 |
| C9 | 宽度变化时重绘（ResizeObserver 合并） | 绘制函数每帧使用实际尺寸 | 不适用 | — |

## 概览

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| O1 | 来源、模型、账号、供应商四个排行，显示项数；默认列出全部，超过 7 项时可收起为前 7 项加“其他”；空时说明；点击进入请求明细 | `Usage.breakdowns` | 测试 `TestOverviewRankingsFoldToTheirTopSeven` | 已验证 |
| O2 | 无请求时显示空状态 | `ledgerView` | 测试 `TestReadFailureOffersRetry` | 已验证 |

## 额度与余额

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| Q1 | 读取已登录账号（有 agent）和有密钥的 API 供应商：`capture.ai.accounts.quota {provider, agent?}`；概览显示时读取，刷新时重读 | `allowances.read` | 测试 `TestAllowancesUseTheSavedChoicesAndNeverInventAReading` | 已验证 |
| Q2 | 同一账号正在读取时，新的读取共用它；读取结束后的刷新重新读取；失败不缓存 | `allowances.readOne` | 测试 `TestOneAllowanceReadInFlightServesARefresh` | 已验证 |
| Q3 | 隐藏/显示账号、显示剩余/已用，写入 AI 草稿（`hideAccounts`、`quotaLeft`），随已保存偏好同步 | `allowances.view`、`Options.Change` | 测试同 Q1 | 已验证 |
| Q4 | 额度趋势：关闭/最近两天/当前周期 | `allowances.trend` | 截图 | 已验证 |
| Q5 | 每个供应商：图标与名称、错误、无数据说明；每个账号：账号（隐藏时“账号 N”）与套餐、读取失败或历史数据提示、余额、余额组成（赠送/充值等，零值保留，失败部分说明，百分比条）、各窗口剩余/已用与进度条、显示文字与重置时间、读取时间 | `quotaView`、`allowanceMessage`、`balanceDetails` | 测试 `TestAStaleAllowanceIsAFailedRefreshAndConsolesAreWebOnly`，截图 | 已验证 |
| Q6 | OpenCode 不提供余额时显示说明而非错误；控制台链接只接受无凭据的 https 地址 | `allowanceMessage` | 测试同上 | 已验证 |
| Q7 | Magpie 剩余额度曲线：周期断点、均匀消耗虚线、短窗口淡化、当前时间线、图例与最近读数提示；点击/Enter/空格放大 | `quotaCurve`、`drawQuotaPlot` | 测试 `TestQuotaCurvesUseTheRecordedReadings`，截图 | 已验证 |

## 请求明细

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| R1 | Magpie 请求热图：53 周、Token/请求数/费用三种指标（记住选择）、四分位深浅、月份与星期标签、悬停提示（指标排在首位）、图例；无请求时隐藏 | `heatmapView` | 测试 `TestHeatmapMetricIsRemembered`、`TestHeatmapTipsSayTheMetricFirst`，截图 `*-requests-hover.png` | 已验证 |
| R2 | 账号、网关密钥（含“未使用网关密钥”）、用途筛选 | `requestsView` | 测试 `TestGatewayKeyFilterIncludesCallsWithoutAKey` | 已验证 |
| R3 | 导出 CSV：系统保存对话框（“导出用量 CSV”、默认 `WinOne-usage.csv`），取消不调用 Host；只把筛选和选定路径发给 `capture.ai.usage.export.save`；回执须为 saved、路径一致、行数为非负整数，否则“无法确认 CSV 已保存”；成功提示保存位置与条数；无请求时禁用 | `exportCSV`、`withExtension` | 测试 `TestCSVExportSendsOnlyTheChosenPathAndConfirmsTheReceipt` | 已验证 |
| R4 | 每条请求：时间（展开详情）、模型图标与名称、供应商与来源、输入/输出与缓存、费用或“价格未知”与套餐积分（含 0）、耗时、首字延迟、输出速度（下限加“≥”，无速度时提示原因）、状态（本地拒绝/失败/成功与状态码，失败标红） | `requestRows`、`requestSpeed` | 测试 `TestRequestsExplainMissingSpeedsAndLabelLowerBounds`、`TestRequestSpeedsSayWhyThereIsNone`、`TestPriceWarningKeepsTotalsAndCreditsShowWithFirstTokenWait` | 已验证 |
| R5 | 详情：网关密钥、请求模型、响应模型、接口、思考 Token、思考档位、缓存读取/写入/一小时写入、首字延迟（毫秒）、结束原因、错误；缺失显示“—” | `requestDetails` | 截图 | 已验证 |
| R6 | 每页 15 条，上一页/下一页，显示“第 x–y 条，共 n 条” | `pager` | 测试 `TestDefaultsToSevenDaysAndPagesFifteenRequests` | 已验证 |

## 请求存档

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| A1 | 点击后才读取 `capture.ai.archive.read`；读取中状态；正文较大说明；请求/回复正文（截断说明、省略说明）与请求头；正文区域纵向滚动 | `archiveView` | 测试 `TestArchiveOpensOnDemandRetriesAndReportsOnlyCompletedExports` | 已验证 |
| A2 | 导出 JSON：系统保存对话框（“导出请求归档”、默认 `WinOne-request.json`），取消不调用 Host；`capture.ai.archive.export.save {archive, path}`；回执须为 saved 且路径一致，否则“无法确认归档已保存”；成功显示路径 | `archiveView.save` | 测试同上 | 已验证 |
| A3 | 读取或导出失败显示错误，“重试”重做失败的那一步 | `archiveView.failed` | 测试同上 | 已验证 |

## 会话

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| S1 | 说明；读取 `chat.usage {provider, model, project, query, offset, topBy, period}` 与 `chat.catalog`（项目名，失败不影响统计）；空目录（null）仍显示“全部项目” | `sessionsView.sync` | 测试 `TestSessionsAndContextUseWinOnesOwnRecords`、`TestEmptyCatalogKeepsTheAllProjectsFilter` | 已验证 |
| S2 | 供应商、项目、搜索筛选；读取中、错误与重试、警告 | `sessionsView.view` | 测试同上 | 已验证 |
| S3 | 合计：会话、总 Token、输出、费用、消息、活跃时间、中位数、P90；未计价说明 | `tiles` | 测试同上（`$0.020000`），截图 | 已验证 |
| S4 | 会话活动日历（Token/输出/消息/会话/费用/活跃时间） | `activityView`、`sessCalendar` | 测试 `TestLevelsGradeByQuartiles`，截图 | 已验证 |
| S5 | 会话分布（消息/分钟/每次提问的工具调用）柱状分箱与说明 | `shapeView` | 截图 | 已验证 |
| S6 | 趋势图（与请求明细同一图表）；点击供应商或模型排行时按其筛选 | `sessionsChart`、`chartView` | 截图 | 已验证 |
| S7 | 按项目、模型、活跃时段分组（可点击筛选项目和模型），工具调用和技能读取次数 | `sessionsView.view`、`groupRow` | 测试同 S1（`read`），截图 | 已验证 |
| S8 | Token/费用/活跃时间最多的会话；最近会话（模型、更新时间、已归档、错误）与分页；点击展开详情 | `sessionList` | 测试同 S1 | 已验证 |
| S9 | 会话详情：打开会话、写入中说明、调用/输入/输出/缓存/思考/压缩/失败、上下文记录（每页 15 条） | `sessionsView.details` | 测试 `TestSessionDetailsShowTheirCountsAndContext` | 已验证（“打开会话”只切到 Chat 页，见已知限制） |

## 上下文

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| X1 | 说明；读取 `capture.ai.usage.get {period, provider, offset}` 的上下文部分与 `chat.catalog` 会话标题；供应商筛选回到第一页；读取中、错误与重试 | `contextView.sync` | 测试 `TestSessionsAndContextUseWinOnesOwnRecords` | 已验证 |
| X2 | 每个供应商：图标、名称、评分（窗口未知时显示“—”且只列缓存和可靠性）、请求与会话数、评分条、中位数/P90/固定开销/每次增长/命中率/压缩次数、标签（数值提示）、组成比例条 | `agentView` | 截图 | 已验证 |
| X3 | 无记录说明 | `contextView.view` | 测试 `TestSessionsAndContextUseWinOnesOwnRecords` | 已验证 |
| X4 | 会话上下文列表：标题（目录标题优先）、模型/请求数/压缩次数、峰值/窗口；点击打开最新请求；分页 | `contextView.view` | 测试同 X1 | 已验证 |
| X5 | Magpie 迷你曲线：按峰值缩放，接近窗口时显示窗口线；当前请求标记；点击某点打开该请求，提示“第 n 次请求：x Token” | `contextView.spark`、`newSpark`、`fmtK` | 测试 `TestFormatsFollowTheReactWindow`（fmtK），截图 | 已验证 |
| X6 | 请求上下文：窗口占用、计数来源说明、400 格组成图（Magpie 最大余数分配，格子提示组成名称）、各组成的条目与 Token；有目录标题时“打开会话” | `promptDetail`、`cellsOf` | 测试 `TestContextCellsShareTheWindowByLargestRemainder`、`TestSessionsAndContext…` | 已验证（“打开会话”见已知限制） |

## 模型归并

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| M1 | 说明；原模型标识与归并名称输入（Enter 提交），“应用归并”经 `capture.ai.magpie.usage-aliases {aliases, edit}` 校验整份草稿，成功写入 AI 草稿并清空输入；失败保留输入与草稿 | `applyAlias`、`aliasesView` | 测试 `TestAliasesValidateTheDraftAndLeaveSavingToThePage` | 已验证 |
| M2 | “添加或移除后，点击保存生效”；空状态；列表（原标识 → 名称）与“移除”（可访问名称“移除 x 的归并”） | `aliasesView` | 测试同上，截图 | 已验证 |
| M3 | 页面锁定时禁用输入和按钮，不发送请求 | `Options.Disabled` | 测试同上 | 已验证 |
| M4 | 未应用的输入作为 `ai-usage-aliases` 的未保存内容报告，放弃时清空；离开归并视图时清空 | `reportAliases`、`leave` | 测试 `TestLeavingTheAliasesForgetsTheirForm`、M1 测试 | 已验证 |

## Magpie 算法移植（MIT）

| 编号 | 上游 | Go 位置 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| G1 | yetone/magpie `42ec2700bd2a4cf8d4ceb76a95767ac221e283e5`：ledGroupSpeed、cacheHitOf、ledAxis、ledCategories、drawLedColumns、drawLedRank、ledLines、ledCurve 与中英文消息 | `ledger.go`、`messages.go` | `ledger_test.go` | 已验证 |
| G2 | `c4a5e61530cdd979369721826eca14ff86f426ee` plugins/usage/page.js：请求速度下限与无速度原因 | `speed.go` | `TestRequestSpeedsSayWhyThereIsNone` | 已验证 |
| G3 | `ede1e32aec1a4f6bf06aeef6ed2e03d4218db226`：额度曲线、请求热图、会话日历与四分位、上下文网格与迷你曲线 | `quotacurve.go`、`heatmap.go`、`calendar.go`、`contextgraphics.go` | `ledger_test.go`、截图 | 已验证 |
| G4 | 数字、货币、紧凑计数、日期与 `toFixed` 与 React 的 Intl 输出一致（中英） | `format.go` | `TestFormatsFollowTheReactWindow`（期望值取自 Node 24 的 Intl） | 已验证 |

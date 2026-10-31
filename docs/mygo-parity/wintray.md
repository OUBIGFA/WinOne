# 程序管理页（wintray）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/features/wintray.tsx`、`wintray-program-actions.tsx`、`wintray-program-settings.tsx`、`wintray-maintenance.tsx`、`program-icon-cache.ts`、`wintray.css`、`styles/settings.css`（设置界面的列表列宽）、`ui/number-field.tsx`、`ui/path-drop.ts`，以及 `main.tsx` 的挂载（`disabled` 取模块状态，`onChanged` 刷新快照）。旧窗口桥接：`apps/desktop-rust/src/component_bridge.rs`（`wintray.*`、`modules.resetWinTray*`、`ProgramInfo`、`InspectDroppedPaths`）、`component_ui.rs`（`PickProgram/PickPrograms`）、`programs.rs`，旧 Go 窗口 `apps/desktop/program_info_windows.go`、`program_cache_windows.go`。MyGo 实现：`apps/desktop-mygo/pages/wintray`；读取程序名称与图标在 `apps/desktop-mygo/desktop/programinfo_windows.go`；两栏分隔条为新增共享组件 `apps/desktop-mygo/kit/split.go`。

状态：**已验证** = 有单元测试或真实窗口证据；**部分验证** = 列出的部分有证据，其余见“未验证范围”；**待验证** = 已实现但没有证据。

证据写法：
- 测试名：未注明文件的在 `pages/wintray/wintray_test.go`；`desktop/`、`kit/` 开头的在对应目录。
- “真实窗口”：2026-10-31 在仓库 `_temp/` 下的隔离配置里运行 MyGo 窗口、真实 Host 与 WinTray Worker 核对；程序是复制到 `_temp` 的系统程序和一个不运行的脚本。隔离配置下 Worker 不改动系统登录任务（`allowSystemStartup` 只在默认配置目录为真）。核对时没有启动任何程序，也没有开启托盘收纳（开启后 Worker 会接入资源管理器的任务栏）。截图未入库。

## 不属于程序管理页的内容（不移植）

| React 内容 | 判断依据 |
| --- | --- |
| `features/wintray-icons.tsx`（已收纳图标列表、`wintray.icons` 轮询、`icon.activate`） | 除自身测试外没有引用；`wintray.test.tsx` 的 “sorts programs directly…” 断言页面从不调用 `icons`、不显示“原程序菜单”。 |
| `features/residency.tsx` | 没有任何引用（见设置页对照）。 |

## 页面级

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| W1 | 两个标签：程序列表（List）、启动设置（Power）；切换经过未保存保护 | `kit.Tabs` + `env.RequestChange` | `TestTabsAndProgramPagesGoThroughTheUnsavedChangeGuard`；真实窗口中英、浅深色截图 | 已验证 |
| W2 | 模块开启读 `wintray.snapshot`，关闭读 `wintray.settings.get`；模块开关变化时重新读取 | 按 `env.Snapshot()` 的 wintray 模块状态选择；每帧比较 | `TestDisabledModuleReadsItsConfiguration`；真实窗口：关闭与开启两种状态都读到设置 | 已验证 |
| W3 | 未读到设置：“正在连接程序管理模块”；读取失败：错误 +“重试” | 状态文字 + `kit.Skeleton` / `kit.Notice` | `TestLoadingSaysTheModuleIsBeingReached`、`TestReadFailureOffersRetry` | 已验证 |
| W4 | 有草稿时出错：错误通知 +“放弃修改并刷新”；`recoveryError`、`backgroundError` 也作为错误显示 | `kit.Notice` | `TestFailedSaveKeepsTheDraftAndOffersDiscard`、`TestRecoveryBlocksEditingUntilRecovered`；`backgroundError` 与 `recoveryError` 走同一路径，没有单独测试 | 已验证 |
| W5 | 模块关闭：“模块已关闭，设置将在开启后生效。” | `kit.Help` | `TestDisabledModuleReadsItsConfiguration`；真实窗口 | 已验证 |
| W6 | 模块开启且有 `recoveryError`：“恢复上次设置”→ `recover` 任务；此时所有编辑、保存、添加、排序禁用 | 按钮 + 禁用条件 | `TestRecoveryBlocksEditingUntilRecovered` | 已验证 |
| W7 | 模块开启且有 `startupError`：通知 +“重试登录任务”（忙碌、有修改或 recoveryError 时禁用）→ `startup.retry` | `kit.Notice` 动作 | `TestStartupErrorRetriesTheSignInTask`、`TestFailedSaveKeepsTheDraftAndOffersDiscard` | 已验证 |
| W8 | 草稿与已保存设置不同即未保存；忙碌（任务、添加、更换、清空）也受保护；放弃 = 重新读取 | `Activity.Report("wintray", dirty, busy, reload)` | `TestAddedProgramsTakeTheirDefaultsAndTheLastIsSelected`、`TestFailedSaveKeepsTheDraftAndOffersDiscard` | 已验证 |
| W9 | 保存：右下按钮组“保存修改”；忙碌、未修改、数值无效、名称为空或有 recoveryError 时禁用；`settings.save {revision, settings}` | `env.Rail.Set` | `TestSaveSendsTheWholeDraftOnceAndReadsAgain`（未知字段保留）、`TestInvalidNumbersStayAsTypedAndBlockSaving`、`TestEmptyNameBlocksSavingAcrossTabs`；真实窗口：真实 Host 保存 4 个程序（修订号 9→10），Worker 接受 | 已验证 |
| W10 | 撤销：重新读取，忙碌或未修改时禁用 | Rail Undo | `TestUndoReadsTheSavedSettingsAgain` | 已验证 |
| W11 | 任务跟进：`state=running` 无 ID →“无法确认操作结果”；每 250 ms `jobs.get`，ID 不符→“无法确认操作结果”；5 分钟仍在运行→“任务仍在处理中…”；非 completed → 任务错误或“操作失败” | `runJob` | `TestLaunchFollowsItsJobAndReportsFailures`（无 ID、失败原因、完成）、`TestRunningProgramIsClosedOnlyAfterConfirming`。ID 不符与 5 分钟上限只做了代码核对 | 部分验证 |
| W12 | 成功提示：程序已启动/窗口状态已更新/程序已关闭/设置已恢复/已保存，然后重新读取；失败：显示错误，只更新三个错误字段（保留草稿） | `env.Toast`；`reload(preserve)` | 各提示分别见 `TestLaunchFollowsItsJobAndReportsFailures`、`TestUnreadableStateOffersRetryAndTheWindowFollowsIt`、`TestRunningProgramIsClosedOnlyAfterConfirming`、`TestRecoveryBlocksEditingUntilRecovered`、`TestSaveSendsTheWholeDraftOnceAndReadsAgain`；失败见 `TestFailedSaveKeepsTheDraftAndOffersDiscard`；真实窗口保存后出现“Saved” | 已验证 |
| W13 | 状态文字：处理中“正在处理”，有修改“有未保存的修改” | 列表上方工具行 | `TestAddedProgramsTakeTheirDefaultsAndTheLastIsSelected`；真实窗口：添加时显示“Working”，添加后“Unsaved changes” | 已验证 |
| W14 | 数值无效：“请检查数值：程序名…、启动设置”；名称为空：“请在程序信息中填写名称：…” | `kit.InlineError` | `TestInvalidNumbersStayAsTypedAndBlockSaving`、`TestEmptyNameBlocksSavingAcrossTabs` | 已验证 |

## 程序列表

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| L1 | “添加程序”（主按钮，只在程序列表标签）：多选对话框（exe/bat/cmd/ps1/py/pyw）；读取名称与图标（最多 4 个并行），任一失败则报错且不添加；新程序默认值（exe：收进托盘；脚本：后台启动；开机启动开；计划关、自动退出 30 分钟、1 天 1 次）；选中最后一个；进行中页面忙碌 | `env.Dialogs.PickFiles`；`desktop.ProgramInfo` | `TestAddedProgramsTakeTheirDefaultsAndTheLastIsSelected`；真实窗口：经真实多选对话框添加 3 个 exe 与 1 个 ps1，名称来自文件说明，图标正确，选中最后一个 | 已验证 |
| L2 | 拖入文件添加（只在程序列表标签、有草稿、非忙碌、无 recoveryError）；只接受文件，文件夹报“此处只接收文件，原内容已保留。” | 页面根 `widgets.DropArea` | `TestDroppedFilesAreAddedAndFoldersRefused` | 已验证 |
| L3 | 两栏：列表默认 40%，可拖动分隔条调整（列表 ≥28%、详情 ≥30%），分隔条名称“调整列表与设置宽度”，可用方向键；两栏各自滚动；工作区窄于 860 时改为上下排列并隐藏分隔条 | `kit.Split`（新增共享组件）；窄时上下排列 | `kit/split_test.go`（拖动、方向键、上下限）、`TestNarrowWindowStacksTheListAboveTheDetails`；真实窗口 1280 宽两栏、820 宽上下排列，均无横向溢出 | 已验证 |
| L4 | 列表表头：程序/收纳/更换/顺序（读屏忽略） | 表头行，三列与行内控件同宽、居中 | `TestDisabledModuleReadsItsConfiguration`；真实窗口中英表头对齐 | 已验证 |
| L5 | 空列表：“还没有程序，添加后可设置启动方式” | 居中说明 | `TestDisabledModuleReadsItsConfiguration` | 已验证 |
| L6 | 行：图标（程序图标或 AppWindow 占位）、名称（省略号 + 完整提示）、托盘收纳开关（仅 exe 可用）、更换程序按钮（FolderOpen，提示“更换程序”，名称“更换 X 的程序”）、排序序号下拉（1…n，名称“X的排序序号”） | 自绘行；列宽 36/56/80、间距 16（同 `settings.css`） | `TestReplacingWithAScriptKeepsIdentityAndSchedule`、`TestProgramsReorderByDraggingAndByPosition`；真实窗口：脚本行的收纳开关禁用 | 已验证 |
| L7 | 点击行或 Enter/Space 选中；选中行有底色；读屏标为当前项 | `Focusable` + `Selected` | `TestRemovingSelectsTheFirstRemainingProgram`、`TestAProgramWithoutANameOpensOnItsInformation`；真实窗口点击选中。Enter/Space 只做了代码核对 | 部分验证 |
| L8 | 拖动行到另一行的位置排序；忙碌或 recoveryError 时不排序 | `Drag`/`ui.Drop` | `TestProgramsReorderByDraggingAndByPosition` | 已验证 |
| L9 | 显示名：保存的名称优先；名称为空时用读到的名称、文件名或“名称” | `displayName` | `TestEmptyNameBlocksSavingAcrossTabs`、`TestAProgramWithoutANameOpensOnItsInformation` | 已验证 |
| L10 | 程序名称与图标：版本信息 FileDescription/ProductName；Shell 图标渲染为 32×32 透明 PNG；按大小与修改时间缓存到配置目录 `cache/program-icons`；读取失败保留已有图标 | `desktop/programinfo_windows.go` 移植旧 Go 窗口实现 | `desktop/programinfo_windows_test.go`（读取真实 notepad.exe 的说明与图标并写缓存；脚本用文件名；文件夹报错）；真实窗口：重新打开页面时图标来自缓存 | 已验证 |

## 程序详情

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 未选中：“选择一个程序编辑设置” | 说明 | `TestDisabledModuleReadsItsConfiguration` | 已验证 |
| D2 | 标题：名称（可换行）+ 路径（省略号 + 完整提示） | `kit.Heading` + `kit.Mono` | 真实窗口截图 | 已验证 |
| D3 | “立即启动”（Play）/运行中“关闭程序”（Square）；关闭前确认对话框 → `program.stop {id, confirmed:true}`；启动 → `launch {id}` | `kit.Dialog(Danger)` | `TestRunningProgramIsClosedOnlyAfterConfirming`、`TestLaunchFollowsItsJobAndReportsFailures`；真实窗口：未保存的新程序按钮禁用，保存后可用（未点击） | 已验证 |
| D4 | “显示窗口”（PanelTop）/“隐藏窗口”（PanelTopClose）→ `program.window {id, visible}`；没有窗口时禁用并提示“没有可显示的原生窗口” | 按钮 + Tooltip | `TestUnreadableStateOffersRetryAndTheWindowFollowsIt`（请求体与提示）；真实窗口：程序未运行时“显示窗口”禁用 | 已验证 |
| D5 | 每 1.5 秒读 `program.state {id}`（模块关闭、有修改、recoveryError 或忙碌时停止；窗口隐藏时暂停）；失败显示“无法读取程序状态”+重试图标按钮 | 帧驱动（`c.Now`/`c.After`） | `TestUnreadableStateOffersRetryAndTheWindowFollowsIt`、`TestRunningProgramIsClosedOnlyAfterConfirming`；真实窗口：真实 Worker 返回未运行 | 已验证 |
| D6 | “移除程序”：从草稿删除，选中剩下的第一个 | 按钮 | `TestRemovingSelectsTheFirstRemainingProgram` | 已验证 |
| D7 | 子标签“启动与计划/程序信息”；切换程序时重新决定初始子标签（有名称→启动与计划，否则程序信息）；切换经过未保存保护（React 的 Tabs 默认经过） | `kit.SubTabs` + `env.RequestChange` | `TestAProgramWithoutANameOpensOnItsInformation`、`TestTabsAndProgramPagesGoThroughTheUnsavedChangeGuard` | 已验证 |
| D8 | “启动行为”（Rocket）：开关“开机启动此程序”；“启动方式”：exe 为收进托盘/后台启动/开机即启动/正常启动，脚本只有后台启动；收进托盘时“收起前等待（秒）”0–600 | `kit.Toggle` + `kit.Segmented`（按内容宽度，放不下时在面板内换行）+ 数字字段 | `TestSaveSendsTheWholeDraftOnceAndReadsAgain`、`TestReplacingWithAScriptKeepsIdentityAndSchedule`、`TestStartupChoicesWrapInsideANarrowPanel`；真实窗口：exe 四种、脚本一种，820 宽英文换行不越出面板 | 已验证 |
| D9 | “时间计划”（CalendarClock）：开关“自定义时间”；开启后“延后启动（分钟）”0–1440、“自动退出（分钟，0 为不退出）”0–1440；开关“限制自动运行次数”（未开自定义时间时禁用并说明）；开启后“最近天数”1–365、“最多运行次数”1–1000 | 同上 | `TestScheduleFieldsFollowTheirSwitches`（断言保存的计划） | 已验证 |
| D10 | 程序信息：“名称”必填（为空时标红并显示原因）、“启动参数（可选）” | `ui.TextInput` + `Error` | `TestEmptyNameBlocksSavingAcrossTabs`；真实窗口截图 | 已验证 |
| D11 | 数字字段：未完成或超范围的输入原样保留并显示“请输入 a–b 的整数”，禁止保存（即使字段已隐藏） | 页面内数字字段：保存原文，`Error` 标红，上下键步进 | `TestInvalidNumbersStayAsTypedAndBlockSaving` | 已验证 |
| D12 | 更换程序：选一个文件；同一路径（不区分大小写与斜杠）不变；否则保留 id、参数、计划，名称取新文件，关闭托盘收纳；换成脚本时强制后台启动 | `env.Dialogs.PickFile` | `TestReplacingWithAScriptKeepsIdentityAndSchedule` | 已验证 |

## 启动设置

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| A1 | “启动行为”（Rocket）：开关“登录任务完成后，无常驻功能时退出 WinOne” | `kit.Toggle` | `TestScheduleFieldsFollowTheirSwitches`（保存 `exitAfterManagedAppsCompleted`）；真实窗口截图 | 已验证 |
| A2 | “等待时间”（Timer）：“程序之间的启动间隔（秒）”0–120、“最长等待程序窗口（秒）”0–120 | 数字字段 | `TestInvalidNumbersStayAsTypedAndBlockSaving`；真实窗口截图 | 已验证 |
| A3 | “清空托管程序并还原”（忙碌、有修改或 recoveryError 时禁用）→ `modules.resetWinTray.preview` → 确认对话框（含数量）→ `modules.resetWinTray {configurationRevision, moduleRevision, confirmed:true}`；成功“托管程序已清空，模块已关闭”；失败通知；结束后重新读取并刷新外壳；进行中页面忙碌 | `kit.Dialog(Danger)` | `TestResetAsksWithTheCountAndTurnsTheModuleOff`；真实窗口：真实 Host 预览数量 4，确认后清空程序、关闭模块、Worker 退出，页面显示模块已关闭 | 已验证 |

## 与 React 的差异及原因

1. 成功提示改用 Toast；“正在处理/有未保存的修改”放在列表上方工具行（React 在保存按钮旁，MyGo 的保存按钮在右下按钮组）。
2. 托盘收纳开关旁不重复显示“托盘收纳”文字，与 React 设置界面一致（`settings.css` 隐藏该标签，控件列宽 2.25/3.5/5rem）；开关保留读屏名称与提示。
3. 选中行只用窗口的选中底色，不加 React 的左侧 2px 强调色条。
4. “启动方式”选项按内容宽度显示，放不下时在面板内换行；React 在宽度 ≤28rem 时改为单列。
5. 数字字段是页面内实现（原文保留、上下键步进、范围提示），没有改动共享组件。
6. 程序状态轮询由帧驱动，窗口隐藏时不构建帧，所以不读取；React 用 `visibilitychange`。
7. 清空确认对话框沿用共享 `kit.Dialog`，点击遮罩会取消；React 设置了 `disablePointerDismissal`。
8. 子标签切换经过未保存保护，与 React `Tabs` 的默认行为一致。

## 未验证范围

- 真实窗口没有启动、关闭任何程序，也没有显示或隐藏程序窗口：这些会在用户桌面上打开窗口，只用单元测试验证。
- 真实窗口没有开启并保存托盘收纳：开启后 Worker 会接入资源管理器的任务栏。
- 行的拖动排序与分隔条拖动只在单元测试中验证（截图工具不能拖动）。
- W11 的 ID 不符与 5 分钟上限、L7 的 Enter/Space 选中：只做了代码核对。

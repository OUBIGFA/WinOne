# 设置页（settings）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/components/settings-panel.tsx`、`appearance-settings.tsx`、`default-page-setting.tsx`、`startup-settings.tsx`、`permission-settings.tsx`、`sidebar-preference.ts`、`settings-layout.tsx`，`features/shortcuts.tsx`、`profiles.tsx`、`profile-import.tsx`、`profile-settings-review.tsx`、`updates.tsx`、`diagnostics.tsx`，以及 `main.tsx` 中设置页的接线（主题、语言、起始页、启动偏好、管理员偏好与重启、连接状态）。旧窗口桥接：`apps/desktop-rust/src/component_bridge.rs`、`component_ui.rs`、`permissions.rs`、`profile_transfer.rs`、`diagnostics.rs`、`dialogs.rs`，旧 Go 窗口 `apps/desktop/updates_windows.go`、`diagnostics_windows.go`。MyGo 实现：`apps/desktop-mygo/pages/settings`（页面）与 `apps/desktop-mygo/desktop`（管理员辅助进程、退出本窗口、打开文件夹）。

状态：**已验证** = 有单元测试或真实窗口证据；**部分验证** = 列出的部分有证据，其余见“未验证范围”；**待验证** = 已实现但没有证据。

证据写法：
- 测试名：未注明文件的在 `pages/settings/settings_test.go`；`desktop/` 开头的在 `apps/desktop-mygo/desktop/`。
- “真实窗口”：2026-10-31 在仓库 `_temp/` 下的隔离配置（`--data-dir` 指向复制的测试配置，独立管道）里运行 MyGo 窗口和真实 Host 核对；覆盖简体中文与 English、浅色与深色、1280 与 820 宽窗口。截图未入库。

## 不属于设置页的内容（不移植）

| React 内容 | 判断依据 |
| --- | --- |
| `components/repository-link.tsx`（“GitHub 仓库（示例）”按钮） | 除自身测试外没有任何引用，旧设置页不显示。 |
| `features/residency.tsx`（后台运行状态，每 3 秒读 `residency.snapshot`） | 除定义外没有任何引用，旧设置页与程序管理页都不显示。 |

## 页面级

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| S1 | 五个一级标签：常规、启动与快捷键、运行权限、配置迁移、关于与诊断（Palette、Power、ShieldCheck、FolderSync、Info 图标）；切换经过未保存保护 | `kit.Tabs` + `env.RequestChange` | `TestTabsGoThroughTheUnsavedChangeGuard`；真实窗口：中英、浅深色、820 宽下五个标签都完整显示，无横向溢出 | 已验证 |
| S2 | 标签内容保持挂载：快捷键草稿、导入审阅在切换标签后保留 | 草稿保存在页面状态；快捷键编辑器隐藏时每帧 `Report()` | 导入审阅：`TestImportReviewsMapsAndAppliesTheArchive`（切到“常规”再回来，审阅与已选位置仍在）。快捷键草稿由共享 `widgets.Shortcuts` 保存，本页没有单独测试 | 部分验证 |
| S3 | “关于与诊断”第一次打开时才读取版本与日志，之后保留 | `aboutOpened` 标志，首次显示时读取 | `TestAboutReadsVersionAndLogsWhenFirstOpened`；真实窗口显示 Host 版本 0.0.1 与隔离配置的日志 | 已验证 |
| S4 | 偏好操作失败：标签下方显示“操作未完成”+ 错误原文 +“重试连接”（重新读取快照）；“连接状态”显示“未连接” | `kit.Notice(Failure)`；重试读 `snapshot` 后 `env.Refresh()` | `TestFailedPreferenceOffersReconnectAndMarksTheConnectionLost` | 已验证 |
| S5 | 保存进行中显示“正在保存…”；成功显示“已保存” | 状态文字；成功用 `env.Toast`，提示用刚保存的语言 | `TestSavingDisablesThePreferencesAndKeepsThePageBusy`、`TestThemeSavesOnlyItsFieldOverTheCurrentSettings`、`TestLanguageChangeIsConfirmedInTheNewLanguage`；真实窗口：切换主题、语言后出现“已保存”/“Saved” | 已验证 |
| S6 | 偏好保存进行中禁用常规、启动与快捷键、配置迁移的全部控件（fieldset），快捷键的保存按钮也随之禁用 | 内容列 `Disabled(busy)`；覆盖 Rail 的 Save/Undo 为禁用；`change` 在保存期间不再发送 | `TestSavingDisablesThePreferencesAndKeepsThePageBusy`（保存期间再选一项不发送请求）。Rail 覆盖只做了代码核对 | 部分验证 |
| S7 | 偏好保存期间页面忙碌（活动来源 `global-settings`），离开页面等待完成 | `env.Activity.Report("global-settings", false, working, nil)` | `TestSavingDisablesThePreferencesAndKeepsThePageBusy`（保存期间 `Activity` 为忙碌，结束后恢复） | 已验证 |

## 常规

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| G1 | 主题（Palette）：跟随系统/浅色/深色；选择后立即保存：点击时重新读取快照，只改 `theme`，`settings.save`（保留其他字段与修订号）；忙碌或未读到设置时禁用 | `kit.Segmented`；`core.Go` 中 `snapshot` → `settings.save`（CallCompleting）；成功后更新本页设置并 `env.Refresh()` | `TestThemeSavesOnlyItsFieldOverTheCurrentSettings`（未知字段 `future` 保留、修订号取最新快照）；真实窗口：经真实 Host 在深色与浅色之间切换，配置文件 `theme` 随之改变 | 已验证 |
| G2 | 语言（Languages）：跟随系统/简体中文/English；保存后重新读取快照，界面语言随之切换 | 同 G1，保存后 `env.Refresh()` | `TestLanguageChangeIsConfirmedInTheNewLanguage`、`TestFailedPreferenceOffersReconnectAndMarksTheConnectionLost`；真实窗口：中英互换后整个窗口切换语言 | 已验证 |
| G3 | “打开界面时”（PanelLeft）·“侧栏”：只显示图标/显示图标和名称；写入窗口偏好，当前侧栏立即折叠或展开；写入失败显示“侧栏设置保存失败，请重试。”并保持原选择，再次成功后清除提示；忙碌时禁用 | `kit.Segmented` + `env.SetSidebarStartup` | `TestSidebarStartupFollowsTheShellAndReportsAFailedWrite`；真实窗口：选择后侧栏立即展开/折叠，偏好写入隔离配置的 `desktop-ui.json` | 已验证 |
| G4 | “起始页面”：下拉列出侧栏全部页面（与侧栏同序，带图标，长名称不截断丢失）；当前值不在目录中时显示首页；选择后立即保存 `defaultPage` | `kit.Select`，选项来自 `shell.Catalog()`；保存同 G1 | `TestStartPageListsTheSidebarAndSavesTheChoice`；真实窗口：列出 12 个页面并带图标，选“程序管理”后真实 Host 保存 `defaultPage: wintray`，再改回首页 | 已验证 |

## 启动与快捷键

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| T1 | “启动与界面”（Power）：登录 Windows 时启动、静默启动到托盘、保持界面后台运行（未设置时视为开），各带说明；切换后立即保存，只提交改变的字段，合并到最新快照的 `startup`（缺省 `runAtLogon:false,startMinimized:true`） | `kit.Toggle` ×3；`snapshot` → 合并 → `settings.save` | `TestStartupSwitchesMergeIntoTheCurrentPreferences`（断言请求体）；真实窗口只核对显示，未切换（避免触及真实自启） | 已验证 |
| T2 | “快捷键”：“打开 WinOne”录入（默认 Alt+A）、注册状态、清除、录制时暂停注册、保存/撤销放到右下按钮组、“重试启用”、“重新读取快捷键”、动作失败提示 | 共享 `widgets.NewShortcuts(env, "application")`；标签显示时 `View`，隐藏时 `Report`，页面关闭 `Close` | 接入：`TestStartupSwitchesMergeIntoTheCurrentPreferences`（显示“打开 WinOne”“Alt+A”）、`TestAdministratorActionsWaitForUnsavedShortcuts`；真实窗口显示录入框、状态与“清除”。录制与注册行为由共享组件负责，不在本页测试 | 部分验证 |

## 运行权限

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| P1 | 当前权限：正在读取权限/当前以管理员身份运行/当前以普通权限运行 | 快照 `administrator`；页面未读到设置时显示“正在读取权限” | `TestPermissionTextsFollowTheCurrentRights`、`TestTabsGoThroughTheUnsavedChangeGuard`；真实窗口显示“当前以普通权限运行” | 已验证 |
| P2 | 开关“始终以管理员身份运行”+“首次启用需要管理员授权。”；忙碌、未读到设置、页面有未保存修改或进行中的操作时禁用 | `kit.Toggle` + 禁用条件读 `env.Activity.State()` | `TestAdministratorPreferenceRunsTheHelperAndKeepsItsResult`、`TestAdministratorActionsWaitForUnsavedShortcuts` | 已验证 |
| P3 | 已记住但当前非管理员：“重新启动 WinOne 后生效。”；未记住但当前是管理员：“从托盘退出后，下次启动将使用普通权限。” | 写在开关说明里 | `TestAdministratorPreferenceRunsTheHelperAndKeepsItsResult`、`TestPermissionTextsFollowTheCurrentRights` | 已验证 |
| P4 | 按钮“以管理员身份重新启动”（ShieldCheck），进行中“正在重新启动”并转圈；已是管理员或状态未知时禁用 | `kit.Button` | `TestRestartClosesTheWindowOnlyAfterTheHandoff`、`TestPermissionTextsFollowTheCurrentRights`（管理员时点击不启动辅助进程）；真实窗口截图 | 已验证 |
| P5 | 修改偏好与重启都通过 `WinOne.Host.exe --desktop-control`：帧协议（4 字节长度 + JSON）、序号校验；“guard”询问窗口是否没有未保存修改；只中继 `snapshot`、偏好时一次 `settings.save`、重启时 `application.restart.prepare/status/commit/cancel`；完成结果校验（偏好：保存结果与返回设置一致且 `preferAdministrator` 等于请求值；重启：已提交并确认的同一令牌，或已是管理员时返回当前设置）；300 秒上限、窗口关闭时取消并结束辅助进程 | `desktop/permissions_windows.go` 移植 `permissions.rs`（`exchange` 与进程管理）；读写线程卡住时用 `CancelSynchronousIo` 解除，再结束进程 | `desktop/permissions_windows_test.go` 的 7 个测试（取消不启动、偏好中继与结果校验、关闭偏好、重启令牌、失败不重试、错误原样返回、卡住的辅助进程在上限时结束）；真实窗口：运行真实辅助进程，它按设计拒绝隔离配置并显示“独立测试配置不允许更改管理员授权 (permission)” | 已验证 |
| P6 | 偏好成功：更新设置、提示已保存；重启 `closing=true`：窗口退出（Host 等待本进程结束后交给管理员实例）；`closing=false`：重新读取快照 | 成功后 `env.Refresh()`；退出：向本窗口发送 `WM_CLOSE(wParam=1)`（Host 托盘退出所用的退出协议） | `TestAdministratorPreferenceRunsTheHelperAndKeepsItsResult`、`TestRestartClosesTheWindowOnlyAfterTheHandoff`（两条路径，退出用替身）。真实的提权交接只能在默认配置下经 UAC 验证 | 部分验证 |
| P7 | 失败原因显示在按钮下方 | `kit.InlineError` | `TestAdministratorPreferenceRunsTheHelperAndKeepsItsResult`；真实窗口显示辅助进程的拒绝原因 | 已验证 |

## 配置迁移

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| E1 | “导出配置”（Download）+ 说明；“导出到文件”：保存对话框（标题“导出配置”、类型“WinOne 配置 (*.winone)”、默认名 WinOne.winone）→ `profiles.export {path, name:"我的配置"}`；取消不提示；成功“已导出：路径”；失败通知；进行中按钮转圈、页面忙碌 | `env.Dialogs.SaveFile`；无扩展名时补 `.winone`，补后文件已存在则拒绝并提示 | `TestExportAddsTheExtensionAndReportsTheFile`；真实窗口：经真实保存对话框与 Host 导出 45 KB 的 `.winone`，显示“已导出：…” | 已验证 |
| I1 | “导入配置”（Upload）+ 说明；“选择配置文件”（打开对话框，*.winone）；整组接受拖入单个文件 | `env.Dialogs.PickFile`；组容器 `widgets.DropArea`（单个文件） | `TestImportReviewsMapsAndAppliesTheArchive`、`TestDroppedArchiveIsReviewedAndFoldersAreRefused`；真实窗口经打开对话框选择刚导出的文件 | 已验证 |
| I2 | 预览：`profiles.import.preview.start` → 分块 `read` → SHA-256 校验 → 总是 `release`；30 秒上限 | `previewtransfer.Receive` | `TestImportReviewsMapsAndAppliesTheArchive`（至少两块、释放一次）及失败时释放的检查；真实窗口：真实 Host 预览成功 | 已验证 |
| I3 | 审阅区：配置名称（缺省“导入配置”）+ 源路径；子标签“导入内容/详细设置/本机路径”（不经未保存保护） | `kit.SubTabs`（request 为 nil） | `TestImportReviewsMapsAndAppliesTheArchive`、`TestDetailsShowTheModuleSettingsInTheArchiveOrder`；真实窗口 | 已验证 |
| I4 | 导入内容：“全局偏好”复选框 + 摘要；每个模块复选框（名称本地化）+ 状态（选中时附“导入后开启/关闭”）；程序管理选中时“将替换当前程序列表（n 个程序）” | `ui.Checkbox` | `TestImportReviewsMapsAndAppliesTheArchive`；真实窗口：8 个模块、状态与“将替换当前程序列表（4 个程序）” | 已验证 |
| I5 | 详细设置：“模块”下拉（只含已选且有设置的模块）+ 设置审阅（字段名本地化，未知字段显示原键；布尔“开启/关闭”；数组列表或“无”；嵌套对象；路径引用解析；动作脚本原样显示；截图的 openEditor、fileNameTemplate、toolbarTools 专门说明）+ 该模块快捷键；没有则“所选内容没有可预览的模块设置。” | `kit.Select` + 递归审阅（`review.go`） | `TestDetailsShowTheModuleSettingsInTheArchiveOrder`；真实窗口：置顶功能的 9 项设置与快捷键 Alt+Q | 已验证 |
| I6 | 本机路径：无路径“所选内容无需指定本机路径。”；有路径时标题“选择本机位置”，每行名称、当前值或“尚未选择”、错误、“选择位置”（file/directory/program 三种对话框）；每行接受拖入；选好后带映射重新预览 | `env.Dialogs.PickFile/PickFolder`；行 `widgets.DropArea`；路径整条换行显示（同 React 的 `overflow-wrap:anywhere`） | `TestImportReviewsMapsAndAppliesTheArchive`；真实窗口：经真实文件夹与文件对话框选定 5 个位置，长路径完整换行 | 已验证 |
| I7 | 不在“本机路径”且有未就绪路径时显示“设置本机路径”，切到该子标签 | 按钮 | `TestImportReviewsMapsAndAppliesTheArchive`；真实窗口 | 已验证 |
| I8 | 问题列表（去重）：模块快捷键问题、“{name}包含当前版本不支持的快捷键”、按“保留+替换”的组合重新检查“多个操作使用了相同快捷键” | 同算法 | `TestImportReviewsMapsAndAppliesTheArchive`（冲突随所选模块出现和消失） | 已验证 |
| I9 | “确认导入”（主按钮，转圈）：条件不满足时禁用；`profiles.import.apply {path,bindings,sourceHash,configurationRevision,moduleRevision,modules,global}`；成功清除审阅、显示“配置已导入”、刷新外壳 | `CallCompleting` | `TestImportReviewsMapsAndAppliesTheArchive`（断言请求体）；真实窗口：真实 Host 应用成功，配置修订号 13，程序路径与截图目录按映射写入 | 已验证 |
| I10 | “重新预览”（同文件同映射）、“取消”（清除审阅与错误） | 按钮 | `TestReplacementFailureKeepsTheReviewUntilCheckedAgain`；真实窗口“取消”后审阅消失，外壳刷新按钮恢复 | 已验证 |
| I11 | 换文件失败：保留旧审阅与映射，需“重新预览”后才能确认；映射或应用失败同样需要重新预览；错误通知 | `needsReview` 标志 | `TestReplacementFailureKeepsTheReviewUntilCheckedAgain` | 已验证 |
| I12 | 有审阅时视为未保存（离开询问，放弃时清除）；导入或预览进行中视为忙碌 | `Activity.Report("profile-import", preview!=nil, busy, discard)` | `TestReplacementFailureKeepsTheReviewUntilCheckedAgain`（审阅期间为未保存，结束后清除） | 已验证 |

## 关于与诊断

| 编号 | React 入口/行为 | MyGo 实现 | 证据 | 状态 |
| --- | --- | --- | --- | --- |
| U1 | “更新”（RefreshCw）：“当前版本：x”；“检查更新”（未配置更新来源时禁用，检查中转圈）；检查中显示“取消检查”；未配置或结果为 not_configured 时“此版本暂未提供在线更新。” | Host 的 `updates.info/check/status/cancel/release`（与 Rust 窗口一致） | `TestAboutReadsVersionAndLogsWhenFirstOpened`、`TestUpdateCheckFollowsTheHostAndOpensOnlyItsRelease`；真实窗口：版本 0.0.1、“此版本暂未提供在线更新。”、检查按钮禁用 | 已验证 |
| U2 | 结果：发现新版本 + “查看发行说明”（只打开本次检查确认的 GitHub 发布页）；已是最新；错误码文字，取消显示“已取消检查” | `updates.release` 返回地址校验后用 Shell 打开 | `TestUpdateCheckFollowsTheHostAndOpensOnlyItsRelease`（其他域名不打开）、`TestUpdateErrorsAndCancel` | 已验证 |
| U3 | 离开页面时取消进行中的检查 | `Close()` 发送 `updates.cancel` | `TestUpdateErrorsAndCancel` | 已验证 |
| B1 | “后台”（Activity）：连接状态行 + “关闭设置窗口后，后台继续运行” + 已连接/未连接 | `kit.Row` | `TestFailedPreferenceOffersReconnectAndMarksTheConnectionLost`；真实窗口“已连接” | 已验证 |
| D1 | “诊断日志”（FileText）：“刷新日志”、“打开日志文件夹”（无文件夹或正在打开时禁用）；“正在读取日志”；读取、打开错误通知；无日志“暂无日志”；每个日志标题、截断说明、日志正文（可聚焦、可选择复制） | `diagnostics.Read/Folder(env.Directory, "host")`；Shell 打开文件夹；只读 `TextArea` | `TestAboutReadsVersionAndLogsWhenFirstOpened`、`TestMissingLogsShowEmptyWithoutAFolder`；真实窗口显示隔离配置的 host 日志（未点击“打开日志文件夹”，避免在桌面打开资源管理器窗口） | 已验证 |

## 与 React 的差异及原因

1. 保存成功的提示改用 Toast（`env.Toast`）。切换语言后的提示用新语言显示；React 当时仍用旧语言。
2. 更新检查走 Host 的 `updates.*` RPC，与 Rust 窗口一致，窗口进程不直接访问 GitHub；状态每 100 ms 读取一次，1 分钟仍未结束时取消。
3. 导出：MyGo 保存对话框不设默认扩展名，所以没有扩展名时补 `.winone`；补后的文件已存在时拒绝并提示，避免未经确认覆盖。对话框只有“WinOne 配置 (*.winone)”一种类型，没有“所有文件”。
4. 导入审阅中的定时关机模块显示为“定时关机”；React 显示原始 ID `timed-power`。
5. 管理员辅助进程：Rust 在关闭“始终以管理员身份运行”时，把返回设置中缺少的 `preferAdministrator` 当作不一致；这里按 false 处理，修正该缺陷。辅助进程询问 guard 时，本页只能看到页面活动与本页状态，看不到外壳层面的待关闭状态。
6. “连接状态”只反映本页读取与保存的结果；外壳刷新失败由外壳自己提示。

## 未验证范围

- P6：`closing=true` 的真实提权交接（需要默认配置、UAC 与管理员实例），只用替身验证了窗口会退出。
- T1：真实窗口中没有切换“登录 Windows 时启动”，以免触及用户的真实自启。
- U2：本构建没有配置更新来源，“发现新版本”与打开发布页只在单元测试中验证。
- D1：真实窗口中没有点击“打开日志文件夹”。
- S2、T2：快捷键草稿与录制由共享 `widgets.Shortcuts` 负责，本页没有单独测试。

# 便捷功能页（convenience）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/features/convenience.tsx`、`modules.tsx`（`only=` 分支）、`components/module-panel.tsx`、`always-on-top.tsx`、`shortcuts.tsx`、`timed-power.tsx`、`mouse-gestures*.tsx`、`mouse-gestures-scenes-model.ts`、`directory-links*.tsx`、`directory-link-intents.ts`，以及 `main.tsx` 的挂载方式。MyGo 实现：`apps/desktop-mygo/pages/convenience`；共享部分：`widgets/modules.go`（模块开关）、`widgets/shortcuts.go`（快捷键编辑器）、`widgets/droparea.go`、`kit/number.go`、`shell`（Explorer 预填）。

状态：**已验证** = 有单元测试或真实窗口截图证据；**待验证** = 已实现，还缺证据。截图证据保存在本地 `_temp/mygo-frontend-20261030/conv-*.png`（不入库）。

## 有意改变的地方

| 改变 | 原因 |
| --- | --- |
| 数字输入框统一为“单位在框内右侧、弱化”样式，去掉 MyGo 默认的 −/+ 按钮 | 与定时关机三段输入的既有要求一致；方向键上下可步进。输入只在合法范围内生效，不会保存非法值。 |
| 链接类型徽标改为中性灰 | 遵守全局黑白灰约定；类型名称文字仍显示。名称冲突仍用红色边条标出（错误提示）。 |
| “已保存”类提示改为 Toast；批量创建结果、导入结果等需要阅读路径的提示仍为信息条 | 简单结果不占页面空间；含路径的结果需要保留。 |
| 设置项整行可点击切换开关 | 与 React 的 `<Label htmlFor>` 行为一致（点标签文字也切换）。 |

## 页面级

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| V1 | 四个标签：置顶功能、鼠标手势、文件与目录链接、定时关机，切换经未保存保护；离开的标签卸载 | `kit.Tabs` + `env.RequestChange`；`page.mount` 关闭旧标签（撤回活动）并新建 | 截图 conv-1…conv-10 | 已验证 |
| V2 | 页面任一来源有未保存或忙碌时锁定各标签的模块开关 | `env.Activity.State()` → `locked` 传给 `Modules.Switch` | 代码审查 | 待验证 |
| V3 | 各标签顶部模块开关（`ModulesPage only=X`）：状态文字、开关、故障详情、重试关闭、恢复/重试安装包、配置错误恢复 | `widgets.Modules.Switch`（与首页共用生命周期代码） | 首页测试 + 截图 | 已验证 |
| V4 | Explorer“创建链接”预填：读取 `directory-links.pending`（启动、被唤醒、获得焦点时），经未保存保护打开便捷功能页；页面空闲时切到链接标签并交给工作区 | `shell.ReadIntents` + `env.TakeIntents("convenience")` + `linksTab.Queue` | 测试 `TestLinkPrefillCreatesWithOneStep` | 已验证 |

## 置顶功能

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| P1 | 模块关闭时读 `settings.get`，否则读 `snapshot.settings`；保留未知字段（如旧的 `enabled`） | `pinTab.reload` + `core.Preserved` | 测试 `TestPinningSavesEditsAndKeepsUnknownFields` | 已验证 |
| P2 | 常规：显示边框；颜色来源（Windows 强调色/自定义）；自定义颜色、粗细 1–15、不透明度 10–100、圆角，选强调色时整组禁用但保留草稿；数值越界时提示并禁止保存 | `pinTab.general` | 测试 + 截图 conv-2 | 已验证 |
| P3 | 操作反馈：播放提示音 | `kit.Toggle` | 测试（点击切换并保存） | 已验证 |
| P4 | 高级：游戏模式暂停；排除程序（每行一个，可拖入多个文件，去重追加） | `pinTab.advanced` + `widgets.DropArea` | 代码审查 | 待验证 |
| P5 | 快捷键：共享编辑器（切换窗口置顶），录制、清除、注册状态 | `widgets.NewShortcuts(env, "always-on-top")` | 截图 conv-10 | 已验证 |
| P6 | 保存“保存设置”（未修改或数值非法时禁用），撤销重读；未保存时离开受保护 | `env.Rail.Set`；活动来源 `always-on-top` | 测试 | 已验证 |

## 定时关机

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| T1 | 当前任务：状态、任务描述与截止时间、剩余时间（每秒刷新）、取消任务（执行中不可取消）、错过提示、错误与通知错误 | `powerTab.current`；运行中每秒读 `timed-power.snapshot`，操作开始后丢弃旧读数 | 测试 `TestTimedPowerConfirmsBeforeStarting` | 已验证 |
| T2 | 时间计划：倒计时/指定时刻/每周重复；时分秒三段输入，单位在框内右侧；快捷时长 15/30/60/90/120/150 分钟；每周选择星期 | `powerTab.schedule` + `kit.UnitNumber` + `presetChip` | 截图 conv-6 | 已验证 |
| T3 | 电源操作：六种操作；关机/重启/注销时显示“强制关闭程序”及说明 | `powerTab.power` | 截图 | 已验证 |
| T4 | 仅提醒：提前提醒秒数 0–3600；仅提醒不执行 | `powerTab.reminder` | 代码审查 | 待验证 |
| T5 | 校验失败提示；开始计时（仅提醒直接开始，否则先确认；强制时为危险确认） | 使用 Host 的 `timedpower.Preferences.Validate` 判断；`kit.Dialog` | 测试 | 已验证 |
| T6 | 保存/撤销模板，不影响正在运行的任务 | `env.Rail`；活动来源 `timed-power` | 代码审查 | 待验证 |

## 鼠标手势

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| G1 | 使用场景：资源管理器场景排在最前但不改写存储顺序；场景开关；场景动作列表（缩略轨迹、名称、动作包、被覆盖提示、手势下拉、启用开关、编辑） | `scenes.View` + `orderedScopes` + `sceneActions` | 测试 `TestGesturesOpenExplorerFirst…`、`TestScenesShowWhichActionWinsAGesture` + 截图 conv-3 | 已验证 |
| G2 | 编辑共享动作包时只为当前场景复制，32 个包上限；新增分配优先放入本场景独占的最后一个包 | `editScenePackage` / `addSceneAction` | 测试 `TestAssigningAGestureCopiesOnlyThisScenesPackage` | 已验证 |
| G3 | 场景设置对话框：名称、匹配方式、程序路径（拖入/拾取）、路径规则（拖入多个/拾取）、窗口类名、使用的动作包、删除场景 | `scenes.settingsDialog` | 代码审查 | 待验证 |
| G4 | 动作编辑对话框、分配手势对话框 | `scenes.editorDialog`、`scenes.assignDialog` | 代码审查 | 待验证 |
| G5 | 手势：列表（右键菜单添加/删除）、名称（去掉开头的 #）、预览（点击/右键录制，中键清空）、录制、清空、删除时一并移除引用动作 | `gesturesTab.trails` | 代码审查 | 待验证 |
| G6 | 录制对话框：`record.start/read/cancel`，按所选手势键绘制，混合学习开关，识别结果与匹配，使用此轨迹 | `gestureRecorder`；侧键 1/2 由 `GetAsyncKeyState` 读取（MyGo 不把侧键作为指针事件） | 代码审查 | 待验证 |
| G7 | 常规：手势键五选一；画笔开关（缺省视为开启，保存显式值）、颜色、不透明度（存为小数）、宽度 | `gesturesTab.pen` | 测试 `TestPenOffIsSavedWithoutChangingItsLook` | 已验证 |
| G8 | 排除：每行一个进程路径正则；拖入或拾取后转义追加 | `gesturesTab.filtersView` | 代码审查 | 待验证 |
| G9 | 高级·动作包：树（右键菜单）、包名称/上移/下移/删除、动作名称/启用/手势/脚本/API 说明/删除 | `gesturesTab.packagesView` | 代码审查 | 待验证 |
| G10 | 高级·编译：程序集（可拖入）、命名空间；高级·执行线程：线程数 1–64、超时 0–3600 | `compileView`、`threadsView` | 测试 `TestCompilerAndThreadSettingsStayEditable` | 已验证 |
| G11 | 保存时去掉空行与首尾空格（命名空间、程序集、排除、窗口类名）；保存“保存鼠标手势设置”；撤销 | `gesturesTab.save` + `committedLines` | 测试 | 已验证 |

## 文件与目录链接

| 编号 | React 行为 | MyGo 实现 | 验证 | 状态 |
| --- | --- | --- | --- | --- |
| L1 | 新建链接：源/目标两栏（地址、Enter 打开、浏览、上一级、刷新、分页加载、勾选当前文件夹、勾选项、文件夹进入），可拖动分栏，可拖动/键盘调整高度并记住 | `linkBatch` + `folderPane` + `kit.Split`；高度存于配置目录 `directory-links-ui.json` | 截图 conv-4、conv-8 | 已验证 |
| L2 | 添加来源菜单：选择文件、选择文件夹、粘贴路径列表；在源栏粘贴路径列表 | `linkBatch.addMenu`；源栏 Paste 命令 | 代码审查 | 待验证 |
| L3 | 目标栏显示计划项（名称冲突、将创建、待检查）与类型；F2/按钮重命名目标项（对话框，身份校验） | `folderPane.entryRow` + `linkBatch.renameDialog` | 代码审查 | 待验证 |
| L4 | 创建：一步完成预览与创建；单个来源可改链接名；类型选择（符号链接可选相对路径）；更多选项：管理员创建、保留脚本、成功时不提示、仅预览；计划导出 | `linkBatch.create/preview/exportPlan` | 测试 `TestLinkPrefillCreatesWithOneStep` | 已验证 |
| L5 | 拖入：拖到源栏或页面其他位置加入来源；拖到目标栏打开为目标文件夹（经 Host 核对）；拖到地址栏打开 | `dropSources`、`dropTarget`、页面根 `DroppedFiles` | 代码审查 | 待验证 |
| L6 | 查找已有链接：选择/拖入文件夹、包含子文件夹、查找、取消、结果与问题列表 | `linkScan` | 截图 conv-9 | 已验证 |
| L7 | 链接历史（只在“查找已有链接”中显示）：按目标分组（文件夹优先、数字按大小）、默认收起；检查选中/全部；一键删除失效历史；导出/导入备份（预览确认）；记录状态、原因、详情、迁移记录与操作；检查、打开位置、移除（确认对话框，发送检查时间）、忘记 | `linksTab.history` / `recordView` / `linkArchive` | 测试 `TestLinkHistoryForgetsOnlyInvalidRecords`、`TestLinkRemovalAsksAndSendsTheCheckedRecord`、`TestLinkGroupsPutFoldersFirstAndNumbersInOrder` | 已验证 |
| L8 | 搬迁与还原：源/目标/名称、预览、开始（警告需确认）、进度与取消、读取状态、还原/恢复/清理（来自历史，经未保存保护）；页面关闭时取消运行中的任务 | `linkMigration` | 代码审查 | 待验证 |
| L9 | 资源管理器集成开关（部分登记、冲突提示） | `linksTab.menuView` | 截图 conv-9 | 已验证 |

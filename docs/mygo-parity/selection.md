# 划词助手页（selection）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/features/selection.tsx`（`moduleId="selection"`，与快捷助手共用一个组件）、`selection.css`、`ai-model-select.tsx`、`ui/capture-option-select.tsx`、`ui/path-drop.ts`、`features/shortcuts.tsx`，以及 `main.tsx` 的挂载方式（`ModulePanel` + `disabled={module?.state==="disabled"}`）。Rust 桥：`component_bridge.rs` 的 `selection` 命名空间只允许 `snapshot`、`settings.get`、`settings.save`，都经 `call_completing`。MyGo 实现：`apps/desktop-mygo/pages/selection`（`selection.go`、`toolbar.go`、`activate.go`，同一个包注册 `selection` 和 `quick-assistant` 两页）。快捷助手的差异见 [quick-assistant.md](quick-assistant.md)。

状态：**已验证** = 有单元测试或真实窗口证据；**待验证** = 已实现但还没有证据。单元测试在 `pages/selection/selection_test.go`；截图在 `_temp/mygo-capture-selection/shots/`（简写 `shots/`），真实窗口使用隔离夹具（所有模块关闭）。

## 页面级

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| S1 | 打开时读 `selection.settings.get`；“正在读取划词设置”；失败提示 +“重试”（有未保存修改时不显示）；读取状态与错误在所有标签上方 | `page.load`；状态文字 + 占位行；`kit.Notice` | `TestLoadFailureShowsRetry`、`TestErrorsShowAboveEveryTab` | 已验证 |
| S2 | 标签：工具栏、常规、快捷键（带图标）；默认工具栏；切换经过未保存保护 | `kit.Tabs` + `env.RequestChange`（与截图页同一机制） | `shots/10-selection-zh-light.png`、`12`、`13`；保护机制见截图页 C5 | 已验证 |
| S3 | 未保存 = 草稿（排除应用按文本解析）与基线不同；活动来源 `selection`；放弃时重读 | `env.Activity.Report("selection", …, load)`；比较两份 JSON | `TestSaveSendsParsedExclusions`、`TestUndoReadsTheSettingsAgain` | 已验证 |
| S4 | 保存（工具栏、常规标签）：“保存设置”，`selection.settings.save` 发送草稿与解析后的排除应用（保留未知字段）；未修改或忙碌时禁用；成功更新草稿、基线和文本框并提示“已保存” | `env.Rail.Set` + `core.Preserved`；`env.Toast` | `TestSaveSendsParsedExclusions`；真实 Host `shots/72-selection-save-real-host.png`（只改了“复制”的显示，其余原样） | 已验证 |
| S5 | 撤销：重新读取 | Rail Undo → `load` | `TestUndoReadsTheSettingsAgain`；真实窗口 `shots/70-selection-undo-en-light-narrow.png` | 已验证 |
| S6 | 读取或保存中禁用全部控件 | 外层容器 `Disabled(busy)` | `TestHeadingsAndBusyDisabling` | 已验证 |

## 常规

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| S7 | 模块关闭时显示“划词助手已关闭” | 按 `env.Snapshot()` 显示 | `TestKeyboardDependsOnAutomatic`；`shots/12-selection-general-zh-light.png` | 已验证 |
| S8 | “显示方式”：开关“划词后显示工具栏”；从属开关“键盘选中文字后也显示”在前者关闭时禁用并显示说明 | `kit.Toggle` + `kit.Indented` | `TestKeyboardDependsOnAutomatic` | 已验证 |
| S9 | “排除应用”：多行输入（5 行起），说明；保存时按行拆分、去空白、丢弃空行 | `ui.TextArea` | `TestSaveSendsParsedExclusions` | 已验证 |
| S10 | 拖入文件：可多个，只接收文件，去重追加；错误提示同截图页 | `widgets.DropArea` + 页面去重。**偏差**：React 追加完整路径，Host 校验拒绝含 `\` 或 `:` 的项目，保存必然失败；MyGo 追加文件名（`filepath.Base`），待主 Agent 确认 | `TestDropFilesAppendsNames`。未在真实窗口拖放 | 已验证（单元） |

## 工具栏

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| S11 | 标题“功能与顺序”和说明 | `kit.SectionHeading` | `TestHeadingsAndBusyDisabling` | 已验证 |
| S12 | 每次显示工具栏标签时读 `capture.ai.configuration.get`；失败提示 +“重试”；之前读到的服务继续可选 | 切换到工具栏（含首次）时 `loadAI` | `TestToolbarReadsAIConfigurationEachVisit` | 已验证 |
| S13 | 列标题：功能 / 位置 / 显示（读屏忽略） | 面板首行，`RoleNone` | `TestHeadingsAndBusyDisabling`；`shots/10-selection-zh-light.png` | 已验证 |
| S14 | 每行：图标（未知用 Sparkles）+ 名称（英文界面优先 `englishName`；空名显示“新功能”）+ 展开箭头；点击展开编辑器，同时只展开一个；名称“编辑 {name}”；忙碌时禁用 | `ui.CollapsibleBase` 触发器 | `TestOnlyOneEditorOpensAndDraftsSurvive`；`shots/11-selection-editor-zh-light.png` | 已验证 |
| S15 | “位置”下拉 1..n，选择后移动该功能 | `kit.Select`；行全部构建完后再移动 | `TestPositionMovesAction` | 已验证 |
| S16 | “显示”开关，名称“显示 {name}” | `ui.Switch` | `TestUndoReadsTheSettingsAgain`、`TestHeadingsAndBusyDisabling`；真实 Host 保存 `shots/72-selection-save-real-host.png` | 已验证 |
| S17 | 编辑器：“功能名称”（中文界面改 `name`，英文界面改 `englishName`）；“图标”下拉 25 个图标 | `ui.Field` + `ui.TextInput`；`kit.Select`（带图标） | `TestEnglishWindowEditsTheEnglishName`、`TestQuickLoadsItsOwnSettingsAndSavesItsFields`（改图标） | 已验证 |
| S18 | 编辑器：非复制功能且 AI 配置已读到时显示模型选择（含“跟随 AI 服务默认设置”）；选跟随默认时删除 `selection` | `widgets.AIModelSelect` | `TestCustomActionFlow`、`TestFollowDefaultRemovesTheActionsModel`；`shots/picker-light.png` | 已验证 |
| S19 | 编辑器：非复制功能显示“AI 指令”多行输入 | `ui.Field` + `ui.TextArea` | `TestOnlyOneEditorOpensAndDraftsSurvive`、`TestCustomActionFlow` | 已验证 |
| S20 | 编辑器：自定义功能（ID 以 `custom-` 开头）显示“删除”，删除后收起 | 按钮；行全部构建完后再删除 | `TestRemoveCustomAction` | 已验证 |
| S21 | “添加自定义功能”：满 24 个时禁用；新增 `{id: custom-<uuid>, name: 新功能, icon: polish, kind: ai, prompt: "", enabled: true}`，展开、聚焦并全选名称、滚动到可见 | `crypto/rand` 生成小写 UUID v4；`AutoFocus` + `SetTextSelection` + `ScrollIntoView` | `TestCustomActionFlow`（ID 格式、聚焦、保存内容）、`TestAddDisabledAtLimit`；真实窗口 `shots/69-selection-add-en-light-narrow.png`（名称已全选并滚动到可见） | 已验证 |
| S22 | 收起、排序、切换标签都不丢草稿 | 草稿在页面状态中 | `TestOnlyOneEditorOpensAndDraftsSurvive`、`TestCustomActionFlow` | 已验证 |
| S23 | （新增）拖动行调整顺序；“位置”下拉保留为键盘方式 | 行头 `Drag`，各行 `ui.Drop`，插入位置细线；拖动不算点击 | `TestDragReordersActions`。未在真实窗口拖动 | 已验证（单元） |

## 快捷键

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| S24 | “显示划词工具栏”快捷键字段、注册状态、保存/撤销 | 共享 `widgets.Shortcuts(env, "selection")` | `TestShortcutsTabUsesSharedEditor`；`shots/13-selection-shortcuts-zh-light.png` | 已验证（接入） |

## 真实窗口检查

| 项目 | 证据 | 状态 |
| --- | --- | --- |
| 中文浅色 | `shots/10`–`13`、`93-selection-editor-zh-light-final.png` | 已验证 |
| 中文深色 | `shots/44-selection-zh-dark.png`、`45-selection-editor-zh-dark.png` | 已验证 |
| 英文深色 / 浅色 | `shots/57`、`58`、`67` | 已验证 |
| 约 820 宽窄窗口：无横向溢出、无裁切 | `shots/64-selection-editor-en-dark-narrow.png`、`67`、`69`、`70` | 已验证 |

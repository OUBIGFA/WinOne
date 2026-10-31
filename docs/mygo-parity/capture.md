# 截图页（capture）功能对照

更新：2026-10-31。React 来源：`apps/desktop/frontend/src/features/capture.tsx`、`capture-ai-settings.tsx`、`capture-filename.tsx`、`ai-model-select.tsx`（含 `ai-model-label.ts`、`ai-model-badge.tsx`、`magpie-model-tells.js`、`ui/capture-option-select.tsx`）、`features/shortcuts.tsx`、`ui/number-field.tsx`、`ui/path-drop.ts`，以及 `main.tsx` 与 `components/module-panel.tsx` 的挂载方式。Rust 桥：`component_capture.rs` 把 `capture.*` 经 `call_completing` 转给 Host。MyGo 实现：`apps/desktop-mygo/pages/capture`（`capture.go`、`filename.go`、`scrolldelay.go`、`ai.go`），共享部分见表内说明。

状态：**已验证** = 有单元测试或真实窗口证据；**待验证** = 已实现但还没有证据。单元测试在 `pages/capture/capture_test.go`；截图在 `_temp/mygo-capture-selection/shots/`（下文简写为 `shots/`）。真实窗口使用隔离夹具（所有模块关闭），Host 为参考包的 `WinOne.Host.exe`。

## 不属于设置页的内容（不移植）

| React 内容 | 判断依据 |
| --- | --- |
| `main.tsx` 传给 `CapturePage` 的 `onEdit`、`onOpenDraft`（`OpenCaptureWindow`）和 `refreshKey` 的刷新来源 | `capture.tsx` 解构了这两个属性但从未使用；`capture.test.tsx` 的 “shows preferences without capture actions or recent images” 断言设置页不显示截图操作和最近截图。设置页没有打开截图编辑器的入口。主 Agent 已认可（2026-10-31）。 |
| `capture-toolbar-preferences.ts`、`ui/capture-drawing-tools.tsx`、`ui/capture-toolbar-actions.tsx` | 只被旧截图编辑器 `capture-editor.tsx`（经 `capture-draft-workspace.tsx`）使用，设置页不渲染。 |
| `ui/capture-toolbar.tsx`、`ui/capture-preview.tsx` | 除自身测试外没有引用。 |
| `onEditorActivity` | `main.tsx` 未传入。 |

## 页面级

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| C1 | 模块关闭时读 `capture.settings.get`，开启时读 `capture.snapshot` 的 `settings`；模块开关状态变化后重新读取 | `page.reload`：按 `env.Snapshot()` 中 capture 的 `state` 选方法；每帧比较开关状态，变化时重读 | `TestLoadUsesSettingsWhenDisabledAndSnapshotWhenEnabled`、`TestLoadingStateAndModuleSwitchReadAgain`；真实窗口（模块关闭）`shots/01-capture-zh-light.png` | 已验证 |
| C2 | 未读到设置时显示“正在加载设置” | 状态文字 + 占位行 | `TestLoadingStateAndModuleSwitchReadAgain` | 已验证 |
| C3 | 读取失败（没有草稿）：错误提示 +“重试”，忙碌时禁用 | `kit.Notice(Failure)` +“重试” | `TestLoadFailureShowsRetry` | 已验证 |
| C4 | 有草稿时出错（保存、选目录、拖入）：错误提示 +“放弃修改并刷新”，所有标签可见 | 标签栏下方 `kit.Notice`，按钮重读 | `TestSaveFailureKeepsDraftAndOffersDiscard`、`TestChooseDirectoryKeepsPageBusyUntilAnswered`、`TestDropFolderOnSaveLocation` | 已验证 |
| C5 | 五个标签（截图后、文件命名、滚动截图、识别与翻译、快捷键）带图标；切换经过未保存保护 | `kit.Tabs` + `env.RequestChange` | `TestTabSwitchGoesThroughGuard`；真实窗口切换时弹出“有未保存的更改” `shots/82-tab-guard-en-light.png`，放弃后草稿还原 `shots/84-ai-after-discard.png` | 已验证 |
| C6 | 识别与翻译首次打开时才读取并挂载，之后保持（草稿跨标签保留） | `aiSection` 首次打开时创建，之后保留并每帧报告活动 | `TestAISettingsSaveWholeConfigurationAndSurviveTabs`（切换标签后不重读、草稿仍在） | 已验证 |
| C7 | 快捷键标签：共享快捷键编辑器（开始截图、截图识别并复制、截图翻译），首次显示时读取 | `widgets.NewShortcuts(env, "capture")` 在首次打开快捷键标签时创建；隐藏时每帧 `Report()`；页面关闭时 `Close()` | `TestShortcutsTabUsesSharedEditor`；`shots/05-capture-shortcuts-zh-light.png`、`shots/55-capture-shortcuts-en-dark.png` | 已验证 |
| C8 | 草稿与基线不同即未保存；活动来源 `capture`（保存、选目录时忙碌）；放弃时本地还原并清除错误 | `env.Activity.Report("capture", dirty, busy, discard)` | `TestSaveSendsWholeDocumentOnce`、`TestSavingDisablesTheSettings`（`Activity.Discard()` 还原） | 已验证 |
| C9 | 保存（右下按钮组）：“保存设置”；忙碌、未修改或滚动间隔无效时禁用；只在前三个标签；`capture.settings.save` 发送完整草稿；成功更新草稿与基线并提示“已保存”；保存中禁用全部控件，不重复提交 | `env.Rail.Set` + `core.Preserved` 保留未声明字段；`env.Toast` | `TestSaveSendsWholeDocumentOnce`（只发送一次、未编辑字段原样、提示）、`TestSavingDisablesTheSettings`；真实 Host 保存成功且其余字段不变 `shots/71-capture-save-real-host.png` | 已验证 |
| C10 | 撤销：忙碌或未修改时禁用；本地还原，不访问 Host | Rail Undo → `discard` | `TestUndoRevertsLocally` | 已验证 |
| C11 | 滚动间隔无效且不在滚动截图标签：提示“滚动间隔须为 100–2000 毫秒的整数。”+“检查滚动间隔”（切到滚动截图并聚焦输入框，不经未保存保护） | 页面底部 `kit.Notice`；`ui.Handle.Focus` | `TestInvalidScrollDelayBlocksSaveAndLeadsToField`（空、99、2001、300.5 四种） | 已验证 |
| C12 | 保存成功显示“已保存” | 改为 `env.Toast`（README 约定） | `TestSaveSendsWholeDocumentOnce`（`h.Toasts()`）；真实窗口 `shots/71-capture-save-real-host.png` | 已验证 |

## 截图后

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| C13 | “处理方式”：单选“截图后编辑”/“截完保存”（各带说明）；选“截完保存”同时关闭“每次选择保存位置”；选“截图后编辑”保留该值 | `kit.Choices` | `TestQuickSaveClearsAskWhereToSave` | 已验证 |
| C14 | “同时执行”：复选框“复制到剪贴板”“截图置顶” | 面板内两行 `ui.Checkbox` | `TestSaveSendsWholeDocumentOnce`、`TestUndoRevertsLocally`；`shots/01-capture-zh-light.png` | 已验证 |
| C15 | “文件保存”：开关“每次选择保存位置”，只在“截图后编辑”时显示 | `kit.Toggle`，按 `openEditor` 显示 | `TestQuickSaveClearsAskWhereToSave` | 已验证 |
| C16 | “保存目录”：显示路径 +“选择目录”；选择中整页忙碌；选中写入草稿，取消不变，失败显示错误；对话框标题“选择文件夹” | `kit.PathField`；`env.Dialogs.PickFolder` 在 `core.Go` 中执行 | `TestChooseDirectoryKeepsPageBusyUntilAnswered`（忙碌、选中、取消、失败、标题）。真实系统对话框未在窗口中打开（对话框由 `app` 实现） | 已验证（单元） |
| C17 | 拖入文件夹到“保存目录”：只接收一个文件夹；错误“每次请拖入一个项目。”“此处只接收文件夹，原路径已保留。”“请从资源管理器拖入本地文件或文件夹。”；忙碌或无草稿时忽略 | `widgets.DropArea`（主 Agent）+ `widgets.CheckDroppedPaths` | `TestDropFolderOnSaveLocation`（文件、两个、一个文件夹）。未在真实窗口拖放 | 已验证（单元） |

## 文件命名

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| C18 | “文件名模板”：输入框，占位“WinOne-{timestamp}-OK”，说明 `capture.fileNameHint` | `ui.TextInput` + `kit.Help` | `shots/02-capture-filename-zh-light.png` | 已验证 |
| C19 | 示例：模板非空时，或获得焦点、编辑后开始；最后一次修改 150 ms 后 `capture.filename.preview {template}`，显示“示例: 名称”；空结果显示“文件名生成失败，请重试”；Host 错误显示在字段下 +“重试”（重新请求并聚焦输入框）；离开标签丢弃未到的结果 | 代数计数丢弃过期结果 | `TestFileNamePreviewFollowsTheTemplate`；真实 Host 返回示例 `shots/56-capture-filename-preview-en-dark.png` | 已验证 |

## 滚动截图

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| C20 | “滚动截图”：开关“从窗口顶部开始” | `kit.Toggle` | `TestScrollFromTopIsAChange`；`shots/03-capture-scroll-zh-light.png` | 已验证 |
| C21 | “滚动间隔（毫秒）”：100–2000 的整数；无效输入原样保留，字段标红并提示“请输入 100–2000 的整数”，禁止保存；上下键按 1 调整 | 页面内数字字段：`ui.TextInput` 保存原文、`Error` 标红、`kit.InlineError`；上下键与 −/+ 按钮按 1 调整到下一个整数并限制在范围内 | `TestInvalidScrollDelayBlocksSaveAndLeadsToField`、`TestScrollDelayStepsWithKeysAndButtons` | 已验证 |

## 识别与翻译

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| C22 | 首次打开读 `capture.ai.configuration.get`；读取中显示“正在加载设置”；失败提示 +“重试”（有未保存修改时不显示） | `aiSection.load` | `TestAILoadFailureOffersRetry` | 已验证 |
| C23 | “文字识别”：“OCR 引擎”本地离线识别 / AI 服务中的模型；选 AI 时显示“截图 OCR”模型选择和说明 | `kit.Segmented` + `widgets.AIModelSelect` | `TestAISettingsSaveWholeConfigurationAndSurviveTabs`；`shots/32-capture-ai-ocr-model.png` | 已验证 |
| C24 | “翻译”：“翻译引擎”内置 Google 翻译 / AI 服务中的模型；选 AI 时显示“截图翻译”模型选择，否则显示“翻译时将识别文字发送到 Google。” | 同上 | `TestAITranslationWithAModel`；`shots/81-capture-ai-llm-en-light.png` | 已验证 |
| C25 | “目标语言”：auto、zh-CN、zh-TW、en、ja、ko | `kit.Select` | `TestAITranslationWithAModel`（选“日语”保存为 `ja`） | 已验证 |
| C26 | 选 AI 翻译时：“翻译提示词”（占位“留空使用默认翻译提示词”）和说明 | `ui.Field` + `ui.TextArea` + `Description` | `TestAITranslationWithAModel`；`shots/81-capture-ai-llm-en-light.png` | 已验证 |
| C27 | “后续对话”：“结果对话”的模型选择 | `widgets.AIModelSelect` | `TestAIFollowDefaultRemovesTheTask` | 已验证 |
| C28 | 模型选择（共享）：跟随默认 / 未配置（值无供应商时）/ 各供应商（图标）/ 已删除供应商 ID；已启用模型（图标、同名区分、免费/倍率/上下文徽标）；三种占位；无供应商或无模型时禁用；失效提示；选供应商自动选第一个模型，跟随默认删除任务，未配置写入空供应商 | 新共享文件 `widgets/aimodel.go`；图标用主 Agent 的 `widgets.ProviderIcon`/`ModelIcon` | `widgets/aimodel_test.go` 四项 + 页面测试；模型下拉的图标与徽标渲染 `shots/picker-light.png`、`shots/picker-dark.png` | 已验证 |
| C29 | 保存：“保存设置”，`capture.ai.configuration.save {configuration}`（整份配置，保留未编辑字段）；未修改或忙碌时禁用；成功提示“已保存” | `core.Preserved` + Rail | `TestAISettingsSaveWholeConfigurationAndSurviveTabs`（providers、keys、magpieSettings、revision 原样）、`TestAITranslationWithAModel`（提示）；真实 Host `shots/76-capture-ai-save-real-host.png` | 已验证 |
| C30 | 撤销：重新读取配置 | Rail Undo → `load` | `TestAIFollowDefaultRemovesTheTask`；真实窗口 `shots/33-capture-ai-undo.png` | 已验证 |
| C31 | 活动来源 `capture-ai`；放弃时重读 | `env.Activity.Report("capture-ai", …, load)` | `TestAIDiscardReadsAgain`；真实窗口 `shots/82-tab-guard-en-light.png` → `shots/84-ai-after-discard.png` | 已验证 |
| C32 | 读取或保存中禁用全部控件 | 外层容器 `Disabled(busy)` | `TestSavingDisablesTheSettings`（设置标签）；识别与翻译标签用同一方式，未单独测试 | 已验证（设置标签） |

## 快捷键

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| C33 | 三个快捷键字段、注册状态、保存/撤销、重试启用、动作错误 | 共享 `widgets.Shortcuts`（主 Agent 维护）；本页只接入 | `TestShortcutsTabUsesSharedEditor`；`shots/05-capture-shortcuts-zh-light.png` | 已验证（接入） |

## 真实窗口检查

| 项目 | 证据 | 状态 |
| --- | --- | --- |
| 中文浅色 | `shots/01`–`05`、`91-capture-zh-light-final.png`、`92-capture-ai-zh-light-final.png` | 已验证 |
| 中文深色 | `shots/41-capture-zh-dark.png`、`42`、`43` | 已验证 |
| 英文深色 / 浅色 | `shots/51`–`56`、`81`、`82` | 已验证 |
| 约 820 宽窄窗口（英文）：无横向溢出、无裁切；“OCR 引擎”等控件换到标签下方 | `shots/61-capture-en-dark-narrow.png`、`62`、`63`、`66`、`75-capture-ai-ocr-model-en-light-narrow.png` | 已验证。注意：五个标签标题在此宽度被省略（如 “Shor…”），这是共享 `kit.Tabs` 的设计（React 换行显示），已报告主 Agent |
| 真实 Host 保存 | `shots/71-capture-save-real-host.png`（截图设置）、`shots/76-capture-ai-save-real-host.png`（AI 配置） | 已验证 |

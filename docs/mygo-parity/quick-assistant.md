# 快捷助手页（quick-assistant）功能对照

更新：2026-10-31。React 来源与划词助手相同：`features/selection.tsx`，`main.tsx` 以 `<SelectionPage key="quick-assistant" moduleId="quick-assistant" …/>` 挂载。Rust 桥：`quick-assistant` 命名空间允许 `snapshot`、`settings.get`、`settings.save`、`show`，都经 `call_completing`；`show` 返回后由桌面进程激活快捷助手窗口（`activate_quick_assistant`）。MyGo 实现：`apps/desktop-mygo/pages/selection`（与划词助手同一个包，按 `quick` 区分）。与划词助手相同的行为见 [selection.md](selection.md) 的 S2–S6、S11–S23（同一份代码，测试覆盖两页共用的部分），这里列快捷助手自己的项目和差异。

状态：**已验证** = 有单元测试或真实窗口证据；**待验证** = 已实现但还没有证据。测试在 `pages/selection/selection_test.go`；截图在 `_temp/mygo-capture-selection/shots/`。

| 编号 | React 入口/行为 | MyGo 实现 | 验证方式 | 状态 |
| --- | --- | --- | --- | --- |
| Q1 | 打开时读 `quick-assistant.settings.get`；“正在读取快捷助手设置”；失败提示 +“重试” | 同 S1，命名空间 `quick-assistant` | `TestQuickLoadsItsOwnSettingsAndSavesItsFields` | 已验证 |
| Q2 | 标签栏右侧“打开快捷助手”（Astroid 图标）；模块关闭或忙碌时禁用；调用 `quick-assistant.show`，再把返回的窗口（`window`、`processId`）切到前台；失败显示页面错误（所有标签可见） | 与标签同一行、同一条细线；`core.Do`：`CallCompleting("quick-assistant.show")` 后调用 `resultwindow.WindowTarget.Activate()`（与 Host 同一实现：核对窗口属于该进程、允许前台、最小化时还原、设为前台并确认）；失败提示 `quickAssistant.activateFailed`（中文与旧窗口相同，新增英文）；打开中按钮显示进行中 | `TestOpenQuickAssistantShowsAndReportsActivationFailure`（无效窗口报错、替换激活后成功、模块关闭时禁用）、`TestErrorsShowAboveEveryTab`。真实窗口中模块关闭，只验证禁用态 `shots/31-quick-tabbar.png`；真实激活未验证（需开启模块，会安装全局热键） | 已验证（单元 + 禁用态） |
| Q3 | 常规：模块关闭时显示“快捷助手已关闭” | 同 S7 | `TestQuickDisabledNoteAndShortcuts`；`shots/21-quick-general-zh-light.png` | 已验证 |
| Q4 | 常规：“窗口行为”：开关“默认置顶快捷助手”（`alwaysOnTop===true`）、“显示快捷助手的思考过程”（未设置视为开启） | `kit.Toggle`；`showReasoning` 未改时不写入 | `TestQuickLoadsItsOwnSettingsAndSavesItsFields`；真实 Host 保存只含 `schemaVersion,revision,alwaysOnTop,actions` `shots/74-quick-save-real-host.png` | 已验证 |
| Q5 | 没有“显示方式”和“排除应用”；不写入划词字段 | 按 `quick` 分支；字段为空指针时不写入 | `TestQuickLoadsItsOwnSettingsAndSavesItsFields`（保存内容无 automatic、keyboard、excludedApps、toolbarVersion） | 已验证 |
| Q6 | 工具栏：图标列表不含“复制” | 图标选项按 `quick` 过滤 | `TestQuickLoadsItsOwnSettingsAndSavesItsFields` | 已验证 |
| Q7 | 工具栏下方说明“使用这些功能”与两条要点 | 标题 + 两行要点，分组名称为标题 | `TestQuickLoadsItsOwnSettingsAndSavesItsFields`；`shots/20-quick-zh-light.png` | 已验证 |
| Q8 | 保存/撤销/草稿/排序/自定义功能与划词助手相同，保存到 `quick-assistant.settings.save` | 同一实现 | `TestQuickLoadsItsOwnSettingsAndSavesItsFields`；`shots/74-quick-save-real-host.png` | 已验证 |
| Q9 | 快捷键：“打开快捷助手”字段 | 共享 `widgets.Shortcuts(env, "quick-assistant")` | `TestQuickDisabledNoteAndShortcuts` | 已验证（接入） |

## 真实窗口检查

| 项目 | 证据 | 状态 |
| --- | --- | --- |
| 中文浅色 | `shots/20-quick-zh-light.png`、`21`、`31-quick-tabbar.png`、`94-quick-zh-light-final.png` | 已验证 |
| 英文深色 / 浅色 | `shots/59-quick-en-dark.png`、`68-quick-en-light-narrow.png`、`73` | 已验证 |
| 约 820 宽窄窗口：无横向溢出、无裁切 | `shots/60-quick-en-dark-narrow.png`、`68`、`73` | 已验证 |
| 中文深色 | `shots/95-quick-zh-dark.png`、`96-quick-general-zh-dark.png` | 已验证 |

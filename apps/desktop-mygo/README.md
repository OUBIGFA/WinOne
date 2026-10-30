# WinOne MyGo 窗口开发说明

`apps/desktop-mygo` 是用 [MyGo](https://github.com/egoist/mygo) v0.7.3 原生 UI 改写的 WinOne.exe。迁移决策、阶段和验收方法见 [MyGo 前端迁移](../../docs/MyGo前端迁移.md)。本文说明怎样在这里写页面。

## 工具链

- Go 1.27.3：`dist/resources/tools/go-1.27.3/bin/go.exe`。命令前设置 `GOROOT` 为该目录，`GOTOOLCHAIN=local`，`CGO_ENABLED=0`，`GOCACHE=dist/resources/cache/go`。
- MyGo 文档与源码：`dist/resources/sources/mygo-v0.7.3/github.com/egoist/mygo@v0.7.3/docs/ui/`（组件说明）和 `ui/`（实现）。先查文档，再写代码，不要猜 API。
- 正式构建：`build/build-desktop-mygo.ps1 -OutputDirectory _temp/<任务>/build`（图标检查、测试、vet、生产构建）。

## 包结构

| 包 | 职责 |
| --- | --- |
| `app` | 进程：启动参数、维护锁、管理员重定向、单实例、窗口生命周期 |
| `host` | Host RPC 客户端（命名管道），方法名与超时沿用旧窗口 |
| `core` | 页面环境 `Env`、`Go`/`Do` 异步、`Activity`（未保存保护）、`Rail`（保存按钮组）、`Preserved`（保留未知 JSON 字段）、`FollowJob` |
| `kit` | 设计系统：主题、字号层级、`Page`/`Group`/`Row`/`Stack`/`Choices`/`Facts`、按钮、`Tabs`/`SubTabs`、`Notice`/`InlineError`/`Empty`/`Skeleton`/`Badge`、`Dialog`、`Select`/`Segmented`/`NumberField`/`PathField` |
| `kit/icons` | Lucide 图标，按名称引用，如 `icons.Save` |
| `widgets` | 需要 Host 的组合控件：`ShortcutRecorder` |
| `shell` | 标题栏、侧栏、内容框、保存按钮组、未保存确认 |
| `pages/<页面>` | 每个页面一个包，在 `init` 里 `shell.Register` |
| `testhost` | 测试用的假 Host（真实管道 + 真实客户端）和 `ui.Tester` 驱动 |
| `i18n` | 中英文案，键名与旧 React 窗口一致 |

## 写一个页面

参考 `pages/overview`（首页）。

1. **注册**：在页面包的 `init` 中调用 `shell.Register("<id>", core.Settings, func(env *core.Env) core.Page {...})`，并在 `pages/pages.go` 中空白导入该包。页面 ID 见 `shell/pages.go`。
2. **状态**：页面是一个结构体，保存草稿、读取结果和进行中的操作。离开页面时实例被丢弃（下次进入重新读取）；需要释放 Host 资源时实现 `core.Closer`。
3. **读取与保存**：Host 调用会阻塞，必须放在 `core.Go`/`core.Do` 中，结果回到 UI 线程后再改状态。方法名、参数与旧窗口完全一致：
   - 旧 `bridge.ts` 的 `WinTray/Modules/AlwaysOnTop/Selection/QuickAssistant/TimedPower/Interconnect/DirectoryLinks/Stroke(method, payload)` 对应 `env.Host.CallCompleting("<namespace>.<method>", params)`。命名空间：`wintray`、`modules`、`always-on-top`、`selection`、`quick-assistant`、`timed-power`、`interconnect`、`directory-links`、`stroke`。
   - `Capture(method)` 对应 `capture.<method>`（AI 服务也在 `capture.ai.*`）；`Chat(method)` 对应 `env.Host.Call("chat.<method>")`；`SaveSettings` 对应 `CallCompleting("settings.save", settings)`；`Snapshot` 用 `env.Snapshot()` 与 `env.Refresh()`。
   - 返回 `{state:"running", id}` 的任务用 `core.FollowJob(env, namespace, job)` 跟进。
4. **未知字段**：编辑 Host 文档（模块设置等）时用 `core.Preserved[T]` 解码和保存，避免丢失本窗口没有声明的字段。
5. **未保存保护**：每次 `View` 开头调用 `env.Activity.Report("<来源>", dirty, busy, revert)`。切换一级/二级标签用 `kit.Tabs(c, &tab, tabs, env.RequestChange)`，离开页面和关闭窗口由外壳处理。
6. **保存按钮组**：有手动保存的页面在 `View` 中调用 `env.Rail.Set(core.Action{Label, Disabled, Busy, Run}, core.Action{Run: revert, Disabled: !dirty})`。按钮固定在右下，不在页面里另放保存按钮。Ctrl+S 触发同一保存。
7. **反馈**：成功用 `c.Toast(i18n.T(...))`；失败用 `kit.Notice(c, kit.Failure, ...)` 放在相关内容上方，或用 `kit.InlineError` 放在字段旁；加载中用 `kit.Skeleton`；空状态用 `kit.Empty`（说明缺什么、怎样添加，并给出操作）。

## 设计规则

用户允许在迁移中重新设计 UI（2026-10-31），参考 [impeccable](https://github.com/pbakaus/impeccable) 的 Operate 模式。功能、入口、选项、数据和保存语义必须全部保留，不能为了简化删减。

- 只用 `kit` 的颜色和字号角色（`Caption`、`Help`、`Label`、`Strong`、`Heading`、`Title`），不在页面里写颜色值。语义色（成功/警告/错误）只用于有含义的状态。
- 设置项放在 `kit.Group` 的面板中：每行左侧是名称和说明，右侧是控件（`kit.Row`）；需要整行宽度的控件用 `kit.Stack`。组与组之间留大间距，组内各行紧凑，用细线分隔。
- 少量互斥选项用 `kit.Segmented`；需要解释后果的选项用 `kit.Choices`；长列表和带图标的选项用 `kit.Select`。
- 禁止：卡片套卡片、标题上方的图标砖、眉标（标题上方的小字标签）、渐变文字、彩色左边框、能内联解决却用弹窗、表情符号代替图标。
- 每个控件都要有悬停、焦点、禁用、加载、错误状态；文字可能很长（英文、路径），要能换行或省略，并给出完整提示（`Tooltip`）。不出现横向滚动。
- 图标：引用 `icons.<LucideName>` 后运行 `node build/generate-mygo-icons.mjs`。

## 文案

- 键名沿用 `i18n/zh-CN.json` 与 `i18n/en-US.json` 中已有的键（从 React 窗口转换而来）。用 `i18n.T("key", "name", value)` 插值。
- 新增文案写在该页面自己的文件中：`i18n/<页面>.zh-CN.json` 和 `i18n/<页面>.en-US.json`，两种语言都要写。不要修改 `zh-CN.json`/`en-US.json` 主文件，避免并行开发时冲突。界面文案只说明用户能做什么、会得到什么结果，不写实现过程。

## 测试与验证

- 单元测试：用 `testhost.New(t, handler)` 起一个假 Host，`ui.NewTester(page.View, w, h)` 渲染，`h.Settle(tt)` 等待请求完成并重绘。用文字或 `Label` 找控件（`tt.Click`、`tt.HasText`），断言实际发出的请求（`h.Called`）。不要复制实现逻辑进测试。
- 运行：`dev.ps1`（在 `_temp/<任务>/` 中自建）把 WinOne.exe 放进完整的候选包副本，以隔离的 `--pipe \\.\pipe\WinOne-<名称>` 和 `--data-dir <隔离目录>` 启动；关闭全部模块的夹具配置可以避免钩子和快捷键干扰真实系统。只结束自己启动的进程（按路径或 PID）。
- 截图：用 window-screenshot 技能按 PID 截图，与 `_temp/mygo-frontend-20261030/baseline/run3/` 中旧版基线对照功能是否齐全；外观以新设计为准。

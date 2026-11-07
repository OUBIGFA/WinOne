# 微信自动登录：直接同步 WinTray（2026-11-06）

## 需求与来源

修复程序管理勾选“微信自动登录”后，微信未真正进入登录流程的问题。直接复用用户指定的本地 WinTray，不引入 Python、OCR、固定屏幕坐标或另一套登录实现。

- 上游：`E:\_Free code\WinTray`，干净 HEAD `db9c545b4a7c0326c9f3978991ece72e3c55fe7d`。
- `modules/wintray/internal/orchestrator/wechat_uia_windows.go` 与上游原文件**字节一致**，SHA-256：`63e6347bd212449f598a7d90d1cfbf803c15f5359beb535eff008bfbf04d61de`。
- 完整同步上游 `wechat_uia_windows_test.go`，仅适配 WinOne 导入路径和 Go 导入分组。
- MIT 许可原文与现有 `modules/wintray/LICENSE` 摘要一致；未新增依赖。来源与实际目标摘要登记在 `third_party/wintray-source.json`。
- 修改 MyGo 前查询上游，v0.7.4 仍是最新，与当前锁定一致。
- 按用户最新要求：**不提交、不推送、不更新安装器**；由其他 AI 收尾后统一推送。未修改真实 WinOne/WinTray 安装、用户配置或微信账号。

## 根因

1. WinOne 仍使用旧的 UIA `Invoke` 路径。微信 4 的 Qt provider 虽返回成功，却不会激活登录按钮；旧原生夹具只有标准 Windows 按钮，未覆盖这个行为。
2. 微信 4.1.12+ 部分新启动主窗口的 UIA class 仍为 Qt 外壳，只有可见的 `MMUIRenderSubWindowHW` 子表面能证明主界面已就绪；旧实现只认可语义主窗口类，无法确认登录成功。
3. MyGo 界面把登录开关放在“收进托盘”专属内容中，正常/任务启动无法设置，与已有后端及 WinTray 入口不一致。

此前来源清单已经保存了新上游文件摘要，但没有验证实际移植文件是否一致。本次增加 `tests/notices/wintray-wechat.test.mjs`，并在 `build/build-wintray.ps1` 开始构建前调用，防止再次“只更新来源记录、漏同步实际代码”。

## 完整能力对照

| WinTray 自动登录能力 | WinOne 接入 | 验证 |
| --- | --- | --- |
| Weixin.exe / WeChat.exe 明确勾选；旧配置默认关闭 | 现有配置契约与 MyGo 开关 | 配置默认/程序名限制、界面显示与保存回归 |
| 正常、托盘、任务启动均可启用；后台模式不可执行但保留偏好 | MyGo `pages/wintray/details.go`；现有 `StartNow` / `StartAndManage` | 三模式保存、切换后台禁用且保留偏好；真实任务启动夹具 |
| 老版按钮使用 UIA Invoke；Qt 登录窗口使用按钮实际边界发一次 HWND 局部点击 | 原版 `wechat_uia_windows.go` | `ready` / `qt-ready` / `qt-rendered-ready` 真实系统 UIA provider |
| 物理像素/DPI 正确换算，不移动鼠标，不点击遮挡窗口 | 原版 `clickWeChatQtButton` | 上游 Qt 子进程按实际按钮屏幕/客户区位置接收点击；生产源码摘要一致 |
| 认可语义主窗；新版必须有微信标题、同 PID、可见渲染子表面 | 原版 `root` / `renderedMain` | 正确主窗、错误标题、缺表面、隐藏表面四种原生观察 |
| 只点一次，未确认主窗不能谎报成功或提前收起 | 保留上游等待状态机与现有登录先于收起调用链 | 主窗前置、同 PID、进程变化、点击计数及登录后收起/还原 |
| 扫码、手机确认、禁用/歧义按钮、多账号留给用户；30 秒边界不变 | 保留上游完整行为及 WinOne 翻译错误码 | 老版/Qt/渲染主窗的未完成登录回归；取消/错误/多账号测试 |
| 开关保存和导出/导入，不迁移账号数据 | 现有配置与 `internal/profiles` | 保存完整草稿与可迁移布尔字段回归 |

## 验证记录

所有证据在 `_temp/wechat-login-sync/`，正式测试源码留在各源码包和 `tests/notices/`。

- **先失败后通过**：`red-native.log` 中旧实现的 `qt-ready`、`qt-rendered-ready` 登录失败，新版渲染主窗被识别为 unknown；旧 MyGo 正常/任务/后台入口回归失败。同步后 `green-native.log` 与 `green-ui.log` 全部通过。
- **原生测试**：三个登录/收起场景、十个未完成登录场景、两个任务启动场景、四个渲染主窗身份场景，使用真实 Windows UIA 与专用 GUI 子进程，不操作真实微信。
- **界面**：程序管理页全套回归通过，生成中文/英文 × 深浅色 × 1000/560 宽 × 四启动模式共 32 张开关布局图；检查了宽窗/窄窗、正常/任务/托盘和后台禁用状态。
- **隔离完整 WinTray 构建**：从 `05fe910d` 建立无其他未提交改动的 detached 工作树，仅覆盖本任务文件。`build/build-wintray.ps1` 全模块 test/vet、x86/x64 原生桥 ABI、Worker/Recovery 构建及 Worker 真实进程集成通过，日志 `isolated-build-wintray-final.log`。
- **隔离完整 MyGo 构建**：同一源码的官方构建入口图标/来源/React 资源阶段通过；随后原命令续接 Go 全包 test、普通/生产 vet、Windows 资源、生产 EXE 与 37 项许可检查通过，日志 `isolated-build-desktop-mygo-final.log` 与 `isolated-build-desktop-mygo-native-final.log`。不是当前其他会话未提交功能的验收。
- **当前工作区限制**：首次整体检查仍有其他未提交 Chat 回归失败，保留原失败日志。结束前当前工作区再验程序管理页，被其他新增插件页的编译错误阻断：`chat/pluginhost/pluginpage/page.go:117` 调用当前 MyGo 没有的 `WebView.IsFocused`（`current-ui-diagnostic.log`）；程序管理测试无法在该最新组合中再次执行。这不影响本任务隔离源码的全套验证，但统一收尾时必须处理，不能声称当前所有未提交功能或最终组合已通过。未修改该插件页。
- **测试环境处理**：启动命令夹具在带空格的仓库路径下会撞到仓库外既有 `E:\_Free` 文件，因此正确返回“unquoted”；未删除、读取内容或修改该文件。为 WinTray 的 Go 临时目录使用同一仓库目录的 8.3 别名后原断言通过。MyGo 的路径规范化断言则使用普通规范路径。没有放宽断言或修改产品超时。

## 后续统一交付注意

- 实际改动保留在主工作区；隔离工作树和构建仅供本任务验证，位于 `_temp/wechat-login-sync/verified-source/`，不能拿它替代其他 AI 正在完成的合并版本。
- 新增来源检查命令：`node --test tests/notices/wintray-wechat.test.mjs`；改动原版生产文件前先核对上游并同步来源/目标摘要。
- 微信相关 UI 回归可用 `build/test-native-ui.ps1 -Modules desktop-mygo -Packages ./pages/wintray -Run TestWeChatAutoLogin`。
- 未对真实安装的微信执行账号登录，也未做物理混合 DPI/多屏验收；不能把专用原生夹具等同于真实账号验收。扫码或手机确认仍需用户完成。

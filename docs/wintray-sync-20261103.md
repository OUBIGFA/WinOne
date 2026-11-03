# 程序管理增量同步与审核（2026-11-03）

## 来源与边界

按用户要求审核 WinOne 工作区中另一会话的改动，并核对独立 WinTray 当前工作树。基准为 `03a809f94b7eec97ae123636b86879357a0052e4`；同时接入该工作树尚未提交的 AutoHotkey、DC 渲染兼容和任务栏未就绪处理。不是把两侧已经分叉的目录整体覆盖。

- 精确来源及 41 份原文件 SHA-256：`third_party/wintray-source.json`。
- 原 MIT 许可证与 WinOne 副本 SHA-256 相同：`0eb3eb100d2f43201dc92197547561de0478614ea9a3b54b2e25bddc3d343ac7`。源码级 Go/npm、导入来源与许可检查通过；未新增运行依赖。
- 保留 WinOne 模块名称空间、统一配置/日志/托盘、独立配置拒绝真实自启、PID/进程路径身份检查、恢复副进程和未保存保护。
- 只读取独立 WinTray 工作树，不修改或提交它的文件。所有运行输出在仓库 `_temp/post-migration-20261103/`。

## 上游能力逐项对照

| 能力/修复 | WinOne 接入点 | 验收入口与边界 |
| --- | --- | --- |
| 窗口枚举不持锁读取标题，避免 UI 互等 | `modules/wintray/internal/orchestrator/window_enum_windows.go` | `window_enum_windows_test.go` 的真实窗口/看门狗 |
| 收纳器终态退出；Explorer 拒绝还原时释放监听器，保留恢复副进程快照 | `internal/traybox/box_windows.go`、`listener_windows.go` | `TestBoxCloseIsTerminalWhenExplorerRejectsRestoration`、`TestRejectedShutdownHandsRecoveryToCompanion` |
| 收纳原生桥嵌套超时不回退暂停令牌；不同令牌不能谎报已恢复 | `native/traybridge/bridge.c` | `TestShellBridgeNestedTimeoutRestoresIcons`，x64/x86 × 内外层超时组合，只接入专用假 Shell |
| 任务栏未就绪时重试；界面线程可继续处理消息；收纳就绪后才放行程序启动 | `worker/tray_startup_windows.go`、`Runtime.Reconcile`、已有 ready 握手 | `TestCollectTrayIconsRetriesUntilTheTaskbarExists`、其他错误立即报告、Worker 取消停止重试；登录程序在 `Activate` 完成后启动 |
| DC-backed 窗口禁止临时加入 `WS_EX_LAYERED`；保留原程序收起/还原能力 | `internal/orchestrator/startup_visibility_windows.go` | `startup_visibility_rendering_windows_test.go`，监视真实 `WM_STYLECHANGING`，覆盖 `CS_OWNDC`/`CS_CLASSDC` 与手动/自动启动 |
| `.ahk` 从用户文件关联启动，保留版本选择、参数、工作目录和显示状态 | `runner_association_windows.go`、`runner.go` | `TestStartProcessAutoHotkeyAssociation`：子进程私有 HKCR、正常/隐藏/无关联/损坏关联、中文与空参数；不修改真实关联 |
| AutoHotkey 运行身份按引擎及主脚本操作数匹配，不能把其他脚本的数据参数当作目标 | `script_process_windows.go` | `program_window_windows_test.go` 的命令行边界；`ahk_runtime_windows_test.go` 提供便携真实 v1/v2 入口，当前未提供解释器 |
| 程序状态、显示/隐藏、确认后关闭 | 已有 `worker/program_actions_windows.go` 与 MyGo `pages/wintray/details.go` | 原功能保留，不另移植独立 WinTray 产品 UI；现有程序操作、确认和身份测试 |
| 微信自动登录，只调用一个可用登录按钮；等待同一进程主窗口，扫码/手机确认留给用户 | `wechat_login_windows.go`、`wechat_uia_windows.go`；MyGo 详情开关 | 真实 Windows UI Automation 子进程夹具：登录/扫码/手机确认/禁用/歧义；未操作真实微信账号 |
| 微信开关保存与配置导出/导入；旧配置不自动开启 | MyGo `model.go`、`internal/profiles/builtin.go`/`import_wintray.go` | MyGo 保存测试、`TestWinTrayWeChatAutoLoginSurvivesPortableRoundTrip`、布尔类型校验；`.ahk` 路径映射回归 |
| 程序选择、拖入与更换支持 `.ahk` | MyGo 添加/更换筛选、模块配置校验、配置导入；旧 Rust 文件筛选保持兼容 | 保留脚本后台启动限制、原程序 ID/参数/计划，不猜解释器位置 |

## 审核补齐的本地问题

1. **关联启动漏接等待回调**：WinOne 的 `startedProcess.Wait` 使用回调，上游新增 Shell 启动只返回 `os.Process`。现在补接真实等待与非零退出码，不再空回调崩溃。
2. **终态语义调用方未同步**：停用/维护仍是可重试操作，改用原有事务式 `SetPaths(nil)`，还原失败时保留收纳选择。普通退出先完成可拒绝的托管窗口恢复，再终态关闭收纳器，避免退出被拒后留下一个仍接收请求但已丢失收纳器的 Worker。
3. **微信偏好在可迁移配置中丢失**：补上可选布尔字段的导出/导入，保留旧档案缺省语义；先由回归复现，修复后 `internal/profiles` 全套通过。
4. **短控制台夹具取样过早**：子进程已开始不表示 conhost 已显示。夹具现在等待实际可见状态后取样，仍要求真实控制台句柄、显示/隐藏、参数和环境完全匹配；三种启动模式的原断言通过，不修改产品启动行为。

## 验证与限制

- MyGo 窗口生命周期七种场景已通过（`close-fourth/`）：未设置/旧开/旧关、托盘退出、未保存、忙碌、慢页面释放；单实例唤醒、位置写入和实例互斥释放均检查。该专用夹具的批准关闭至进程结束为 13–29 ms，不代表所有页面/负载的统一性能保证。
- WinTray 原生套件依赖正常桌面的 Shell：私有桌面没有任务栏且低完整性子进程不能访问它，失败保留在 `wintray-private-failed.utf8.log`。不通过修改产品、放宽断言或跳过该失败来伪装通过；正常桌面测试只操作专用窗口，真实 Task Scheduler/Explorer 注入/重启入口保持未选中。
- 源码级许可与定向 Profile/控制台检查已通过；当前全模块回归、完整 MyGo 构建、包内启动和隔离安装矩阵仍在收尾，最终证据与固定安装器摘要见 `docs/development.md` 的当前章节。
- 未验证真实微信、真实 AutoHotkey v1/v2 解释器、实体 UAC/IM/供应商账号、物理输入法、混合 DPI/多屏与长期性能；保留全部入口，不把夹具当作真实账号验收。

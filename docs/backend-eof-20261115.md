# 后台慢回复 EOF 与默认联网验证（2026-11-15）

## 根因与修复

`internal/transport/pipe_windows.go` 原先在读取请求前设置整个连接的 5 秒截止时间，收到 `capture.*` 请求后改为 15 秒。`SetDeadline` 同时约束最后的回复写入，但这段时间已经包含后台处理请求的耗时。

AI 模型测试的后台预算为 95 秒，Worker 为 90 秒，界面允许等待 100 秒。请求运行超过 15 秒后，后台即使已经得到成功结果或业务错误，也会因为过期的写入截止时间丢失回复。连接随后关闭，界面显示“无法连接后台：EOF (transport)”。该现象不能据此判断为后台退出或供应商网络断开。

已用真实 Windows 命名管道复现：普通请求运行 6 秒、模型测试和账号查询运行 16 秒，旧实现均返回 EOF；紧接着的新请求仍然成功，且原操作只执行一次。

现在分别限制读取请求和发送回复，各为 5 秒。发送回复的计时从处理完成、结果序列化之后开始。业务等待仍由调用方和具体处理函数决定，保留取消、空闲连接保护、无人读取回复时的关闭和原有操作预算；没有自动重试写操作。

## 自动验证的联网范围

按用户本次要求，50 个 Go 测试文件中的真实 HTTP 服务、TCP/UDP、网关、登录回调、浏览器账号和互传检查默认不执行。需要用户明确要求，再设置 `WINONE_NETWORK_TESTS=1`，并满足原有账号或公开服务开关。纯内存解析、文件操作和本机命名管道检查继续保留。

13 个独立上游、浏览器和互传验证入口在未明确选择联网时会直接停止，实际检查确认它们没有创建测试输出目录。FileShare 的真实端口测试标为默认忽略。未修改防火墙、网络权限或产品的联网功能。

## 验证与限制

- `build/test-transport.ps1` 的真实管道、消息帧和静态检查通过。覆盖慢成功回复、慢业务错误、后续请求、只执行一次、调用取消、空闲发送者和无人读取回复。
- 13 个独立联网入口的默认拒绝行为与语法检查通过。没有把拒绝执行记为联网测试通过。
- 扩大源码检查时，根模块检查通过，但既有 WinTray 原生窗口检查出现以下 6 项失败：`TestUnresponsiveEscapeNeverEscalatesToDisabledClose`、`TestStartProcessAutoHotkeyAssociation/custom`、`TestStart_ConsoleExecutableIsHiddenToTray/StartAndManage`、`TestStartupVisibilityPartialCloseIsNotReportedAsComplete`、`TestStartupVisibilityDoesNotMistakeTransparencyForNativeClose`、`TestStartupVisibilityCancellationRestoresTemporaryAttributes`。因此不把整轮源码检查记为通过。原始日志保留，未修改这些用例的断言。
- 最终采用 `-CompileOnly` 完成所有组件的编译、静态检查、来源、许可证和程序包清单核对。联网、真实供应商、桌面交互及安装生命周期没有在最终交付流程中运行；Windows 本地验收仍按项目当前约定保留为可用能力。

## 构建来源

修复源码为 `4a57785d4ff523e2e98393682c73b9bdf33a763d`。主程序、Host、结果窗口等 8 个本项目 Go 程序均记录此提交和 `vcs.modified=false`。

完整构建和最终清单检查返回后，外层任务检查发现其他任务修改了文档及安装器终端提示，因此保留该轮 `build-summary.json` 的失败状态。另行核对确认编译输入仍与该提交一致，并记录在 `build-audit.json`。安装器使用该提交的独立干净副本生成，未混入并行改动，也未把后续文档提交写成程序的编译来源。

原始复现、最终管道检查、联网入口检查、构建审计和交付记录保存于 `dist/reports/backend-eof-20261115-4a57785/`。旧固定安装器、配套卸载器和校验文件保留在 `_temp/backend-eof-20261115-01a1dcf7/prior-release/`。

## 本次交付

固定安装器 `dist/release/WinOne-setup.exe` 已更新为 261,532,664 字节，SHA-256 为 `03938384e92ec6530915b09b5b28ae5bd236b01f7062d8b49ca8fa850bcc7f0b`。配套卸载器为 1,359,622 字节，SHA-256 为 `eae97966b7783e31bfdc66d56768d28528d3fb57a1ac02d2b695d8c610482a40`。两份校验文件及 `release-info.json` 已同步，5,883 个包文件和安装器/卸载器对应关系核对通过。

安装维护、更新和首次提示三个辅助程序也来自同一个干净提交。第一次文件替换因 PowerShell 的空字符串参数转换失败，原五个交付文件保持原样；验证正确的空值传递后完成原子替换，并重新校验全部文件。该过程记录在交付验证中。

本次没有安装或更新用户正在使用的 WinOne，没有修改真实配置、网络权限或防火墙。由用户自行运行固定安装器。

# 资源基线

## 故障恢复版本基础采样（2026-10-08）

四个内置模块关闭、一个已登记测试模块关闭时，7 个完整样本的 Host 私有工作集峰值 **40.29 MiB**、Private Bytes **52.90 MiB**，仍超过 40 MiB 门槛。运行测试 Worker 后，10 个完整样本中 Host、Worker 和控制台辅助进程的峰值为 **80.98 / 107.90 MiB**，两组平均 CPU 均未计得增量。完成了进程退出核对，但这不是实际扩展负载或长期稳定性结论。

与上一轮的小幅差异不作为收益；Go 静态缓冲的驻留波动仍存在。原始记录在 `_temp/repository-hygiene-20261008-9513b969/archived-dist/package-recovery-20261008/final-resources/`。正式版性能尚未通过。

## 模块发现与签名测试 Worker（2026-10-08）

当前源码的 Host 在四个内置模块关闭、一个可选测试模块已登记但关闭时，10 个完整样本私有工作集峰值 **40.37 MiB**、Private Bytes **52.63 MiB**，仍未达到 Host 40 MiB 门槛。运行签名测试 Worker 后，Host、Worker 和控制台辅助进程的 10 个完整样本峰值为 **81.41 / 108.34 MiB**；两个场景平均 CPU 均未计得增量。正常托盘退出后确认 Worker 收到恢复命令，全部已观察进程均已退出。

这是无 WebView 的短时协议负载，测试 Worker 是复制并签名的 Go 测试程序，不能代表 WinTab、Dock 或其他实际模块。同负载空目录中间候选对照的提交量增加约 1.90 MiB，工作集因 Go 32 MiB 静态缓冲驻留变化而大幅波动，不能算作优化收益。完整记录在 `_temp/repository-hygiene-20261008-9513b969/archived-dist/module-host-discovery-20261008/`，方法及版本见 [模块包配置与发现](模块包配置与发现.md)。正式版性能仍未通过。

## 四模块启用与基础负载（2026-10-07）

包为 `model-picker-20261007`。真实 Host 同时运行生成截图编辑、置顶边框、托管控制台和内置划词监听；自动划词读取关闭，未读取用户选区，未打开设置 WebView、OCR/AI 或划词结果。26 个完整样本的产品私有工作集峰值 **51.98 MiB**、Private Bytes 峰值 **231.32 MiB**、平均 CPU **0.019%**。

禁用后恢复原窗口和托管程序，并核对所有已观察子进程退出。仅 Host 保留的 10 个完整样本中，私有工作集峰值 **9.20 MiB**、平均 CPU **0%**，通过现有 Host 空闲门槛。划词停止约 150ms，截图/置顶/程序管理约 302–603ms。五项快捷键释放、偏好保留及禁用后 Host 重启均通过。

证据：`_temp/repository-hygiene-20261008-9513b969/archived-dist/installers-model-picker-20261007-r2/verification/module-lifecycle/`。这是规定基础负载的短时记录；完整 AI/设置叠加、100 次连续任务和 24 小时稳定性仍待完成。

## 当前设置窗口启动（2026-10-07）

使用当前 `icon-quality-20261007` 四模块包和独立配置，无调试连接、无常驻设置 WebView。1 次新配置启动为 604.96ms；20 次同配置重开 P95 为 **587.78ms，未通过 300ms 门槛**。21 次各观察到 6 个 WebView2 进程并确认正常退出，最长回收 4.08 秒。首次不是已验证冷缓存启动，首页就绪不是输入到屏幕的延迟。

本次仅修正测量脚本的父 PID 复用判断，并记录可追溯的进程身份；未修改产品启动实现，不将历史数据差值当作优化收益。过程中的失败用例及两次 Chromium 诊断记录单独保留，不混入正式样本。详细阶段及限制见 [桌面启动分段诊断](桌面启动分段诊断.md)，证据在 `_temp/repository-hygiene-20261008-9513b969/archived-dist/startup-diagnostics-20261007/`。

## 独立 OCR/翻译结果窗（2026-10-06）

当前试用包为 `_temp/repository-hygiene-20261008-9513b969/archived-dist/results-ai-20261006-r2/`，3135 个文件校验通过；ZIP 为约 **189.73MiB**，含清单的展开体积约 **426.87MiB**。本轮没有重打安装器，下面此前安装器数据仍属于旧负载。

同一生成图片、两轮模拟对话、模型切换及 620×480 窗口场景：默认回收节奏下结果宿主的私有工作集曾为约 58.7MiB；调整结果宿主的 GOGC 至 50 后，最新版本为约 **28.1MiB**。未设置固定堆上限，也未清空进程工作集；Host 和 AI 工作进程沿用各自的默认配置。依据为 [Go GC 指南](https://go.dev/doc/gc-guide#GOGC) 的内存/CPU 权衡，最终仍以实际采样和交互回归判断。

最新交付场景取得 8 个完整样本、7 个 CPU 区间。Host 与结果宿主合计私有工作集峰值 **35.81MiB**，Private Bytes 峰值 **130.15MiB**，采样平均 CPU 为 **0%**；两个进程随后正常退出。该采样发生在识别/翻译和对话请求完成后，未运行设置 WebView、其他 Worker 或模型辅助进程；外部 C# 测试控制程序不计为产品进程。它不证明真实截图采集、OCR/模型运行峰值、冷缓存启动、长时负载或全硬件矩阵已经通过。

原始资源、二进制哈希、包体与流程记录保存在该包的 `verification/`。功能回归同时保持模型请求未完成，核对窗口仍能移动和缩放，八方向边缘命中正确，随后原请求正常完成。

## 此前本地包体（2026-10-06）

包含 OCR、翻译及三个基础模块的完整本地包：便携 ZIP 为 **184.93MiB**，ZIP 展开总计 **415.45MiB**。共享文件检查后的本地安装器为 **135.78MiB**。清单校验覆盖 3128 个文件，归档另含清单自身。程序 ZIP 的记录在 `_temp/repository-hygiene-20261008-9513b969/archived-dist/local-installer-debug-20261006/verification/package-sizes.json`；新安装器与编译对照在 `_temp/repository-hygiene-20261008-9513b969/archived-dist/upgrade-validation-20261006/`。尚未完成与独立工具的同功能成本对照。

## 三模块联动与回收（2026-10-06）

场景：实际 Host 同时运行原生截图编辑、带边框的置顶窗口及托管控制台。截图由 800×600 生成图片恢复；程序窗口均为专用测试程序。设置窗口、OCR 和翻译任务未运行。

两次同负载采样分别取得 30、26 个完整样本，私有工作集峰值分别为 97.77、55.34MiB，Private Bytes 峰值为 235.04、237.13MiB。保留这两组波动，不将差值视为优化收益；二进制哈希以各报告为准。最终组平均 CPU 为整机容量的 0.0507%。

原始报告包含 Host、三个 Worker、两个测试程序及测试控制台的 conhost。另存的产品报告只单列这三个测试进程，保留所有其他子进程；缺失根进程、错误归属、重复记录或不完整样本会使检查失败。转换逻辑已覆盖产品辅助进程保留与空 CPU 区间。

最终回收结果：

| 操作 | 结果 |
| --- | --- |
| 取消截图后关闭截图模块 | Worker 约 0.45 秒退出 |
| 关闭置顶模块 | 边框消失、原置顶状态恢复；Worker 约 0.60 秒退出 |
| 关闭程序管理 | 控制台恢复显示、程序保留；Worker 约 0.30 秒退出 |
| 正常退出 Host | 测试程序仍运行，窗口恢复状态保持 |

模块关闭后的 10 个完整样本仅观察到 Host：私有工作集峰值 7.90MiB、Private Bytes 峰值 52.10MiB、平均 CPU 0.0088%，通过当前空闲预算。此项仅为短时、生成输入的联动验证，不覆盖实际截图采集、输入呈现延迟、OCR 叠加负载、24 小时稳定性或独立工具成本对照。

复现：`build/test-module-lifecycle.ps1 -BundleDirectory <完整候选目录> -OutputDirectory _temp/<新目录> -GoExe <go> -RunSystemChecks`。证据：`_temp/repository-hygiene-20261008-9513b969/archived-dist/module-lifecycle-20261006/host-exit-final/`；前一组位于同级 `overlap-final/`。

## 截图工具栏修复后的 Worker 复测（2026-10-06）

新构建的截图 Worker 连续恢复并取消 100 张 800×600 合成截图，包含未完成文字输入；同一进程完成全部调用并正常退出。没有实际截屏、写系统剪贴板或强制 GC。

后 80 次 GDI 固定为 89、USER 为 69；第 41–100 次句柄固定为 627。Private Bytes 在运行中有回落，最后一次为 46.79MiB：

| 次数 | Private Bytes 范围 MiB | 平均值 MiB |
| --- | ---: | ---: |
| 21–40 | 45.19–53.02 | 49.32 |
| 41–60 | 52.02–54.95 | 53.91 |
| 61–80 | 47.57–57.39 | 54.11 |
| 81–100 | 46.79–57.75 | 52.53 |

本组未观察到后半段持续增长；只覆盖单个 Worker 的合成恢复/取消，不代表冷启动、实际输入、Host 与其他模块叠加负载或长期稳定性通过。程序集 SHA-256：`B02CA973E3EE9F1C4451A712AEA2D400B7966FBD49453841F72B238A32AF7E29`。

证据：`_temp/repository-hygiene-20261008-9513b969/archived-dist/local-capture-debug-20261006/worker-resources-100/`。复现入口仍为 `tests/capture-host-probe/run-private.ps1 -Suite production-resources`，通过 `WINONE_CAPTURE_RESOURCE_EXE` 指定实际 Worker，`WINONE_CAPTURE_RESOURCE_ITERATIONS=100`。

## 当前桌面启动复测（2026-10-06）

首页已补齐 OCR/翻译快捷键，并将就绪计时移至快捷键状态或错误提示显示完成之后。模块列表仍随读取完成显示，不等待快捷键查询。314 项前端测试、48 项普通 Rust 测试与真实窗口联调通过。

新桌面/Host 搭配现有已验证模块文件，使用独立配置连续启动 21 次。无调试连接，关闭后不保留 WebView2。首次新配置启动约 1187.95ms；其后 20 次重开为 922.05–1026.77ms，P95 **1025.18ms**，未达到 300ms 目标。门禁返回失败，原始结果保留。首次新配置不等于系统冷缓存；本次不测显示器实际呈现、管理员交接或完整任务负载。

21 次均观察到 6 个 WebView2 子进程并确认退出，关闭至回收约 519–2476ms。采样环境：i7-12700K、约 64GiB、Windows 11 build 26220。UI SHA-256：`CF7DAE477D42E6A12158CF6CC6AC98E1BD6CC2C816E98A678BAA366F7BB752E4`。

入口：`build/benchmark-component-startup.ps1 -BundleDirectory <完整程序目录> -OutputDirectory _temp/<新目录> -Runs 21 -EnforceWarmBudget`。证据：`_temp/repository-hygiene-20261008-9513b969/archived-dist/local-debug-20261006/warm-final-21/`。此前 21 次候选样本另存于同目录 `warm-21/`，不与最终候选混用；试用包基线位于 `baseline/`。本轮未更新日常试用包，未宣称启动性能改善。

## 设置窗口复用与释放评估（2026-10-05）

以已交付的同一试用包和独立空配置对照：隐藏窗口并保留 UI 时，完整样本中的私有工作集峰值为 153.38MiB，Private Bytes 为 260.83MiB；关闭释放后分别为 37.60MiB 和 49.63MiB。隐藏组 11 个样本中 10 个完整，另一个因进程性能计数器暂不可用而标记不完整；关闭组 15 个样本全部完整。隐藏后的重新显示至 DWM 同步约 14.65–16.07ms，这不是输入至屏幕呈现延迟。

关闭调试连接后，原版首次就绪约 905.5ms，后续为 491.4–599.8ms（进程内部计时）。Host 与 WebView 并行启动的首轮样本首次为 651.8ms，后续波动 509.4–831.6ms；只能确认首次启动的串行等待减少，不能认定热启动达标。

评估结论：保留“存在时复用，关闭后释放”的方式，并单独持久化窗口位置、大小和最大化状态。它更符合后台工具的轻量要求；不靠隐藏常驻满足重开速度。上述数据是短时对照，启动预算继续保留。

证据：`_temp/rust-startup-budget-20261005/retention-comparison/`、`no-debug-corrected/`、`parallel-host/`。首轮无调试采样因日志管道仍由子进程持有而中止；修正了测量工具的收尾后重新测量，没有把等待日志 EOF 算作界面存活。

## 复用现有 UI 的 Rust 宿主（2026-10-05）

设置窗口直接加载现有组件页面；截图继续使用现有原生画布。以下数据来自 i7-12700K、64GiB、Windows 11 build 26220，不继承旧 Rust 自绘原型的资源结论。

| 场景 | 私有工作集峰值 MiB | Private Bytes 峰值 MiB | 平均 CPU |
| --- | ---: | ---: | ---: |
| 设置窗口打开，含 Host 与全部 WebView2 子进程 | 138.05 | 282.40 | 0.0066% |
| 设置窗口关闭后 | 5.14 | 49.41 | 0% |
| 全部模块禁用 | 4.73 | 48.88 | 0% |
| 全部模块启用，无任务 | 4.63 | 48.82 | 0% |

开窗与关窗组分别有 25、30 个完整样本，观察到的 7 个 UI 进程均退出，关窗至回收约 493ms。另两组各 30 个完整样本，只观察到 Host，满足现有空闲预算。0% 表示采样区间内没有计得 CPU 增量。开窗资源报告对应恢复系统外框后的版本；之后只改变窗口标识、维护拒绝和就绪计时，哈希以各报告为准。

启动计时使用现有 UI 完成设置与首页加载后的 `winone-ui-ready` 性能标记；另记录下一帧机会。五次样本首次 1146.5ms，随后 536.9–559.7ms；50 次导航事件至更新后的帧机会最大约 30.5ms。后续五次启动出现 617–3984ms 波动。测试使用独立后台桌面和调试连接，不能代表系统冷缓存、显示器实际呈现或完整 P95 验收。**启动预算尚未通过；用户已授权先更新试用版，继续优化。**

证据：`_temp/rust-ui-parity-20261005/components-resources/`、`idle-final-2/`、`readiness-timing/`、`readiness-stages/`。资源复现沿用 `build/benchmark-desktop.ps1`、`build/benchmark-idle.ps1`。采样根进程提前退出时，工具现在保留不完整样本和空计数，不崩溃或伪造零占用；回归通过。

## Rust 更新入口同负载对照（2026-10-05）

使用独立空配置、已启动的 Host、后台原生窗口和同一 64 KiB 日志；每版打开并关闭 UI 五次。更新前开窗组得到 5 个完整资源样本，其他组各 10 个，均未出现不完整样本。两版采样期均未计得 CPU 增量。

| 状态 | 私有工作集峰值 MiB | Private Bytes 峰值 MiB | 句柄峰值 | GDI / USER 峰值 |
| --- | ---: | ---: | ---: | ---: |
| 更新前，日志页打开 | 8.11 | 53.34 | 662 | 60 / 90 |
| 更新后，日志页打开 | 8.59 | 54.13 | 666 | 64 / 92 |
| 更新前，UI 已关闭 | 3.98 | 47.70 | 262 | 0 / 1 |
| 更新后，UI 已关闭 | 4.55 | 48.67 | 262 | 0 / 1 |

新候选窗口就绪 52.8–100.0ms，日志全文可读 408.3–573.4ms，正常关闭 10.4–14.4ms；这些包含探针开销，不是输入到屏幕呈现的延迟。Host 文件由 7,900,672 字节增至 12,277,248 字节。未执行真实发行源检查，不覆盖联网后的峰值、真实冷启动、全模块任务或长期稳定性。

证据：`_temp/rust-updates-20261005/resources-before/`、`_temp/rust-updates-20261005/resources-after/`；二进制哈希见各资源报告。复现：`build/benchmark-desktop-rust.ps1 -HostExe <host> -DesktopExe <ui> -OutputDirectory <新忽略目录> -Runs 5 -DurationSeconds 10 -LogView`。

更新请求另有真实进程退出和本地 TLS 连接回收测试。默认共享 HTTP 连接池原先保留闲置连接，现按检查创建连接池并在结束时释放。此项验证连接回收，不替代完整网络活动资源测量。

## 截图句柄趋势复核：300 次循环（2026-10-03）

同一最新试用包 Worker 完成 300 次合成草稿恢复/取消，正常退出码 0。第 101–150 次句柄为 629，第 151–200 次回落，第 201–300 次稳定 624；第 51–300 次 GDI 为 89。Private Bytes 在第 51–100 次最高 58.82 MiB，最后 50 次为 44.20–45.61 MiB。此次未观察到持续增长，前次 100 次的句柄增量不能直接认定为泄漏。

证据：`_temp/repository-hygiene-20261008-9513b969/archived-dist/release-verification-1003/capture-300/`。使用原生恢复与取消，不截屏、不写剪贴板、不强制 GC，不包含其他模块和 Host；不能替代完整负载及 24 小时稳定性。复现设置 `WINONE_CAPTURE_RESOURCE_ITERATIONS=300`，沿用 `production-resources` 入口。

## 最新试用包截图 100 次循环（2026-10-03）

完整包内同一个截图 Worker 连续恢复并取消 100 张 800×600 合成截图，含未提交文字；重复调用保持同一任务，取消清除草稿，最终退出码 0。第 21–100 次：GDI 固定 89，USER 69–70，句柄 623–629；Private Bytes 约 40.02–57.16 MiB，最终 48.44 MiB。恢复到取消耗时 105.17–204.42ms，不是截图冷启动或输入延迟。

原始结果：`_temp/repository-hygiene-20261008-9513b969/archived-dist/release-verification-1003/capture-100/`。未强制 GC，未采集桌面或写剪贴板；不含 Host、设置宿主及其他模块。句柄有小幅增长，需更长观察，不能作为 24 小时稳定性或完整进程组验收通过。可复现入口：`run-private.ps1 -Suite production-resources`，通过 `WINONE_CAPTURE_RESOURCE_EXE` 指定实际 Worker。

## 自动补双击试用包空闲复测（2026-10-03）

最新试用包以独立空配置测量两种状态，每种约 30 秒。全部模块关闭：26/26 个完整样本，私有工作集最高 4.23 MiB、Private Bytes 47.97 MiB；全部开启无任务：30/30 个完整样本，分别为 4.33 MiB、48.44 MiB。两组只观察到 Host，CPU 有效采样区间内未计得增量。第一组采样间隔受实际采样耗时影响，未凑足或伪造 30 个样本。

两组通过现有空闲预算检查。证据：`_temp/repository-hygiene-20261008-9513b969/archived-dist/release-verification-1003/idle-auto/`。仅覆盖短时空闲，不代表三模块叠加负载、冷/热启动或长期稳定性通过。

## 0.0.1 后台空闲复测（2026-10-03）

独立配置、无用户程序和快捷键，每种状态采样 30 秒；两组均为 30 个完整样本、29 个有效 CPU 区间，只观察到 Host，未出现 Worker 或 WebView2。

| 状态 | 平均 CPU（整机容量） | 私有工作集最高 | Private Bytes 最高 |
| --- | --- | --- | --- |
| 三模块关闭 | 0.0053% | 4.69 MiB | 48.74 MiB |
| 三模块开启、没有任务 | 0%（采样区间内未计得 CPU 增量） | 4.61 MiB | 48.76 MiB |

全部关闭状态符合 CPU 低于 0.2%、私有工作集不高于 40 MiB 的预算；Private Bytes 单独报告，不与私有工作集混用。`build/benchmark-idle.ps1` 已加入预算失败检查，缺失 CPU 区间或不完整采样不能通过；边界、异常值和按时间加权的回归已通过。原始数据与模块状态：`_temp/repository-hygiene-20261008-9513b969/archived-dist/release-verification-1003/idle/`。本轮不覆盖窗口实操、叠加任务负载或长期趋势。

## 程序管理批量检查（2026-10-03）

本机只读基准检查 1、10、50 个不存在的程序，每组 3 次，平均耗时分别为 6.71、6.71、7.04 ms；未设置自动退出的 50 个程序检查为零分配。命令：在 `modules/wintray/` 运行 `go test ./internal/orchestrator -run '^TestEndOverdueProgramsWithoutLimitsDoesNotAllocate$' -bench '^BenchmarkEndOverduePrograms$' -benchtime=3x -count=1 -benchmem`。原始结果：`_temp/repository-hygiene-20261008-9513b969/archived-dist/ui-repair-1003/program-scan.log`。

此项仅覆盖程序发现与自动退出检查，不含真实收纳、窗口操作或完整进程组；样本不足以报告 P95 或长期趋势。三个模块叠加负载、冷/热启动、输入响应、CPU、内存、句柄、包体和退出回收仍须单独验收。

## 原生文字与最终测试包（2026-10-02）

连续恢复文字草稿时发现：每关闭一个原生输入框，GDI 句柄仍增加约 1 个。分离测试中，画布预热后保持稳定，文字输入框 40 次循环则增长；关闭输入框前重置其 Font 属性后，40 次循环的后半段稳定为 51。原因是 WinForms 为显式字体缓存了独立 HFONT；属性重置会释放该句柄。依据：[WinForms 10 Control.cs](https://github.com/dotnet/winforms/blob/v10.0.0/src/System.Windows.Forms/System/Windows/Forms/Control.cs) 的 Font setter 和 DisposeFontHandle。

修复后的最终包运行同一个真实截图 Worker，连续恢复并取消 100 张含未提交文字的合成截图，未强制 GC。第 21–100 次结果：

| 指标 | 结果 |
| --- | --- |
| GDI 对象 | 稳定为 89 |
| USER 对象 | 73–103，期间回落 |
| Private Bytes | 41.21–59.97 MiB，最终 52.90 MiB |
| 退出 | 全部操作完成后正常退出 |

本轮原图为 800×600，覆盖原生恢复、文字控件、取消和退出；不包含实际桌面采集、Host、WebView2、4K 图像或显存，不能视为整产品资源验收。原始数据：`_temp/native-font-bundle-resources-1002/output/resources.json`。文字控件分离证据：`_temp/native-inline-static-1002-a/`、`_temp/native-inline-font-release-1002-a/`。

## 原生 Worker 连续操作与资源归还（2026-10-02）

发现并修正两个资源所有权问题：背景绘制反复取得未释放的图像对象，窗口图标属性返回的副本没有由调用方释放。背景现由窗口持有，更新与退出时释放；图标在窗口销毁后释放。改动进入依赖准备脚本，并通过原始归档、重复执行和残缺变更拒绝测试。

原始12次重复操作中，Private Bytes约62→300 MiB，GDI对象73→109。修正后的100次重复操作（另有2次准备会话）结果如下：

| 指标 | 本轮结果 |
| --- | --- |
| Worker进程 | 同一进程完成全部调用，最后正常退出 |
| GDI对象 | 全部测量均为67 |
| USER对象 | 预热后稳定，最终67 |
| Private Bytes | 23.98–49.33 MiB，最终39.09 MiB |
| 原生回调开始至首帧 | 103.42–179.08 ms，P95为132.23 ms |

只测截图Worker自身，未包含Go Host、现有设置界面、真实采集和硬件输入；首帧也不等于可操作时点。工作集字段是总工作集，不是私有工作集。没有通过强制GC改变结果。`GC.GetTotalMemory(false)` 在第46次会话返回一条负估算值，原因未定；原值保留，不纳入内存结论，系统Private Bytes与GDI/USER计数仍有效。整机性能预算尚未通过验收。

证据：`_temp/native-capture-host-probe-1002/native-worker-100-atomic-99cffe18/output/`；原始失败及分离测试见同级 `native-worker-resources-dc44bcbd`、`components-canvas-e6b41429`、`components-toolbar-ba50f463`。首次100次尝试在第90次遭遇诊断文件半写入，未标成功；诊断快照改用原子写入，测量结果逐次保存后重跑完成。GUI对象门禁：`tests/capture-host-probe/verify-worker-gui-lifetime.mjs`，该门禁不替代内存预算与实际体验验收。

## 历史测量

2026-09-30 与 2026-10-01 的空闲、窗口和截图编辑预览采样。**仅覆盖下表场景，不代表整产品已达到轻量目标。**

## 当前测试包复测（2026-10-01）

使用 `_temp/pin-settings-1001/bundle/`，独立空配置，三模块健康检查通过。空闲预热 4 秒，设置页预热 8 秒，每组采样约 30 秒。

| 场景 | 完整样本 | 进程数 | 私有工作集采样最大值 | Private Bytes 采样最大值 | CPU 均值 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 全部模块关闭 | 26/26 | 1 | 3.77 MiB | 47.32 MiB | 未观察到可分辨增量 |
| 三模块开启，无任务 | 30/30 | 1 | 4.11 MiB | 48.09 MiB | 未观察到可分辨增量 |
| 设置页打开，第一轮 | 30/30 | 8 | 154.75 MiB | 308.79 MiB | 0.0324% |
| 设置页关闭，第一轮 | 30/30 | 1 | 4.49 MiB | 48.52 MiB | 未观察到可分辨增量 |
| 设置页打开，计时复测 | 30/30 | 8 | 123.93 MiB | 309.83 MiB | 0.0082% |
| 设置页关闭，计时复测 | 30/30 | 1 | 4.58 MiB | 48.43 MiB | 0.0027% |

两轮窗口测试使用相同二进制。打开时包含 Host、WinOne 和 WebView2；关闭后仅剩 Host。已观察到的 7 个 UI 进程全部退出，计时复测耗时 **0.80 秒**（含检测开销），满足当前 15 秒回收目标。不能将两次工作集波动解读为优化收益。

脚本现已显式打开设置页，并记录关闭耗时。历史窗口测试的具体页面未单独核验，不与本次设置页作直接内存比较。首个 CPU 间隔仍按未知处理。

本轮不覆盖截图、置顶、托管程序运行、管理员 Worker、GPU 显存或长时间循环。完整轻量化目标仍未验收。

原始报告：`_temp/resource-refresh-1001/idle/`、`desktop/`、`desktop-timed/`；含二进制摘要、模块健康及窗口退出记录。测量工具自测通过，测试进程已结束。

## 截图编辑负载复测（2026-10-01）

以下为缩略图版本的历史记录，每个分辨率独立启动，预热 100 次后测量 100 次。合成图片执行模糊、马赛克和文字预览，不截桌面、不操作剪贴板。两轮执行的托管程序集摘要一致。

| 原图 | 中位耗时 | P95 | 生命周期工作集峰值 | Private Bytes 采样峰值 | 句柄范围 | GDI / USER |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1920 × 1080 | 105.68 ms | 109.45 ms | 104.38 MiB | 60.56 MiB | 368–368 | 21 / 23 |
| 3840 × 2160 | 230.24 ms | 240.13 ms | 178.96 MiB | 129.54 MiB | 366–369 | 21 / 23 |

每轮输出均为 1600 × 900，原图记录不变，结束时仅保留一个编辑预览文件。GDI 和 USER 对象数在全部测量中保持不变；4K 句柄首尾为 368、369。

1080p 前十次与后十次的最终 Private Bytes 均值为 30.50、27.96 MiB；4K 为 27.52、28.27 MiB。短样本及垃圾回收波动不足以证明整产品没有泄漏。

本表仅统计 Worker，耗时含提交与 10ms 轮询；不含选区、Host、WebView2、前端图片解码及实际界面响应。工作集峰值包含预热，Private Bytes 为间隔采样，不能混作私有工作集或绝对峰值。

原始数据：`_temp/capture-load-1001/`、`_temp/capture-load-counters-1001/`。后者增加句柄、GDI、USER 计数，包含 100 次逐次结果及汇总。完整进程组、长时间使用和其他功能循环仍待验收。

## 原尺寸编辑预览（2026-10-01）

使用 `_temp/capture-detail-1001/bundle/`，构建结束后独立测量；每个分辨率预热 100 次、测量 100 次。模糊、马赛克和文字负载与前述测试相同，输出改为原图尺寸。

| 原图及输出 | 中位耗时 | P95 | 生命周期工作集峰值 | Private Bytes 采样峰值 | 句柄范围 | GDI / USER |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1920 × 1080 | 76.97 ms | 85.40 ms | 97.27 MiB | 53.00 MiB | 368–373 | 21 / 23–24 |
| 3840 × 2160 | 248.50 ms | 260.10 ms | 165.70 MiB | 115.84 MiB | 366–369 | 21 / 23 |

每次输出尺寸校验通过，原图记录不变，结束时仅保留一个编辑预览。前十次/后十次的最终 Private Bytes 均值：1080p 为 29.00/27.47 MiB，4K 为 28.18/28.52 MiB。

原始数据：`_temp/capture-detail-1001/resources-isolated.json`，日志同目录 `resources-isolated.log`。首轮 `resources.json` 与构建并行，排除出性能比较。

仅统计截图 Worker，不含 WebView2 解码、显示、GPU 和完整交互延迟。输出分辨率已改变，不能将本表视为同负载优化对比；完整进程组峰值仍待验收。

## 原位编辑窗口静态基线（2026-10-01）

使用 `_temp/editor-resource-1001/bundle/`，独立空配置、人工生成的 3440×1440 图像和 1600×900 选区。窗口就绪后预热 8 秒，再采样 30 秒；未读取用户桌面图像，未执行绘制操作。

| 完整样本 | 进程数 | 私有工作集采样峰值 | Private Bytes 采样峰值 | CPU 均值 |
| ---: | ---: | ---: | ---: | ---: |
| 30/30 | 9 | 149.26 MiB | 506.00 MiB | 0.0107% |

包括 Host、截图 Worker、WinOne 和 6 个 WebView2 进程。全程可见、未最小化、96 DPI、无原生标题栏，客户区 3440×1441（含 Wails 底部补偿像素）；窗口不在前台。

原始报告：`_temp/editor-resource-1001/open-warm/`。`open/` 首轮在 WinTray 空闲退出时出现 1 个不完整样本，整轮排除；`cleanup-check/` 的 2 秒数据只验证测试清理，不作为性能基线。测试进程全部退出，成功配置已移入回收站。

本轮只测窗口打开后的静态占用；不含启动峰值、绘制/预览期间峰值、GPU 显存和正常关闭。测试结束采用自有进程清理，不能作为正常关闭资源回收证据。

```powershell
./build/benchmark-capture-editor.ps1 -BundleDirectory _temp/editor-resource-1001/bundle -OutputDirectory _temp/editor-resource-next -DurationSeconds 30 -WarmupSeconds 8
```

## 环境与方法

- Windows 11 build 26220；Intel i7-12700K，20 个逻辑处理器，约 64 GiB 内存。
- 使用本地开发测试包，准确构建以原始 JSON 中的 EXE SHA-256 为准。
- 每个场景使用新建配置目录，无用户程序、自启任务或快捷键；启动后等待 4 秒，再采样约 30 秒。
- 前两轮 Host 空闲测试不打开设置窗口，不执行截图或置顶；后续场景见对应章节。观察器不计入产品进程组。
- 通过进程创建时间校验身份，跟踪根进程和已观察到的后代；不记录命令行、用户名或其他程序信息。

## 结果

| 场景 | 样本完整数 | 进程 | 私有工作集采样最大值 | Private Bytes 采样最大值 | 句柄最大值 | GDI 最大值 |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| 全部模块关闭 | 30/30 | Host | 3.87 MiB | 47.74 MiB | 239 | 7 |
| 三模块开启，无任务 | 30/30 | Host、WinTray Worker | 8.23 MiB | 96.23 MiB | 483 | 30 |

两组各有 29 个可计算 CPU 的间隔；未观察到可分辨的 CPU 时间增量。第一样本的 CPU 为未知，不按零计入平均值。未出现截图、置顶 Worker 或 WebView2。

第一组在本次条件下满足 Host 的 40 MiB 私有工作集初始预算。Private Bytes 是另一项指标，不应与私有工作集混为一谈。

原始数据位于 `_temp/resource-probe/host-disabled.json` 和 `_temp/resource-probe/enabled-idle.json`。短时工具冒烟结果位于 `_temp/resource-probe/harness-smoke/`，不用于预算判断。

## 空闲释放后的复测

完成原生初始化后，空配置且无任务、错误或恢复要求的 WinTray Worker 会退出。状态和托盘图标查询不会重新启动它；新增需要原生应用的配置仍会按需启动 Worker，且不重放登录队列。

| 相同空闲场景 | 样本完整数 | 采样进程 | 私有工作集采样最大值 | Private Bytes 采样最大值 |
| --- | ---: | --- | ---: | ---: |
| 全部模块关闭 | 26/26 | Host | 3.75 MiB | 47.08 MiB |
| 三模块开启，无任务 | 30/30 | Host | 3.96 MiB | 47.95 MiB |

开启空闲场景的 CPU 均值约 0.0027%，采样中仅有 Host；句柄最大值 255，GDI 最大值 7。相比首轮，Private Bytes 从 96.23 MiB 降至 47.95 MiB。原始报告位于 `_temp/resource-probe/after-idle-release/`，包含本次二进制摘要。

回归覆盖空闲后的连续查询、配置按需启动且不启动程序、登录退出、历史导入、模块关闭及初始化失败回退。非空程序列表仍保守保持 Worker，尚未对所有托盘/定时组合细分常驻需求。

## 可重复执行

```powershell
./build/benchmark-idle.ps1 -BundleDirectory _temp/unified-config-bundle -OutputDirectory _temp/resources/run-01
./tests/resource-metrics/tests.ps1
```

输出目录必须未存放同名报告；脚本拒绝覆盖旧基线。测试配置结束后进入回收站，报告保留。模块测试配置不允许修改真实系统自启。

手动测量正在运行的进程组：

```powershell
./build/measure-resources.ps1 -RootProcessIds 1234,5678 -OutputPath _temp/resources/manual.json -DurationSeconds 30
```

根 PID 需要包含独立启动的 Host、界面和管理员 Worker。自动发现仅覆盖观察到的子进程；不能省略脱离父进程树的 Worker。计数器读取失败会标记样本不完整。CPU 使用单调时钟，按全部逻辑处理器容量归一化。

## 截图编辑预览

使用同一台机器，每个分辨率启动独立的真实截图 Worker，生成渐变测试图，执行模糊、马赛克和文字预览。每组预热 100 次后再测量 100 次，输出预览为 1600 × 900；每轮验证尺寸，结束时确认原图记录未变且仅保留一个编辑预览文件。

| 原图 | 中位耗时 | P95 | 工作集峰值 | 采样 Private Bytes 峰值 |
| --- | ---: | ---: | ---: | ---: |
| 1920 × 1080 | 118.66 ms | 132.49 ms | 101.68 MiB | 58.89 MiB |
| 3840 × 2160 | 240.69 ms | 250.59 ms | 175.72 MiB | 128.89 MiB |

仅统计截图 Worker。工作集峰值为进程生命周期值，包含预热；它不是私有工作集。Private Bytes 每约 10 ms 采样一次，可能漏掉更短峰值。耗时包含任务提交与轮询，不包含原图获取、Host、WebView2 和界面显示；正式界面的轮询间隔与探针不同。

本轮解决了两项问题：

- 隐藏 Worker 的长轮预览减速：旧版 4K 后十次均值约 598 ms。改为仅在操作执行期间请求 HighQoS，完成或取消后恢复原策略，默认启动的长轮耗时保持约 250 ms。没有修改系统电源设置。
- 重复创建标注 JSON 配置：改为共享严格配置，保留未知字段拒绝。4K 预热后的测量段，结束状态 Private Bytes 前、后十次均值为 27.21、27.94 MiB；各十次窗口约 27.04–27.94 MiB。不能据此宣称整个产品没有内存增长。

原始数据在 `_temp/capture-preview-benchmark/shared-json-warm-100/`，包含环境、EXE 与实际托管 DLL 摘要。早期对照报告只记录了 apphost EXE 摘要，未完整标识托管代码；最终报告已补齐。调度对照在 `cpu-cycles-100/`、`no-tiering-100/`、`high-qos-100/` 和 `scoped-qos-100/`。带诊断工具的 `scoped-profile-100/` 不用于性能表；其 GC 计数用于调查内存保留。

复跑：

```powershell
./build/benchmark-capture-preview.ps1 -CaptureExecutable _temp/editor-20260930/bundle/capture/WinOne.Capture.exe -OutputDirectory _temp/resources/capture-run -Iterations 100 -WarmupIterations 100
```

`-HighQoS` 和 `-DisableTieredCompilation` 仅用于单变量诊断，改变的只是本次测试子进程；正式基线不使用这两个参数。探针不截取用户桌面，不操作剪贴板或自启。

## 滚动结果交接（2026-09-30）

拼接结果交给预览，再交给编辑或保存时，不再复制整图。引擎和预览窗口分别释放自己的引用，由接收方负责最终释放。重试期间仍保留上次结果，新结果到达后替换；失败时清理未采用的拼接图。

真实窗口拼接测试验证移交前后是同一个 Bitmap、移交后引擎不再持有结果，以及引擎关闭后图片仍可读取。原生 UI 测试覆盖取消重选、替换释放、窗口关闭和拒绝向已关闭窗口交图。日志位于 `_temp/native-checks/35426448194f4ce5920fa802bb703f3d/capture-scroll.log`，UI 测试输出位于 `_temp/scroll-ownership/ui/`。

这些证据证明移除了交接时的两次整图复制；尚未测量长页面端到端峰值，不代表整个截图流程只占用一张图片的内存。

## 设置窗口进程组（2026-09-30）

新增 `build/benchmark-desktop.ps1`：使用独立空配置，三个模块开启，无自启任务或快捷键；确认模块健康和设置窗口可见后，等待 8 秒并采样 30 秒。正常关闭窗口后确认已观察到的 UI 子进程退出，再采样 30 秒。包含 WebView2，观察器不计入产品进程组。

| 构建与状态 | 完整样本 | 进程数 | 私有工作集采样最大值 | Private Bytes 采样最大值 | CPU 均值 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 调整前，设置窗口打开 | 30/30 | 9 | 114.73 MiB | 287.24 MiB | 0.0382% |
| 调整前，设置窗口关闭 | 30/30 | 2 | 5.66 MiB | 50.52 MiB | 0% |
| GUI Host，设置窗口打开 | 30/30 | 8 | 160.88 MiB | 302.30 MiB | 0.0107% |
| GUI Host，设置窗口关闭 | 30/30 | 1 | 4.21 MiB | 47.93 MiB | 0% |

Host 改为 GUI 程序类型后，直接启动不再附带控制台进程。两轮均确认 7 个已观察到的设置窗口进程退出，关闭后新版仅剩 Host。两轮设置程序的 SHA-256 相同，打开窗口的差异主要集中在设置进程；尚未确定波动原因，不能据此宣称打开窗口的内存下降。

原始报告位于 `_temp/resources/desktop-final/` 和 `_temp/resources/desktop-gui-host/`，包含二进制摘要、模块健康和窗口退出记录。2 秒冒烟数据不用于预算判断。本轮没有执行截图、置顶或托管程序任务，也未完成独立工具对照。

```powershell
./build/benchmark-desktop.ps1 -BundleDirectory _temp/gui-host/bundle -OutputDirectory _temp/resources/desktop-run-02 -DurationSeconds 30
```

## 带窗口条件的复测

采样器现逐样本记录客户区尺寸、DPI、前台/最小化/可见状态、可用物理内存和父进程编号。关闭场景要求每个完整样本都只包含原 Host；出现新窗口或功能进程时判为无效。

2026-10-02 补强打开窗口的验收：每个样本必须完整，窗口可见且未最小化，客户区及DPI有效，进程组包含指定Host、设置进程和WebView2。失败时保留配置，不进入有效基线。合成异常场景回归通过，既有 `desktop-context-2` 的30个完整样本通过新规则复核；这不是当前版本的新增窗口测量。

`_temp/resources/desktop-context-1/` 的关闭阶段出现新设置进程和截图进程，已在 `window-lifetime.json` 标为无效，不用于关闭基线。该次新增进程的触发原因尚未确定；诊断后的临时配置已移入回收站。

`_temp/resources/desktop-context-2/` 的打开/关闭两组均为 30/30 完整样本：

| 条件 | 私有工作集峰值 | Private Bytes 峰值 | CPU 均值 |
| --- | ---: | ---: | ---: |
| 后台可见窗口，客户区 1024 × 681，96 DPI | 111.68 MiB | 284.51 MiB | 0.0081% |
| 正常关闭后，仅 Host | 4.38 MiB | 48.41 MiB | 0% |

窗口全程未最小化，系统可用物理内存最低约 22.8 GiB。此前 160.88 MiB 样本没有同期窗口条件记录，不能用本轮结果证明其波动原因或宣称内存优化。

## 2026-10-02 后台空闲复测

使用当前测试包，独立空配置，无托管程序、全局快捷键或登录任务。每组预热8秒后采样约30秒；采样前核对三个模块的实际启用状态，采样后要求全部样本完整且进程组仅包含原 Host。

| 场景 | 完整样本 | 进程数 | 私有工作集峰值 | Private Bytes 峰值 |
| --- | ---: | ---: | ---: | ---: |
| 三模块全部禁用 | 25/25 | 1 | 3.91 MiB | 47.70 MiB |
| 三模块开启、无任务 | 30/30 | 1 | 4.43 MiB | 48.45 MiB |

两组均只观察到 `WinOne.Host.exe`，未观察到模块 Worker 或 WebView2 常驻。CPU采样均值为0%，仅表示本次短时采样没有记录到可分辨增量。原始报告、二进制摘要和状态检查：`_temp/resources-1002/idle-current/`。测试配置已回收。

本轮未打开设置窗口，不代表界面打开/关闭后的回收、截图和托管任务峰值，也未与独立工具进行等负载比较。可见窗口测量等待用户允许。

`benchmark-idle.ps1` 已加入状态检查及完整样本门禁；失败时保留配置，清理仅使用本脚本持有的测试进程句柄。资源探针和 PowerShell 采样器回归通过。

## 接下来三步

1. 扩大设置窗口的重复采样，记录前台状态与显示条件，定位占用波动。
2. 记录截图冷启动与任务峰值，作为统一截图界面改造的对照。
3. 对非空程序列表细分常驻需求；保留托盘、定时任务及恢复要求，补充重复操作趋势测试。

尚未测量完整截图流程峰值、冷/热选区启动 P95、多屏/HDR，以及三个模块完整操作的 100 次循环趋势，也未与三个独立工具进行等负载比较。

## 依据

- [Microsoft：Working Set](https://learn.microsoft.com/en-us/windows/win32/memory/working-set)：驻留物理内存的页面集合。
- [Microsoft：GetProcessTimes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getprocesstimes)：用户态与内核态 CPU 时间，100 纳秒单位。
- [Microsoft：GetGuiResources](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getguiresources)：GDI/USER 对象计数与查询权限。
- 本机 `Win32_PerfRawData_PerfProc_Process` 已核对 `WorkingSetPrivate`、`PrivateBytes`、`HandleCount` 字段。
- [Microsoft：PROCESS_MEMORY_COUNTERS_EX](https://learn.microsoft.com/en-us/windows/win32/api/psapi/ns-psapi-process_memory_counters_ex)：工作集、进程生命周期峰值和 PrivateUsage 的原生字段。
- [Microsoft：SetProcessInformation](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-setprocessinformation)：HighQoS/EcoQoS 控制与系统自动推断；用于限定操作期间的调度策略。
- [Microsoft：复用 JsonSerializerOptions](https://learn.microsoft.com/en-us/dotnet/standard/serialization/system-text-json/configure-options)：选项实例的元数据缓存可跨线程复用，复制构造不复制缓存。
- [Microsoft：.NET 运行时编译配置](https://learn.microsoft.com/en-us/dotnet/core/runtime-config/compilation)：用于排除分层编译因素，产品未关闭该功能。

## 独立截图窗口静态资源与回收（2026-10-02）

使用 `_temp/capture-recovery-lifecycle-1002/bundle/` 的正常发布构建，未开启浏览器调试。控制器和应用均在不切换显示的独立 Windows 桌面中运行，96 DPI，合成3440×1440图像。窗口出现后预热8秒，每阶段采样约20秒。统计包括Host、两个WinOne界面、截图Worker及所有已观察到的WebView2后代；跨阶段保留PID与创建时间，避免漏算父进程退出后的残留子进程。

| 场景 | 完整样本 | 进程数 | 私有工作集采样峰值 | Private Bytes采样峰值 | CPU均值 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 设置窗口打开 | 15/15 | 8 | 130.29 MiB | 317.06 MiB | 0.0058% |
| 设置与截图窗口同时打开 | 20/20 | 16 | 289.18 MiB | 726.99 MiB | 0.0204% |
| 截图关闭后、Worker仍空闲待退出 | 18/20 | 9 | 139.97 MiB | 325.59 MiB | 未观察到可分辨增量 |
| 两个界面关闭，等待65秒后 | 20/20 | 1 | 4.70 MiB | 48.74 MiB | 未观察到可分辨增量 |

截图关闭后的2个不完整样本是子进程退出时无法再读取计数器，保留在原始报告中，不并入有效数据。最后20个样本均仅剩Host。截图界面进程正常关闭耗时0.352秒，设置界面进程0.014秒；这两个数值不代表整个WebView2进程组的精确回收时间。测试结束后仅对自建Host进行清理，两个界面均已正常退出。

原生状态确认采样时两个窗口均可见、未最小化。设置窗口的 HasCaption 记录为真、截图窗口为假；该字段只读取 Win32 样式位，不能据此判断 Wails 无边框窗口是否实际绘制了系统标题栏，本轮不作外观结论。

本轮测的是窗口打开后的静态负载，未包含桌面采集、首次启动、实际绘制、前台GPU负载或视觉连续性。测试图片在测量前生成，截图窗口由测试器直接启动。不能与旧版单窗口静态样本直接作收益比较，也不能据此宣布整产品轻量或无缝体验达标。

报告、二进制摘要与测量脚本：`_temp/window-role-resources-1002/isolated/`。首轮从其他桌面读取窗口失败，未产生有效样本，不参与本表。合成配置和网页缓存已移入回收站，保留原始计数报告。

下一步对照相同负载测冷/热启动与实际绘制；既有门槛仍为首次选区冷启动P95不高于800ms、热启动不高于300ms，不放宽门槛以适配当前实现。

## 预置图片的截图窗口出现耗时（2026-10-02）

同机、96 DPI、隔离桌面、3440×1440合成PNG，系统缓存未清空。WinOne使用未启用调试的内部构建，Host在计时前已通过健康检查，图片也在计时前生成。计时从创建界面进程到发现可见且尺寸正确的原生窗口，50ms轮询。**不是首次成像或首次可接受输入的测量，不包含真实截屏和图像编码。**

| 路径 | 样本 | 最小 | 中位数 | 最大 |
| --- | ---: | ---: | ---: | ---: |
| WinOne：新配置目录首次界面 | 5 | 1164.55ms | 1277.50ms | 1306.80ms |
| WinOne：同配置再次打开，Host保持运行 | 15 | 828.79ms | 855.19ms | 920.19ms |
| ShareX 21.0.0 原生参考，新进程打开 | 20 | 256.50ms | 310.91ms | 362.14ms |

原生参考直接使用 `RegionCaptureForm(Annotation, options, image).ShowDialog()`，保留其窗口与绘制实现，关闭窗口/控件检测及分享功能，禁止自动复制。它不加载WinOne配置、不包含Host通信，默认InputDelay仍为500ms，因此不能拿窗口出现时间宣称其完整输入体验达标。之前采用不同消息循环的两轮失败已排除，未修改上游代码来让测试通过。

**结论：现有全WebView截图路径不具备响应达标条件。** 仅窗口出现的所有样本已高于既定首次800ms、热启动300ms目标；不降低门槛，也不将系统缓存热度或预置图片当作实际截图性能。原生参考只用于下一轮承载原型选择，不是WinOne最终性能承诺。

## 原生承载原型启动计时复核

2026-10-02，补计创建进程至托管入口的成本。控制器在创建进程前记录单调时钟，原型按同一时钟报告首轮原生绘制结束和工具栏ready。计时依据：[Microsoft高精度时间戳](https://learn.microsoft.com/en-us/windows/win32/sysinfo/acquiring-high-resolution-time-stamps)。

六次独立进程、独立缓存、私有桌面、预置3440×1440图片，关闭浏览器调试。先运行可见初始化组，再运行隐藏初始化组；均未刷新系统文件缓存，因此不能标为冷/热启动P95。

| 初始化条件 | 样本数 | 启动至原生首帧范围 | 启动至工具栏ready范围 |
| --- | ---: | ---: | ---: |
| 可见、输入禁用 | 3 | 312.21–450.42 ms | 781.38–938.66 ms |
| 隐藏 | 3 | 296.88–302.47 ms | 796.32–813.61 ms |

原始记录：`_temp/native-capture-host-probe-1002/startup-observations.json` 及对应 `startup-*/output/state.json`。首帧仅指WM_PAINT返回，不证明桌面合成器已经呈现，也不证明鼠标选区可操作。真实采集、Host通信及实际用户输入均不在本次测量中。

当前原型仍未满足完整启动验收。两种初始化方式没有足够证据支持稳定优势，不采用隐藏常驻、透明过渡或降低门槛。下一步需补原生初始化分段、实际选区可操作时点及完整启动链路；此前托管入口计时不能替代本轮完整进程计时。

### 启动分段定位

随后两次独立缓存测量加入准备阶段时间戳，记录于 `startup-stages-1-41935c50`、`startup-stages-2-a628d5b0`（均在上述原型临时目录）。

| 阶段 | 两次耗时范围 |
| --- | ---: |
| 探针入口至打开图片 | 18.11–19.37 ms |
| 复制、解码画布 | 23.94–24.11 ms |
| 原生窗体构造 | 139.05–156.35 ms |
| 窗体构造后至挂载工具栏 | 29.21–32.48 ms |
| WebView2环境就绪至控制器就绪 | 373.52–395.81 ms |
| 控制器就绪至工具栏ready | 140.46–223.34 ms |

创建进程至探针入口分别427.02ms和61.48ms，波动原因未定，不归因于缓存或安全软件。本轮总首帧315.56/690.65ms、工具栏865.51/1301.91ms，不能只保留较快样本。

WebView2控制器创建是本轮最大的可定位阶段；只优化图片复制不能解决工具栏启动成本。已向用户提出是否允许截图工具栏采用统一风格的原生控件，以放宽全面shadcn/ui约定；尚未获得答复，不替换UI路线。无论采用哪种方案，原位文字与真实连续操作仍必须完整验收。

报告与脚本：`_temp/capture-ui-latency-1002/`、`_temp/native-capture-reference-1002/modal/`。均记录二进制摘要、计时范围及逐次样本。失败试验保留在原生参考目录上层及`probe/`中。合成配置、图片和浏览器缓存移入回收站，保留计时证据。

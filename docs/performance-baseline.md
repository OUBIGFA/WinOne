# 资源基线

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

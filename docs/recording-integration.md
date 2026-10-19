# 视频与 GIF 录制接入

实施日期：2026-10-17。需求原文保留在 [视频与GIF录制方案](视频与GIF录制方案.md)。

## 范围与已确认取舍

- 复用当前 `modules/capture/NativeUI` 的屏幕选区，不另建选区工具。固定入口顺序为 **选区 → 滚动截图 → 视频 → GIF**；视频/GIF 不进入可隐藏的标注工具偏好。
- 用户明确确认录制内存**不设置硬上限**，覆盖原方案的 60 MB 要求。仍要求有界输入、固定缓冲复用、按尺寸实测，不以缩小选区伪装低内存。
- 不调用 ffmpeg、不启动额外编码程序、不落盘序列帧。Rust 录制 DLL 由现有 C# Capture Worker 按需加载；gifski 静态链接。Chat 的独立搜索工具与本录制链路无关。
- 当前契约接收一个目标显示器。跨屏选区和小于 16×16 的选区明确要求重新选择，不偷偷裁掉超出部分。滚动长图和从磁盘重开的图片不是实时屏幕坐标，录制入口不可用。
- 本次不录制音频；原方案没有要求音轨采集。

## 能力对照

| 方案要求 | 接入位置与行为 | 验证方式 |
| --- | --- | --- |
| WGC / 系统硬编 | `modules/recording/src/capture.rs` 使用 windows-capture 2.0.1；`encoder.rs` 使用 Media Foundation/MediaTranscoder，检查 H.264/HEVC 硬件编码器并启用硬件加速 | 合成 MP4 实际编码/读取样本时间；真实 WGC 短录 |
| GIF 高质量 | gifski 1.34.0 静态库，quality 默认 85；Collector 与 Writer 分线程，明确宽高防止上游默认缩放 | GIF 解码尺寸、颜色、帧延迟与总时长 |
| 接入选区 | `NativeCaptureWorkflow.cs` 给画布选区加虚拟桌面物理原点；支持负坐标 | C# 契约测试与 Rust 几何测试 |
| 偶数与 DPI | 宽高向下取偶；最小 16×16。`physicalDesktop` 不再次乘 DPI；`logicalMonitor` 明确按显示器相对坐标乘 scaleFactor | 奇数、负原点、混合坐标空间、非法尺寸回归 |
| VFR 去重 | 固定网格/水平块指纹；未变化且未到 1 秒保活间隔时丢弃。只在入队成功后更新有效指纹 | 静止/变化/队满后保留变化测试；记录 `filterMaxUs` |
| 实际 PTS | `Instant` 单调时钟；暂停累计扣除。后一帧/End 决定前一帧 duration，末尾静止画面不会被截断 | 2 小时时间轴算术；6 秒仅 3 个 MP4 样本；实际暂停后时长 |
| 有界与复用 | 8 帧队列、10 个输入缓冲（队列 + 生产者 + 待定时长帧）；繁忙时丢帧、保留真实时间 | 池地址复用、队列统计、实际短录 |
| MP4 输入所有权 | 池化 IBuffer 由 COM 样本持有，最后一个引用释放后才归还；不提前重用仍被编码器读取的内存 | 实际编码、颜色与方向解码 |
| GIF 输入所有权 | `third_party/gifski` 只加可选缓冲归还与末帧 PTS 接口；原 RGBA 分配转移所有权，不每帧复制一份完整输入 | 合成与 WGC GIF 解码、末帧时长 |
| 隐藏工具自身 | 控制窗创建 HWND 后、显示前设置 WDA_EXCLUDEFROMCAPTURE；引擎验证只排除本进程窗口并保存/恢复原属性，捕获前同步 DWM | 首帧及后续帧解码检查：重叠控制窗下方仍是目标内容，不是黑块 |
| 长录文件写入 | 同目录唯一 `.partial` 流式写入；周期 Flush；停止排空队列并 Finalize 后用不覆盖的 MoveFileW 发布 | 写入/发布、目标文件中途出现时保护、真实保存 |
| 磁盘保护 | 启动前检查；运行中独立监督线程每秒检查，即使 WGC 没有新帧也会执行；剩余不足 200 MiB 安全停止保存 | 阈值实现与错误路径检查；未用填满真实磁盘做实验 |
| GIF 软限制 | 默认 120 秒或 1800 个接收帧，控制窗可调整时长到 1–600 秒；达到限制自动保存并说明原因 | 配置范围与停止路径；连续长时实测见未验证项 |
| 生命周期 | JSON ABI：`start_recording`、`pause_recording`、`resume_recording`、`stop_recording`、`cancel_recording`、`status` | C# ABI、UI 状态、真实暂停/继续/停止/取消 |
| 返回结果 | 路径、体积、有效时长、实际编码帧数、静止丢帧、背压丢帧、队列峰值、缓冲字节和比对耗时 | `RecordingStatus` 与测试报告 |

## 界面与内容保护

控制窗直接组合标准 WinForms 控件和现有 `NativeMenuTheme`/Lucide 图标，跟随 WinOne 语言与主题。可设置帧率、编码格式、GIF 质量/时长、是否录制鼠标。选择保存位置后才隐藏截图画布；取消选文件或启动失败保留原截图。

运行中提供暂停/继续、停止并保存、取消。录制和暂停期间保留与编码裁剪一致的黑白虚线选区框，框和面板在显示前设置捕获排除。面板提供与全局一致的 Pin / 45° 固定状态，保留原有默认置顶；取消固定后点击窗外关闭面板，正在录制时先停止并保存。取消录制仍需明确确认。窗口关闭和 Host 退出请求走停止保存，不将关闭等同丢弃。保存成功可打开所在目录。停止之后再调用取消不会删除已完成文件。

默认建议文件名为 `WinOne-<timestamp>-Video-OK.mp4` / `WinOne-<timestamp>-GIF-OK.gif`。自定义模板保留，存在 `-OK` 后缀时在其前加入类型；没有该后缀时追加类型。GIF 入口改为清晰的三个字母，不再使用 Film 图标。

视频不包含原截图标注，所以原图已有标注时会保留其截图草稿，而非以视频导出为由悄悄删除。磁盘目标文件若在录制途中被其他程序创建，保留它和已收尾的 `.partial`，报告恢复位置。

## 构建、来源与许可

- 录制入口：`build/build-recording.ps1`，独立缓存 `dist/resources/cache/rust-recording/`。
- 截图入口 `build/build-capture.ps1` 与统一 `build/build.ps1` 已接入 DLL 及 `recording-runtime.json`；旧已验证包的回退不被强制要求补新录制文件。
- 来源及原文：`third_party/import-notices.json`、`third_party/recording-notices.json`、`third_party/license-origins.json`。
- gifski 是 **AGPL-3.0-or-later**，不是 MIT。保留完整源码/许可和局部改编说明 `third_party/gifski/WINONE.md`；组合分发须满足对应源码义务，不改写 WinOne 根 GPLv3 许可。
- 接入时已补齐 58 个依赖记录。gifski 原包漏带 LICENSE，按 `.cargo_vcs_info.json` 对应提交取原文；gif-dispose 使用原始声明允许的 Apache-2.0 选项并保存声明，不编造版权文字。

## 2026-10-16 续接验收

修复 MP4 的启动时长损失：MediaStreamSource 会把首个非零 PTS 归零，现将首帧固定从录制起点计时。编码探针故意让首帧晚到 250ms，仍只提交 3 帧；MP4/GIF 时间轴均保留完整 6000ms。真实 WGC 的 MP4 解码时长为 4673.5ms，状态记录为 4673ms；暂停时间已扣除。`build/check-recording-media.mjs` 增加状态与实际视频时长的校验。

`_temp/20261016-tray-recording-chat-final/recording-3/` 包含 11 项 Rust 测试、C# 选区/工具栏/ABI 和真实录制/UI 证据；`recording-decoded-3/` 的 4 个实际 MP4/GIF 产物均为 640×360，方向、颜色及重叠控制窗排除通过。真实 MP4 此次捕获 276 帧、编码 19 帧，队列峰值 1，输入池 9,216,000 字节；这些数据不等于完整进程内存。

编译结束后，用相同 DLL 补做真实 WGC/UI 复验，证据为 `recording-live-idle/` 与 `recording-decoded-idle/`。MP4 状态 4668ms，实际解码 4668.5ms；捕获 303 帧、编码 20 帧、队列峰值 3、背压丢帧 0。GIF 编码 17 帧、队列峰值 1、背压丢帧 0。本轮最大比对耗时分别为 118.9μs、78.1μs，满足此环境下小于 200μs 的目标。

较早与重编同时进行的短录曾达到 MP4 470μs、GIF 269.5μs，原始结果保留，不能据后一次结果保证所有负载均达标。补充的原算法独立基准覆盖 640×360、641×361、1920×1080 与 3840×2160（含行填充），各 2000 次：中位数 27.7–28.6μs、p95 不超过 29.5μs、最大 161.4μs。索引预计算未有明确收益，实验代码已撤销，基准原算法结果见 `fingerprint-bench.log` 的 `before` 行。

连续两小时、混合 DPI/多屏、磁盘耗尽仍属于下方未验证范围。

本轮按用户最新要求只交付源码、隔离候选运行包及验收记录，不更新正式安装器。

## 可重复验证

在仓库根目录选择现有资源后：

```powershell
. ./build/use-local-resources.ps1
./build/test-recording.ps1 `
  -ShareXSourceDirectory $WinOneBuildParameters.ShareXSourceDirectory `
  -CargoExe $WinOneBuildParameters.CargoExe `
  -OutputDirectory _temp/<新任务>/recording `
  -RunSystemChecks
```

不传 `-RunSystemChecks` 时不捕获桌面；合成编码只写本次测试目录。显式系统测试仅录制自己创建的不透明测试窗口，不读取用户其他窗口内容。`build/check-recording-media.mjs` 用 Chromium 解码实际产物，核对上下方向、颜色、尺寸、时长和控制窗排除；测试 PNG 是界面/解码证据，不是录制合成序列帧。

2026-10-17 已完成合成 MP4/GIF、真实 WGC 短录、暂停补偿、取消清理、双主题/双语原生控制窗状态、首帧排除、文件冲突保护。详细数量与产物路径以 `docs/development.md` 的本轮记录为准。

**尚未验证：**2 小时连续实录与完整进程组的长期内存/句柄曲线；全部显卡/HEVC 编码器；真实多显示器 DPI 切换和磁盘耗尽。时间轴单测不等于 2 小时实测，也不能保证断电或进程被强杀时标准 MP4 的尾部已经写完。未宣称固定 60 MB、普遍节省 50%–80% 或任何未经测量的性能结论。

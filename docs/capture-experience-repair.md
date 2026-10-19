# 覆盖安装、窗口重显与截图体验修复

日期：2026-10-18，2026-10-19 续接中断后的录制复验与交付。基于当时工作区已有改动定点修改；未提交、推送或操作真实安装/配置。

## 修复与验收对照

| 用户问题 | 根因与改动 | 当前验证 |
| --- | --- | --- |
| `invalid upgrade journal path` 阻断覆盖安装 | 旧事务和旧 OCR/AI/Chat 依赖按当前构建清单校验，文件改名或新增必需项会使历史失效。恢复记录与旧包使用历史依赖规则；新包保持严格完整清单。仍拒绝越界、设备名、ADS、重复路径、未知根文件、变更哈希、未完成事务/标记不匹配 | 日志/旧依赖回归、隔离安装入口与恢复测试通过；完整新安装器矩阵待构建后核查 |
| 关闭/最小化重开白屏 | ShowWindow 的同步重入可能丢失 UI 状态借用期间的布局更新；隐藏与最小化时还会把视口压成 1×1。显式同步原窗口与渲染控制器的可见性，保留最后有效视口，在原生消息返回后处理同步意图，不刷新页面或重建表单 | 原生 PrintWindow 对比 6 次：完整画面约 68–139ms 恢复、像素匹配率 100%，同进程/窗口/文档。不是仅检查 DOM 或让 CDP 强制重画 |
| 滚动截图重复选区 | 原按钮未把当前物理像素选区传给采集器。现按虚拟桌面原点转换一次，直接传给同一 ShareX 采集器；显式“重截”仍可重新选区 | 选区传递、负原点、原窗口替换、取消/采集/编码失败保留草稿测试通过 |
| 长图编辑误退出 | 沿用了普通截图右键取消、双击复制退出规则；续接还发现 OCR 结果交接中与重截显示失败的分支位于防误退出判断之前，已将长图规则移到所有状态之前 | 长图普通/右键/双击/失焦不隐式结束；确认、保存、取消、关闭仍明确可用。新增交接/失败状态回归先复现右键退出，再通过；完整草稿、像素输出与六套原生测试通过 |
| 滚轮缩放 | 原引擎要求 Ctrl，普通滚轮触发放大镜；从普通截图切换成长图后，视图事件未完整接续 | 普通滚轮围绕消息中的鼠标位置缩放，逆向缩放不改变原图坐标，工具栏留在视口内；中键拖动画布保留 |
| 视频/GIF 区域不可见 | 截图画布隐藏后未保留录制边界 | 复用 ShareX 区域窗口，WinForms Region 让内部真正镂空、窗口表面只占选区外侧；灰阶虚线覆盖精确编码范围，暂停保留，结束释放；不抢焦点，显示前设置 WDA_EXCLUDEFROMCAPTURE |
| 录制置顶、外点关闭、GIF 图标、文件名 | 补同一 Lucide Pin 与 45° 固定状态；非置顶外点走安全关闭/停止保存，取消仍单独确认。GIF 使用三个字母；建议名称插入 Video/GIF 标识 | 双语双主题真控件、外点安全保存关闭、直接 WGC 与生产面板 → DLL 的 MP4/GIF 均通过。6 个成片完成解码：GIF 全帧、MP4 各 7 个时间点，尺寸/方向/四边/控制窗排除/时长一致 |
| 链接影响划词 | 行内 Hyperlink/Text 节点可能返回空或不完整选区；焦点也可能停在别的控件 | 优先最近 Edit/Document 选区，结合鼠标两端查找，逐条核对同一前台窗口祖先；不读整文、ValuePattern 或模拟复制。真实 Chromium 的跨链接/链接内/异处焦点，以及原生文本、空选区、密码、过期来源与剪贴板不变验证通过 |
| 托盘过宽、空分隔线 | Owner-draw 固定最小宽度 224，分隔符也撑宽；Host 无条件添加组分隔线 | 按文字和绘制槽位测量；分隔符不参与宽度，收纳组为空时从原生菜单移除，包含高对比度。96/144/192 DPI、动态增删/重现与原生菜单测试通过 |

## OCR 调研与取舍

用户随后特别追问新方案能否保留当前“随叫随到”。不能将低常驻目标理解为接受长闲置后的识别等待。

### 来源和能力对照

- [SnapFind](https://github.com/Loecedas/SnapFind/tree/39416254c48aa7633254ecb79f890802fea2b401)，MIT，锁定提交 `39416254c48aa7633254ecb79f890802fea2b401`。仅调研，未复制代码、模型或依赖。
- [ShareX v21.0.0](https://github.com/ShareX/ShareX/tree/d2502561f63fc3ff502cacd91514e3f7f2948c74)，既有 GPL 截图/滚动拼接/透明区域窗实现继续复用；没有引入第二套滚动编辑器。
- WebView2 可见性依据：[ICoreWebView2Controller IsVisible](https://learn.microsoft.com/microsoft-edge/webview2/reference/win32/icorewebview2controller#get_isvisible)；Wry 0.57.0 的 `set_visible` / `set_bounds` 实际实现已核对。

| SnapFind OCR 方案能力 | WinOne 对应状态/决定 |
| --- | --- |
| 后台启动推理引擎、识别前等待初始化 | 现有 Host 持有单个已预热 OCR worker；禁用模块或使用 AI OCR 不常驻本地模型 |
| 短时复用引擎、串行识别/释放保护 | 现有 Go gate、代际 idle lease、进程 Job Object 与取消所有权继续保留 |
| 闲置五分钟卸载引擎 | 做了实验和真实冷/热对照，**未采用**。当前 Python/模型重新加载的秒级开销会使快速框选后等待；实验代码仅留 `_temp/experience-repair/ocr-idle-experiment/` |
| EmptyWorkingSet、GC/MKL 缓冲清理 | 不照搬为“零成本”方案。工作集下降不等于进程私有提交释放，下一次还可能缺页；不在正式调用后强制压工作集 |
| RapidOCR 极速版 | SnapFind 当前默认是 PP-OCRv4 ONNX。WinOne 已为 PP-OCRv6 small + RapidOCR 3.10/ONNX Runtime，不为减体积降级模型 |
| 原生 PaddleOCR v6 tiny/small | 已核对其存在与上游两套架构的区别；没有证据证明换原生 Paddle 库优于现用 v6 ONNX，故不盲换、不伪称引擎名称变化就是质量升级 |

### 实测

使用 `_temp/20261018-installer/build/capture/Ocr/` 的现有正式构建运行时，生成小段与中英多行图片，经真实 Python、模型与字节管道识别。没有清空系统文件缓存，也不是物理热键到剪贴板的全链路计时。

- 已就绪小段文字：约 **17–20ms**；多行文字约 **62–72ms**。
- 新进程显式加载/预热：本次 **1431.5ms**。
- 本次首个未预热进程直接识别：**5619.5ms**；单次数据不代表稳定冷启动分布，但足以否定无条件等效。
- 预热后的单个 OCR 进程约 108.7 MiB 私有提交、132.5 MiB 工作集。这不代表整个 WinOne 进程组，不能说零开销。
- 复用资源目录的旧脚本不支持零长度预热协议，首次基准因此失败；该组未作为正式版结论。打包脚本本来就从源码重新复制适配器，正式构建脚本/模型已单独核对。

结论：保留现在已就绪模型，不把五分钟卸载或截图时临时加载实验带入安装器。进一步降低开销必须先证明不降低当前体验。尚未做 SnapFind 原生 Paddle/v4 与当前 v6 的完整识别质量语料对比。

## 2026-10-19 续接复验

- `build/test-recording.ps1 -RunSystemChecks` 现在强制执行 `build/check-recording-media.mjs --panel`；仅生成文件、状态返回 completed 不再足以让总入口通过。
- 实录探针在窗口完成原生显示后建立测试窗口顺序；新增不同颜色的自有底层窗口，录制与暂停/静止等待期间检查遮挡，失败时取消未完成测试录制。实际发生的浏览器/独立 WinTray 菜单遮挡被检测并中止；未关闭用户程序。后续把自有测试窗口移到工作区右下侧再完成复验。
- `resume-recording-5/`：11 项 Rust 单测、原生选择/工具栏/ABI、双语双主题/外点保存关闭与两条实际录制路径通过。6 个 640×360 产物：合成 GIF 3 帧、直接 WGC GIF 17 帧、生产面板 GIF 9 帧全部解码；3 个 MP4 各取 7 个时间点。边缘与重叠面板下方像素正确，实际时长与状态误差小于 40ms。
- `resume-scroll-red/` 明确复现 `Long-image recognition was dismissed by 0x205`。修正状态判断顺序后，`resume-capture-green/` 的 workflow、scroll-workflow、facade、autosave、pointer、text-recognition 全部通过；追加的识别交接/重截显示失败用例验证右键、双击、失焦保留编辑器，Escape 仍可显式退出。
- 完整构建首轮在 `internal/maintenance` 的进程位置检查处遇到 `Access is denied` 而停止，没有跳过失败继续打包。后续用保留的真实进程句柄稳定复现另一退出快照错误：进程已经结束，`QueryFullProcessImageName` 返回 `ERROR_GEN_FAILURE`。`processInInstallation` 现持有同步句柄，只在同一进程已确定退出时忽略查询/路径错误；对存活或无法确认的进程仍拒绝维护。新增回归先失败、后通过，安装与维护测试连续 10 轮通过（`process-snapshot/red.log`、`green-2.log`）。
- 成片解码入口补上浏览器启动失败的资源清理；六个录制产物复验通过，缺失浏览器的负向测试非零退出且不残留本地服务（`resume-recording-decoded-final/`、`decoder-browser-failure.log`）。
- 用户于 2026-10-19 要求将“Chat界面交互与边栏布局优化需求”一起构建，包括后续的自定义供应商 HTTP 地址和密钥反复查看。最新工作区已包含相关实现；本次重新统一构建并复验，不仅沿用另一会话的完成声明。
- 合并构建的真实 `--check-ui` 又发现可见性同步回归：初始化中的隐藏窗口和有界启动探测被误当作用户隐藏窗口，控制器暂停且不再更新探测尺寸。现保留首次原生窗口初始化前的活动渲染，只让已初始化后的隐藏／最小化窗口暂停；新增状态组合回归，补建 Rust 后真实维护锁下的启动探测连续三次通过（约 2.99–3.24 秒）。
- 构建源码前后指纹确认只变更了 `apps/desktop-rust/src/component_ui.rs`，因此保留此前通过且未变的前端/Go/模块构建，重建 Rust 后继续执行原统一入口剩余的启动、Host、集成、许可和载荷记录检查。原失败日志和 `build-combined-result.json` 保留，续验单独记录，不将失败轮次改写成通过。
- 完整构建、候选实窗与安装器矩阵继续验证，最终状态以交付记录为准；本节单项结果不替代安装验收。

## 证据与复现入口

临时证据集中 `_temp/experience-repair/`：

- `installer-regression.log`、`distribution-tests-2.log`：历史恢复、旧依赖、路径/哈希边界及隔离安装入口。
- `capture-tests-2/` 的 workflow/scroll-workflow，`capture-tests-3/` 的 facade/autosave/pointer：真实原生消息、草稿和输出像素。
- `selection-live-2/results.json`：真实 UIA/Chromium 選区，所有用例仅使用生成文本。
- `ui-resume-private/result.json` 和 `native-frames/`：原生窗口重显像素与延迟。
- `recording-final/`：上轮原生状态与短录资料，不作为最终成片通过证据。`resume-recording-5/decoded/report.json` 与 `live/` 是续接后通过的最终录制/UI/解码证据。
- `ocr-package-cold/worker.json`、`ocr-package-prepared/worker.json`：正式运行时冷/热对照。

统一入口：`build/test-capture-native.ps1`、`build/test-recording.ps1 -RunSystemChecks`、`build/test-selection-reader.ps1`、`build/test-desktop-components.ps1 -TestScript rust-ui-resume-check.mjs`，构建使用 `build/build.ps1`，之后 package → installer。

## 未验证边界

尚未验证：全部 UIA 提供程序/浏览器、混合 DPI 多屏切换、全部 GPU/HEVC、两小时实录、真实休眠恢复、干净机器安装和长期稳定性。全量构建与新安装器矩阵状态在交付时更新；原生单项测试不替代安装验证。正式安装由用户自行执行。

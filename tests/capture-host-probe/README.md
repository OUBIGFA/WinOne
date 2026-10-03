# 原生画布承载原型

真实 Host 配置联调由 `apps/host` 的 `TestNativeCaptureConfigurationRoundTrip` 驱动。设置 `WINONE_NATIVE_CONFIG_PROBE` 为已构建原型的绝对路径、`WINONE_NATIVE_CONFIG_DATA` 为新的隔离目录后显式运行；测试启动 C# 子进程，复用 Go Host 正式配置队列和持久化，验证空列表、默认值、部分工具及原生启动读取。未设置时跳过，不操作用户配置或托盘。

`--native-toolbar-customization <新目录>` 验证原生勾选菜单、失败保留、重试、恢复默认与关闭后完成，必须使用私有桌面。`--native-configuration-lifetime <新目录>` 以隐藏 Worker 和真实管道检查保存时拒绝退出、完成后退出；配置服务为测试响应，不作为真实 Host 持久化验收。

`--native-workflow-probe <新目录>` 在私有桌面验证正式 `NativeCaptureWorkflow`：合成帧、真实输出服务与原生会话，覆盖整屏/选区直接保存、保存失败重试、编辑复制、取消、窗口布局变化及配置快照。不会调用系统剪贴板或采集用户桌面。

原生模式通过 `NativeSessionProbe` 观测正式模块的 `NativeCaptureSession`；状态写盘和计时仅在测试层。正式会话不创建浏览器控件、不读取原型资源、不写测试文件。`HostedToolbar` 仅保留浏览器对照入口，原生 Worker 和控件回归使用正式会话。真实屏幕采集、Host 默认调用和用户测试包尚未切换。

实时原生状态由 `NativeSnapshotServer` 通过同用户命名管道提供，使用长度帧和读取确认；初始/最终状态保留为文件，`state-complete.json` 标记最终状态。JavaScript 使用 `native-state.mjs`，PowerShell 使用 `Read-SharedProbeState`，不得轮询原生会话的 `state.json`。旧浏览器对照仍采用原文件协议。已有本地启动脚本的就绪检查也须切换到共享读取函数。

Worker 回归现在调用完整 `NativeCaptureWorkflow`，仅替换采集图像与窗口列表；`NativeSessionProbe` 可借用会话，只负责观测。`WINONE_PROBE_LANGUAGE=en-US` 用于原生工具栏渲染，检查长标签下的可达操作和两个溢出菜单。

`NativeWindowRegions` 复用正式窗口细分策略，将有序桌面坐标候选送入原生引擎；绘制工具暂停查询和吸附。`WINONE_PROBE_EMPTY_SELECTION=1` 加 `WINONE_PROBE_TEST_WINDOW_REGIONS=1` 使用合成窗口区域验证鼠标消息、内部细分及边缘恢复。真实窗口快照必须在显示截图窗口前取得；当前测试不证明真实应用识别或硬件鼠标行为。

`WINONE_PROBE_EMPTY_SELECTION=1` 让Worker测试从无选区开始。管道回归检查隐藏输出控件、空确认明确反馈、无图片输出及取消；新区域拖动期间继续保留已确认选区，原生完成事件才替换。此模式不模拟硬件拖选或窗口检测。

Worker原型的“截图置顶”使用正式Pin服务，完成截图窗口后继续持有贴图。设置 `WINONE_PROBE_TEST_PIN=1` 验证真实贴图存续、关闭、无额外复制及Worker退出；`WINONE_PROBE_PIN_COUNT=20` 增加重复次数并检查末段GDI计数稳定。独立窗口原型没有Worker生命周期时不启用该入口。

Worker连续验证可设置 `WINONE_WORKER_REPEAT=100`，逐次原子保存计数、首帧耗时与自然GC信息。使用 `verify-worker-gui-lifetime.mjs <repeat-observations.json>` 核对同进程及GDI/USER归还；异常托管估算原样报告，不钳制成零。该门禁不判定内存预算或真实截图性能。`test-source-resources.ps1 -Archive <锁定归档> -OutputDirectory <新目录>` 检查资源所有权适配从干净源码可重现，并拒绝残缺修改。

`--worker-probe <合成PNG> <工具栏资源> <新输出目录> <管道名>` 仅在自建私有桌面运行，使用正式CaptureApplication的管道、STA、互斥、任务记录和退出逻辑。`native-worker-check.mjs` 验证确认、取消、编辑期间拒绝重复任务/退出、同进程再次截图及空闲后主动退出。真实屏幕采集由合成图替代，复制使用隔离接收器。正式Program尚未注入原生处理器，因此默认快捷键路线没有切换。

共享输出模式提供原生“另存为”按钮和Ctrl+Shift+S，复用正式输出端口。`WINONE_PROBE_SAVE_AS_CASES=1` 将路径选择替换为隔离测试，依次取消、返回不可写路径、返回有效路径；验证截图保留、重试和不重复复制。未设置该测试变量时使用正式系统文件选择框；该系统对话框尚未实测。

设置 `WINONE_PROBE_SHARED_OUTPUT=1` 时，原型引用正式截图模块的 `NativeCaptureOutput` 端口，复用配置快照、文件命名、防覆盖写入及最近结果持久化。测试只写隔离目录，复制仍由图像接收器代替系统剪贴板。`native-controls-check.mjs` 验证真实PNG名称、尺寸和输出数量；这不表示Host快捷键及真实截屏入口已经切换。

编辑工具已补齐椭圆、画笔、实色遮盖。椭圆与画笔复用上游绘制；遮盖使用独立不透明填充图层，共用原生创建、移动、缩放与历史。上游描边路径会缩小填充区域，不能直接充当遮盖；本实现按被选区域覆盖像素，整数及小数边界均有回归，未通过扩张几何补边。输出、撤销重做、遮盖与矩形样式互不污染已有验证，真实拖绘仍待验收。

工具栏、完成事务、定位及图标已移入 `modules/capture/NativeUI`，测试通过项目引用使用同一份实现。图标由 `build/generate-capture-icons.mjs` 在开发时生成并嵌入截图模块程序集，运行时不依赖浏览器。握柄、显示器约束及原生溢出菜单继续沿用已有验证。

`--native-toolbar-render-probe <新目录>` 只允许在自建私有桌面运行，输出标准宽度和320px宽度的实际控件渲染。静态渲染与六个显示器边界几何案例通过；握柄定向消息及后续完整原型操作通过。真实鼠标、多DPI、深色主题及完整工具集尚未验收。

原生文字可行性入口：`--native-text-render-probe <新输出目录>`。标准Windows EDIT控件通过WM_CTLCOLOREDIT显示画布背景；导出在原生HDC上按照EM_LINEINDEX、EM_LINELENGTH、EM_POSFROMCHAR给出的实际分行绘制，不另行计算换行。中文、自动换行、制表符、文件名标点和空白五组样本与控件DrawToBitmap逐像素一致。原生文字已接入原型会话；真实IME、滚动与混合DPI仍未验收。

依据：[WM_CTLCOLOREDIT](https://learn.microsoft.com/en-us/windows/win32/controls/wm-ctlcoloredit)、[EM_LINEINDEX](https://learn.microsoft.com/en-us/windows/win32/controls/em-lineindex)、[EM_POSFROMCHAR](https://learn.microsoft.com/en-us/windows/win32/controls/em-posfromchar)。通用DrawText的DT_EDITCONTROL仅承诺部分编辑控件特性，实测文件名标点换行不同，因此未采用双排版路径。

仅用于可行性验证，不进入 WinOne 发布包。

用户已授权截图工具栏使用统一风格原生控件。设置 `WINONE_PROBE_NATIVE_TOOLBAR=1` 启用同窗口的两行标准ToolStrip；此模式的工具栏与原位文字均不创建WebView2控制器。默认未设置时保留浏览器工具栏作为对照。两条路径共享原生引擎、确认事务和键盘操作。

`native-controls-check.mjs` 根据真实原生子控件句柄定向投递消息，覆盖撤销、重做、保存、文字、确认及单窗口身份；未调用系统剪贴板。已覆盖原生文字修改取消、标注撤销重做、超长内容拒绝保存及修正后重试；图标、完整布局及实际鼠标验收未完成，不能作为最终产品界面。

- 使用 ShareX 21.0.0 的独立源码副本，放在忽略的 `_temp/` 目录；不能修改产品依赖 checkout。
- 构建传入 `ShareXSourceDirectory`；依赖准备流程为引擎项目加入统一的 `NativeUI/ShareXHost.targets` 导入，不再使用逐引用的全局属性。已有隔离副本先加载 `build/sharex-native-host.ps1` 并执行 `Initialize-ShareXNativeHost <副本目录>`。
- 后续还原使用 `dotnet restore --locked-mode`，同时传入该属性；SDK 版本及包内容摘要由 `packages.lock.json` 锁定。
- `modules/capture/NativeUI/Engine/ShareXCaptureAdapter.cs` 作为同程序集的适配层编译，不重写上游文件。选区、绘制、效果和历史继续由原生引擎负责。
- `--self-test` 验证原生标注、输出、撤销及确认失败重试，不显示窗口。
- WebView2 SDK 固定为1.0.4258.31。同窗口工具栏已接入工具选择、撤销、重做、原位文字、保存、确认及取消；完整鼠标流程和产品启动性能尚未通过验收。

工具栏单独构建：在前端目录运行 `vite build --config tests/native-toolbar.vite.mjs`，产物进入忽略的原型目录，不覆盖产品页面。随后用 `--show-probe <合成PNG> <工具栏产物目录> <新的输出目录>` 启动原型；图片必须匹配桌面尺寸。这会显示原型窗口，只能在获准的桌面环境运行。

该入口预置一个选区和矩形，仅验证控件与引擎的通信，不代表鼠标框选已验收。`native-toolbar-check.mjs` 检查真实控件、实时原生窗口及父窗口身份、输出和正常关闭。原生图像及历史始终由 ShareX 引擎持有；浏览器没有截图画布。

诊断环境变量仅用于测试：

- `WINONE_PROBE_CDP_PORT`：本地调试端口。
- `WINONE_PROBE_WEBVIEW_PROFILE`：复用自己的隔离浏览器缓存，禁止指定用户浏览器目录。
- `WINONE_PROBE_VISIBLE_INIT=1`：对照隐藏初始化的节流成本。可见但禁用输入的初始化策略尚未通过透明显示和鼠标行为验收，不能作为产品默认方案。
- `WINONE_PROBE_LAUNCH_TICKS`：控制器在创建进程前记录的 `Stopwatch.GetTimestamp()`；输出增加从启动请求到原生首帧、工具栏就绪的耗时。未提供时这两个字段为空，不能将托管入口耗时冒充进程启动耗时。

原型不得以测试通过替代产品验收。只有统一工具栏、单选区、原位文字、连续效果绘制、双击确认、右键取消、输出失败重试和窗口退出一起通过后，才考虑产品接入。

原位文字验证使用现有 `CaptureInlineText`，输入区和工具栏属于同一个 WebView2 控件，通过原生窗口区域仅保留各自的绘制和命中范围。中间区域仍交给原生画布。原生输出复用既有 Segoe UI/GenericTypographic 文本约定，文本进入原生历史，未调用上游文字对话框。

`native-text-check.mjs` 已覆盖取消文字、多行中文、直接保存时提交文字、撤销重做和正常退出，并读取实际原生区域及窗体数。文字按钮现只选择工具，输入位置由原生点击消息确定；命中已有文字时重新编辑该图层。定位、再次编辑、取消、保存与一步撤销已通过定向消息回归；硬件鼠标、完整输入法和多DPI仍未验收。输入与输出的字形边界还观察到约1–2像素差异，不通过补偏移量来掩盖，也不宣称所见即所得验收完成。

SDK 许可：[Microsoft.Web.WebView2 1.0.4258.31](https://www.nuget.org/packages/Microsoft.Web.WebView2/1.0.4258.31/License)，BSD三条款，原文见 `LICENSE-WebView2.txt`；运行时复用系统已有 WebView2。原生引擎许可沿用项目已记录的 ShareX GPL。

`native-text-edit-check.mjs` 验证已选中文字重新载入、原位编辑、取消不改原文和一步撤销；输出像素校验记录在本地原型报告。清空文字及撤销恢复由原生自测覆盖。此路径依赖当前原生选中项，不代表鼠标选择流程已经验证。

`native-text-pointer-check.mjs` 经 `click-private-window.ps1` 定向投递 Win32 消息，验证点击坐标、越界拒绝、命中文字和第二个文字对象。辅助程序校验本原型路径、PID、窗口及私有桌面名称，并在专用线程中发送；不移动系统鼠标，不对用户桌面发送消息。它不代表硬件鼠标手感验收。旧文字与重编辑脚本已改用相同的原生消息路径。

`native-confirm-check.mjs` 通过 `WINONE_CONFIRM_CASE=retry|text|empty-text|cancel` 验证复制失败保留截图、重复确认、文字提交后确认和右键取消。`retry` 需同时设置 `WINONE_PROBE_COPY_FAIL_ONCE=1`。接收器将结果写入隔离目录的 `confirmed.png`，不访问系统剪贴板；确认不隐式保存其他文件，取消不产生复制结果。空白文字与非空文字使用同一提交路径，均继续待执行的确认或保存。

每个隔离桌面须使用独立的 `WINONE_PROBE_WEBVIEW_PROFILE` 和调试端口；Node 的 `WINONE_NATIVE_PROBE_CDP` 必须匹配该端口。浏览器进程退出与宿主退出并不同步，跨桌面复用缓存或端口会导致缓存占用或连接旧端点。此类失败应保留证据，不能靠产品重试掩盖。

单选区适配在原生 `ShapeCreated` 完成事件替换旧区域，不在拖动途中丢弃原选区。工具栏撤销/重做保留当前截图范围，输出要求恰好一个有效选区，不再默认取最后一个。`SingleSelectionTests` 覆盖换选区、标注撤销/重做和输出尺寸；不覆盖硬件拖选与原生快捷键。

完整双击不能用当前消息辅助器验收：上游 `StartRegionSelection` 会刷新 `InputManager`，由 `MouseState.Update` 读取系统鼠标位置及按键。定向消息中的坐标无法替代这份状态。`double-message` 只验证确认入口，禁止将其报告为完整双击流程通过。

键盘入口已移除上游独立文字、剪贴板和历史命令，由统一工具栏处理确认、保存、取消和历史。`native-keyboard-check.mjs` 覆盖页面焦点回落后连续撤销/重做、原生T消息进入原位文字、文字Ctrl+S保存及原生Esc消息退出。文字输入自身的复制、撤销与IME仍交给输入控件。该验证使用CDP及定向消息，硬件组合键和真实输入法仍待验收。

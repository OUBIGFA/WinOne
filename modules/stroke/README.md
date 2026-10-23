# Stroke 鼠标手势

复用 [poerin/Stroke](https://github.com/poerin/Stroke) 的固定提交 `cc72a7a73f98e6f9c5af0f9f1871bd006118b9c9`。原始文件放在 `upstream/`，逐文件摘要见 `third_party/stroke.json`。实际 `LICENSE` 为 GPL-3.0；上游 README 的 MIT 描述与其冲突，本包保留实际许可证，不标注为 MIT。

WinOne 接管设置、启停、进程和错误反馈。模块 ID 与 RPC 命名空间均为 `stroke`。配置中心生成 `modules/stroke/managed-settings.json`，worker 只读；缺失或损坏时明确失败，不读取独立 Stroke 的二进制 Settings。新配置只启用资源管理器文件夹范围。匹配须同时满足系统 `explorer.exe` 的完整路径及 `CabinetWClass` / `ExploreWClass` 根窗口类；桌面、任务栏及未知窗口在吞下鼠标按键前放行。

## 2026-10-23 输入重构

只用当前场景中启用的动作建立候选。128 向量配置和录制格式继续保留；识别转换为八方向、多段序列，每移动 15 像素更新候选。失配从原始按下位置回放完整轨迹，随后交还原生输入。按住 250 毫秒未形成轨迹，或起笔时按 Ctrl / Shift / Alt，也进入原生操作。

执行目标固定为起笔窗口。未分配的滚轮和第二鼠标键按原顺序回放。按用户精简要求取消额外按键叠加，脚本参数保持兼容但掩码恒为 0。设置页收敛为使用场景、手势、常规、排除和高级；动作包、编译、执行线程放在高级内。

build/build-stroke.ps1 -Test 验证状态机与轨迹绘制。显式设置 WINONE_STROKE_INPUT_TEST=1 后还会安装短暂测试钩子，在自有测试窗口验证真实 SendInput：未启用垂直右键拖拽、启用水平手势、未分配滚轮。默认测试不注入系统鼠标输入。

## 能力与兼容对照

| 上游能力 | WinOne 入口与执行 | 核查 |
| --- | --- | --- |
| 20 个预置轨迹与 128 向量配置 | 保留录制格式，执行时转为八方向候选序列 | 原生状态机与真实输入往返测试 |
| 新增、删除、改名、预览、重新绘制、中键清空 | 手势列表及右键菜单；点击预览或录制按钮；空白手势保留但不参与匹配 | 前端界面；实际 worker 接受空轨迹配置，录制生成 128 向量 |
| 原版 10% 混合学习 | 录制弹层默认保留混合选项，使用当前草稿的旧向量，不丢掉连续未保存的学习结果 | `record.read` 使用原版 0.9 / 0.1 公式；GUI 真 worker 录制 |
| 五种手势按键、7 种特殊手势 | 常规页选择触发按键；动作下拉提供 #0–#6；额外按键叠加已精简，掩码为 0 | 未启用特殊键回放、原生右击和取消测试 |
| 8 个默认动作包、31 个默认动作 | 动作树完整保留全局、媒体、窗体、标签页、资源管理器、网页浏览器、应用软件、键盘包 | `TestDefaultsRetainEveryOfficialReleaseAction` 逐项比较原始脚本和手势 |
| 动作包/动作管理、C# 编辑及优先级 | 动作树、右键菜单、名称/手势/脚本编辑；包上移下移；后面的包优先 | 保存、原始数量与 UI 检查；完整脚本实际编译 |
| 每包进程路径正则、排除规则、十字准星添加路径 | 应用范围关联动作包；支持每行一条 .NET 正则；拖动添加路径或选择窗口；排除标签 | Host 原生窗口/进程路径读取；原生匹配策略。Go 不用 RE2 拒绝 .NET 特有语法 |
| 画笔颜色、0–100% 透明度、0–10 像素宽度、隐藏轨迹 | 常规标签；保留原版 `Pen` 语义；WinForms `OnPaint` 绘制保留线段；0 不透明度或宽度隐藏绘制 | 连续手势与追加线段的原生窗口像素检查、重绘与清空检查、GDI/DC 释放及 DPI 检查 |
| 运行时 `Base.PenColor/Opacity/Thickness` | 脚本执行器回传画笔变化，主 worker 更新轨迹层 | 保留 Base API；事件与轨迹层共用配置对象 |
| 37 个默认命名空间、8 个默认引用、自定义 DLL | 编译标签完整保留；显式 DLL 完整路径和依赖解析；不扫描目录中的全部 DLL | 原始默认值对照；真实外部 DLL 插件加载测试 |
| 已编译插件的 Base / Stroke 程序集引用 | `Base.dll` / `Stroke.dll` 转发公开类型到 `Stroke.Base.dll` | 用官方原始 Base.dll / Stroke.exe 作为只读引用编译插件，再验证其二进制调用新输入通道 |
| 1–64 个 STA 执行线程，默认 8 | 执行线程标签；一个惰性执行进程内按需建立带消息泵的 STA 线程 | 真实双 STA 动作共享状态；不在启动时预建 8 个线程 |
| 跨动作共享 `Base.Data` | 同一个脚本执行进程共享原字典 | 两个并行动作共享字典值的实际进程测试 |
| `Base.KeyPoint` 与默认目标窗口 | 每个动作保留触发时上下文，通过执行上下文传递到后台 Task | 原生起点、窗口身份及旧二进制插件的 Task.Run 测试 |
| Base 键鼠、媒体、窗口、置顶、程序/文件启动 API | 编译原始 `Base.cs`，修正 hook 错误处理与 x64 INPUT；键鼠输入通过受管通道 | x64 INPUT 为 40 字节、union 偏移 8；模拟 Ctrl 恢复及用户已按下按键保护 |
| `Base.KeyboardHook` 和持续订阅 | 保留上游事件 API 与 STA 消息泵；停止执行器时解除或由进程退出回收 | 原始接口编译并保留。未对真实用户键盘安装测试钩子 |
| 正常启停、异常脚本、无限等待脚本 | Host `Lazy` / Gate 管理；先释放鼠标钩子与轨迹，再关闭已确认的执行进程 | 无限脚本退出、父进程消失、模拟按键归还测试 |
| 脚本启动独立应用 | 保留 `Base.Run`；没有“结束整个后代树”或按名字 Kill | 隔离 fixture 应用在 Stroke 退出后仍存活的测试 |
| 登录启动、语言、错误反馈 | 使用 WinOne 全局登录启动、中英资源与模块状态/诊断；不另建托盘或计划任务 | 由 Host 集成统一入口；`strokeStatus` 不启动空闲 worker |

上游默认值来自官方 `2026-05-01/Stroke.zip` 的 Settings 记录，使用系统 SDK 的 `System.Formats.Nrbf` 解码为记录数据，没有使用 BinaryFormatter 或实例化其中的类型。原始解码数据保存在 `upstream/defaults-2026-05-01.json`。仅作两项明确适配：其他应用范围默认关闭；原版打开 `C:` / `D:` 的两个分支改用用户目录 / 此电脑，保留两个可编辑的分支。

## 生命周期与协议

worker 启动参数：`--pipe <name> --data-dir <module-directory> --parent <host-pid>`。初始进程不安装钩子；Host 确认启用后调用 `activate`。管道限当前 Windows 用户、拒绝远端，并核对真实客户端 PID 等于父进程。请求遵循 WinOne 1 MiB、有长度前缀的 JSON 帧协议。

- `snapshot`：返回进程身份、受管配置、钩子、动作进程/线程、排队与诊断。
- `activate` / `settings.reload`：先完整校验与编译，再切换设置；失败保留原运行配置。
- `record.start/read/cancel`：录制会话只接收设置画布坐标，调用 Stroke 原算法。迟到取消不会清除新的录制会话。
- `shutdown`：停止新输入、释放钩子与轨迹、归还 Base 发出的模拟按键、取消队列、等待或结束自身执行器，确认后退出。

脚本不是沙箱。执行器保持 Base 共享状态与键盘事件能力，默认不限制脚本时长；可选超时为 0–3600 秒。超时会结束当前执行器中的动作，下一次请求重新创建执行器。用户脚本自行 P/Invoke 或启动的外部软件不在 Base 输入账本内，也不会被当作可递归终止的进程组。

`--fixture` 仅供显式隔离测试：仍创建真实轨迹层、编译器和执行进程，但不安装全局鼠标钩子，Base 输入送到记录器。它不是正式运行的降级路径。

## 构建与验证

`build/build-stroke.ps1 -OutputDirectory <本次根目录下的 _temp 路径> -Test` 使用现有 Roslyn 编译器和 Windows .NET Framework 引用，不下载或安装运行时。正式运行只需要 Windows 11 自带的 .NET Framework 4.8；.NET SDK 不随包分发。

产品文件为 `WinOne.Stroke.exe`、`Stroke.Base.dll`、`Base.dll`、`Stroke.dll`、`Stroke-LICENSE.txt`、`schema.json`、`defaults.json`、`migrations.json`。四个二进制放在同一目录。`sources/`、`tests/` 和 `WinOne.Stroke.Tests.exe` 是验证产物，不是模块运行依赖。

- 设置、schema 和完整默认动作：`go test ./modules/stroke`。
- 真实进程、录制、DLL、共享状态、退出：设置 `WINONE_STROKE_TEST_EXE` 指向候选程序后运行同一 Go 测试。
- 旧二进制插件：构建时用 `-LegacyReferenceDirectory` 提供固定发行包的原始 Base.dll / Stroke.exe；脚本核对摘要，只作编译引用，不执行它们。随后同一 Go 测试验证类型转发、后台 Task 上下文和受管输入。
- GUI：`tests/mouse-gestures-check.mjs` 使用隔离 profile 和真实 worker，覆盖中英、深浅色及 520/1000 px；仅由夹具记录输入。
- 性能：GUI 脚本调用既有 `build/measure-resources.ps1`，采样整个 worker/执行器组；报告明确为未安装全局钩子的夹具场景。

物理鼠标、真实资源管理器前台操作、真实用户键盘钩子、UIPI/提权窗口及多显示器实机轨迹未在自动验收中执行。测试遵守不干预用户操作的边界，不能将上述范围标为已完成实机验证。

# 截图片段收集与识别结果编辑

日期：2026-11-28。SnapFind 固定提交 `39416254c48aa7633254ecb79f890802fea2b401`，MIT。保留 WinOne 当前 Tools3000 截图画布、RapidOCRLite / PP-OCRv6 small、共享结果窗和 OCR 就绪策略。

## 功能对照

| SnapFind 能力 | WinOne 接入 | 验证 |
| --- | --- | --- |
| 同屏多选区、序号角标、按序识别 | 截图工具栏“连续采集文字”；截图内默认 Ctrl+Shift+C，可在快捷键页调整。继续框选即加入列表 | 私有桌面生成画面、片段尺寸与顺序检查 |
| 默认多选区开关 | 截图后 → 连续采集 → 截图时默认连续采集文字；默认关闭，设置保存后跨启动保留 | 设置页保存/重读测试 |
| 跨应用切换、顶部暂存条 | 点击“切换界面”或画布中按 Tab；已有片段留在暂存条。点击“继续截图”或再按截图快捷键抓取当前屏幕 | 私有桌面切换、重新抓屏、旧片段仍可用 |
| 缩略图抽屉 | 点击“查看片段”，显示编号、缩略图与原始尺寸；原图像素保留 | 原生列表与实际窗口截图 |
| 上移、下移、删除、Ctrl+Z 撤销 | 原顺序与撤销栈适配到 C++；删除最后一项后仍可撤销 | C++ 行为测试、私有桌面真实控件 |
| 拖动已选区域重新取图 | 当前画面中的片段可拖动；重新裁取像素并记录可撤销操作。其他画面的片段只在抽屉显示 | 原始像素、位置、跨屏身份与移动撤销测试 |
| 完成后统一识别 | “识别并编辑”打开共享结果窗；“识别并复制”直接复制。按列表顺序逐个识别，以换行分隔；跳过无文字片段 | Host 协议、真实图像映射、失败保留与重试 |
| 结果直接改字 | OCR/翻译结果可“编辑识别文字”，支持保存与取消。复制、朗读和后续提问使用修正原文；翻译结果重译。输入框内 Enter 换行 | 真实 MyGo 控件测试、中文多行和空文本后重新编辑 |
| 短时复用 OCR 引擎 | 文字、原位翻译与贴图版面识别共用 Host 已预热进程；保留行框与单字坐标 | 同进程文字/版面交替协议、取消/退出/恢复测试、真实 NativeAOT 模型 |

未接入网页搜索、引擎/模型切换、闲置五分钟卸载及自动压缩工作集；这是本次明确范围。普通截图的标注、复制、保存、贴图、长截图、录制和翻译入口继续保留。结果复制后保留窗口与对话，原位翻译沿用默认关闭、点击才执行的行为。

## 复用方式

原始文件保存在 `third_party/snapfind/`。直接移植 `SelectedRegionItem`、`RegionActionHistoryItem`、调序/删除/撤销和跨会话暂存逻辑；界面交互参考 `ScreenshotWindow`、`MultiSessionBarWindow` 与 `EditWindow`。WPF 窗口不随包携带：原生画布保留 Tools3000 实现，暂存条使用 Windows 原生按钮与列表，结果编辑使用现有 MyGo 文本控件。原源码、改编文件及许可摘要登记于 `third_party/import-notices.json`。

`modules/capture-native/capture_collection.*` 管理有序片段与截图流程；`collection_window.*` 承接暂存条和缩略图抽屉。`apps/result-ui/ocr_edit.go` 承接识别原文编辑。暂存仅属于本次操作，取消/完成后释放，不作为截图历史自动保存。

## OCR 协议与生命周期

继续使用长度前缀 PNG 请求。最高位表示本次请求需要版面坐标，其余位为图片长度；零长度仍表示预热。原文本调用不变，旧独立 `--pin-layout` 调用仍兼容。Host 使用同一序列化队列、取消归属、进程监督和待命策略，不额外保留另一套模型。

原位翻译与贴图通过 Host 获取版面结果；长结果使用既有增量读取，避免单条管道消息过大。连续采集允许空白识别结果，但进程崩溃、取消和识别错误不会变成空白成功。NativeAOT OCR 同时复用既有 `InteractiveWorkScope`，识别期间关闭执行节流，结束后恢复原策略。

## 验证入口

- `build/test-capture-followup.ps1`：OCR 队列、Host 交接与结果窗测试。
- `build/build-capture-native.ps1 -RunTests -BuildProbe`：原生片段像素/顺序/撤销、既有标注与草稿、录制 ABI、NativeAOT OCR。
- `build/test-capture-native-cpp.ps1 -Suite collection`：私有桌面的真实画布与控件；支持中英文和深浅色。
- `build/test-capture-warm-ocr.ps1`：生成 640×180 图片，在真实 NativeAOT 程序中交替调用文字/版面识别，核对 PID、文字、行框和单字坐标。耗时仅代表该生成样例。
- `build/build-desktop-mygo.ps1` 与 `build/build-results.ps1`：设置页/结果窗测试、静态检查与生产构建。
- `build/test-source.ps1`：仓库源码、Host、模块测试及静态检查。

当前任务证据先存 `_temp/snapfind-integration-20261128/`，最终筛选到 `dist/reports/snapfind-capture-20261128/`。实际运行结果以交付报告为准。没有操作真实安装、真实配置或用户窗口；真实多显示器/DPI 切换、长时间连续采集和所有外部 AI 服务不由隔离回归替代。

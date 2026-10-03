# 原生截图共享组件

- `NativeToolbarControl`：原生工具栏、主题、握柄和溢出菜单。
- `NativeMenuTheme` / `NativeMenuColors`：截图菜单、子菜单和截图置顶菜单共用的字体、间距和配色。
- `NativeToolOptions`：当前工具的颜色、线宽、字号、马赛克像素大小及模糊强度，使用原生菜单和数值控件。
- `NativeToolbarCustomization`：工具勾选草稿、保存与恢复默认；Host 确认后才应用。
- `CaptureCompletion`：输出成功后关闭，失败保留现场，重复请求不重复输出。
- `ToolbarPlacement`：工具栏定位与显示器边界约束。
- `NativeInlineText`：截图内文字输入与导出排版。
- `NativeCaptureSession`：同一窗口中的选区、工具操作、文字编辑与确认取消；观测订阅可选。
- `NativeCaptureWorkflow`：采集与配置快照、窗口布局校验、编辑/直接保存、取消和输出。
- `NativeHostConfiguration`：校验 Host 身份，通过统一配置事务保存工具栏偏好。
- `NativeCaptureDocumentCodec` / `Engine/NativeCaptureDocument`：受限、可编辑的图层文档与恢复；不是引擎对象序列化。
- `NativeCaptureDraftStore`：不可变原图、原子文档更新、修订及图像完整性校验；自动保存协调器尚未接入。
- `NativeDraftRetirement`：已落盘草稿的持久退役、清理重试和新草稿保护。
- `NativeWindowRegions`：整窗与内部区域候选，绘制期间暂停查询。
- `NativeTextPlacement`：文字输入的坐标与选区边界。
- `Engine/`：原生画布适配，由 `ShareXHost.targets` 编译进截图引擎。
- `Assets/Icons`：本地图标及许可证，由 `build/generate-capture-icons.mjs` 生成。

原型和回归测试引用截图模块中的同一份实现。真实屏幕采集及默认入口仍在整合，目录落位不表示原生截图已经交付。

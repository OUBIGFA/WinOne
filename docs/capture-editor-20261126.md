# 截图工具栏与序号输入修复

主工具栏原本按选区左边缘定位，属性栏又按主工具栏左边缘定位。现沿用 Tools3000 的避让、拖动和窄屏换行，仅将默认水平锚点改为画面中心、属性按钮中心。视频/GIF 准备、滚动准备及滚动控制栏采用相同规则；录制中控制栏沿用社区已有的选区居中行为。

序号说明原来使用独立 EDIT，自行绘制提示，鼠标指针仍会交给截图父窗口。新增回归在原程序中复现 Ctrl+A 无法全选替换。现在替换为 Windows RichEdit，窗口隐藏创建，在字体、背景和位置准备好后显示。原生控件负责文本、选区、光标、输入法和撤销；社区画布只画气泡，确认后再画说明文字。编辑状态与实际尺寸纳入社区前景合成缓存，防止确认后留下无字气泡。输入周围默认保留水平 8 px、垂直 6 px 的气泡内边距，并随字号增加。

## 来源与接入

| 社区能力 | 实际接入 | 验证 |
| --- | --- | --- |
| [Win32++ 10.2 CRichEdit](https://github.com/DavidNash2024/Win32xx/blob/395bc45bc691d1706c640b7cbac39dadcabcbdcc/include/wxx_richedit.h) 的控件生命周期、取文、选择、格式及尺寸通知 | `third_party/win32xx/richedit.h` 保留对应消息封装，将 CWnd/CString 改为 HWND/std::wstring；`capture_number.cpp` 调用 | 原生全选、替换、撤销/重做、多行、换行、光标及实际控件绘制 |
| Tools3000 v1.4.11 的按钮、布局、拖动、避让与序号绘制 | `build/prepare-capture-native.mjs` 生成适配副本，来源不修改 | 原生布局矩阵与私有桌面截图/录制/滚动 |
| Tools3000 的说明绘制与撤销 | 保留 Number/Text handler，修正编辑状态变化时的缓存失效 | 编辑、确认、删除重排、撤销与草稿恢复 |

Win32++ 为 MIT，锁定提交 `395bc45bc691d1706c640b7cbac39dadcabcbdcc`。仅适配上述 CRichEdit 部分，不声称接入整个 Win32++ 应用框架。系统提供 `Msftedit.dll`，安装包不增加第三方输入法或编辑器运行时。来源与许可摘要见 `third_party/import-notices.json`，许可原文随安装包提供。

验证结果和最终安装器身份在 `docs/development.md` 记录。真实中文输入法候选窗、多显示器硬件和真实屏幕录制与模拟输入的私有桌面测试分开报告。

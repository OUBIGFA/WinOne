# Tools3000 原生截图来源

- 上游：https://github.com/yuan278501381/Tools3000
- 固定版本：v1.4.11，提交 `f0e968ba937e7e3df9f3f5a8d627be8b19e398dd`。
- 原始归档和逐文件摘要：`source.json`。校验后的本机源码放在 `dist/resources/sources/Tools3000-v1.4.11/`；不修改来源副本。
- 原始 GPLv3 和 Windows/WebView2 链接例外保留在本目录。第三方运行库按最终使用的依赖另行列入许可清单。
- `modules/capture-native/CMakeLists.txt` 直接编译上游真实截图、OCR 与依赖 Core 源码；WinOne 适配只处理宿主协议、配置、执行结果和生命周期。

默认构建与固定安装器已接入新原生目标。功能映射、真实窗口测试及未覆盖范围见 `docs/native-capture-20261125.md`。

通过 `build/prepare-capture-native-resources.ps1` 校验来源；首次缺少依赖时显式传入 `-BuildDependencies`，按上游 vcpkg baseline 编译。`build/build-capture-native.ps1` 在独立工作目录生成适配副本，直接编译社区引擎，再加入既有 OCR 算法的 NativeAOT 程序和原 Rust 录制引擎。编译工具、缓存和源归档不随产品分发。

适配源在 `build/prepare-capture-native.mjs`。除 Host、配置和输出边界外，还保留 WinOne 已有的序号说明、按键语义、长图取消/重拍、文件选择窗口退出保护，以及工具栏图标和布局。每处替换验证固定源码锚点，来源副本逐文件校验。许可原文和实际动态依赖见 `third_party/capture-native-notices.json`。

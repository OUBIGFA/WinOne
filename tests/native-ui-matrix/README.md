# 原生组合回归（显式系统测试）

入口：`build/test-native-ui-matrix.ps1`。普通 `build/test-native-ui.ps1` 不启动该实窗/真实 Host 流程。

```powershell
# 静音视频由现有编码器生成；不安装或修改系统编解码器。
./build/test-native-workspace.ps1 `
  -OutputDirectory _temp/<任务>/workspace `
  -PDFWorker <已验证的 WinOne.PDF.exe> -FFmpegExe <ffmpeg.exe>

./build/test-native-ui-matrix.ps1 `
  -OutputDirectory _temp/<任务>/matrix `
  -BundleDirectory <已有隔离完整运行资源目录> `
  -ChatDirectory <本轮 build/build-chat.ps1 的输出> `
  -VideoFixture _temp/<任务>/workspace/silent.mp4
```

输出必须在仓库 `_temp/` 且被忽略；Unix socket 的绝对路径须短于108字节。每次使用新输出，保留旧失败证据。

## 流程与边界

- 在专用 Windows 私有桌面启动测试窗口和真实 Host；所有系统模块关闭，profile、HOME、缓存和输出独立。不切换用户桌面，不用物理 SendInput，不连接真实账号，不修改自启或真实安装。
- 测试入口 `apps/desktop-mygo/tests/native-ui-matrix/main.go` 在desktop module编译，正常模式直接调用生产 `app.Main`；不复制UI，也不让根Host模块因测试发现而获得MyGo依赖。夹具模式以**同一个隔离 WinOne.exe 路径**请求真实 Host，保留生产的精确进程身份检查，不增加权限绕过。
- 实窗阶段使用 MyGo 官方自动化协议操作真实原生控制：默认回复目标偏好、群聊删除决定、脚本真实执行/暂停/恢复、模板列表、删除/恢复。
- 窗口退出后以同一个隔离可执行路径运行正式 Go 原生组件组合测试，连接仍运行的真实 Host：完整本地 OneBot 设置读取、资料/记忆/技能/语音保存；文件保存冲突、离开保护、真实 Git 索引、PDFium、原生媒体；模板与项目精确预览批准。
- 浏览器阶段启动真实 MyGo Window/WebView2，使用原版 Cindy guest 识别真实 DOM 文本选择，检查原生批注、真实截图、Host 附件/草稿持久化及 marker commit。不是模拟 guest 回执或源码匹配。
- Windows 原生右键菜单不由 MyGo socket 承载；非输入桌面的完整菜单点击路径另列未覆盖。真实模型推理、账号平台、物理麦克风/扬声器、输入法及挂起不由本入口证明。
- 每个组件独立执行，失败不能跳过其后独立项；最终任一失败都返回非零。`result.json` 仅记录实窗阶段，不代表整个入口成功；必须同时核对各 `*-tests.log` 和统一命令退出码。
- 只对路径确认属于本测试 bundle 的进程请求退出/必要清理。私有桌面可能没有 Explorer 托盘宿主；不能把强制清理说成已验证正常托盘退出。

产物带测试端点、客户端及测试二进制，**不用于正式安装或分发**。正式安装器和字体许可矩阵不在此入口重复执行。

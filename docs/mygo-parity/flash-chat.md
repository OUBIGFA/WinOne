# 闪聊完整功能对照

更新：2026-11-11。当前入口使用 MyGo 原生界面，隐藏浏览器只保留通信、身份、加密文件与媒体引擎。用户明确确认这条路线，并要求参照高质量界面重做布局。源码、原始测试及 SDK 保留在 `modules/flashchat/`，原生界面在 `apps/desktop-mygo/pages/flashchat/native_*.go`。设计依据见 [本轮说明](../flashchat-native-ui-20261110.md)。

## 原生入口对照

| 能力 | 当前原生入口与实现 |
| --- | --- |
| 随机标识、随机词组、公开/私密房间、邀请粘贴 | `native_layout.go` 大厅；`NativeBridge.roomAddress` |
| 群聊、Markdown、代码复制、输入状态、草稿 | `native_view.go`、`native_layout.go` 消息与组合输入区 |
| 私聊、未读、身份、公钥、历史 | 会话侧栏、成员菜单；每个在线成员均挂载私聊接收端 |
| 摄像头、麦克风、屏幕、按住说话、设备和音量 | 页头操作、设备对话框、输入区；`NativeMediaBridge` |
| 多文件、加密下载、进度、撤销、拖入文件 | 宿主选择器、`widgets.DropArea`、`shareNative` 与原传输引擎 |
| 行内图片、音频、视频、放大及播放控制 | `native_preview.go`；只传递媒体像素与播放状态 |
| YouTube | 用户确认改为原生按钮，在默认浏览器播放 |
| 昵称、通知、声音、输入状态偏好 | 原生设置的个人信息与消息提醒分组 |
| 网络配置、增强连接、连接状态 | 原生网络设置；保留原发现和中继实现 |
| 资料导入/导出、密钥、重置确认 | 原生资料设置与宿主文件选择器 |
| 社区、二维码、复制、全屏、使用说明 | 大厅、邀请对话框、更多操作 |
| 嵌入与 SDK | 填写自己的 HTTPS 地址后生成代码；私密房间仅生成携带密钥的 iframe |
| 语言、主题、宽窄窗口 | WinOne i18n、共享 kit、原生响应式布局 |
| 切页保持、关闭释放、权限与本地资源 | `completePage`、随机令牌与精确来源验证 |

## 2026-11-11 原生界面验证

- `native-access-final.log`：原生大厅、输入、加入、群聊、摄像头/麦克风/屏幕开关、邀请/二维码/复制、设置与退出通过真实控件操作。后台文档无可读网页内容，产品控件为 MyGo。
- `flows-release-final.log`：三端群聊与公开历史、私聊首条接收/隔离/重开、私密房间及其中的私聊、密码与历史边界、文件授予/加密下载/内容/撤销、下载同时发送、图片和音视频文件、对端实时媒体、资料往返、切页保持与关闭释放通过。
- 默认测试覆盖原生按住说话释放、外部视频按钮地址、桥接令牌/来源/文件权限、语言与布局；前端 376 项测试通过。实际组件图覆盖中英、明暗、520/1000 DIP、长消息、大厅/设置/邀请/帮助。
- SDK 构建通过；原生生成嵌入代码使用用户填写的 HTTPS 根地址。真实第三方部署、物理输入法/设备/系统声音、文件管理器拖放和通知到达未在本轮实测。
- 公共强制中继两端实际双向消息成立；三端完整测试在 A/C 未连接时超时，保留 `flows-relay-surface.log`，不标记整个公共中继流程通过。

以上证据在 `dist/reports/flashchat-native-ui-20261110/`；资源测量、设计依据、构建与安装结果见 [本轮说明](../flashchat-native-ui-20261110.md)。下面是旧网页入口的历史证据。

## 前一轮网页界面验证（历史）

以下记录属于 2026-11-10 的网页界面接入，用于核对保留能力。不能代替本轮原生控件、布局或媒体呈现的验证，也不将旧 Go 移植的本机测试算作当前跨网证据。

| 上游能力 | WinOne 入口与实现 | 本轮验证 |
| --- | --- | --- |
| 随机房间号、随机词组、公开/私密房间、邀请链接 | `pages/Home`、`config/invitation.ts` | 公开/私密房间与邀请链接已实测；密码不同不能收消息 |
| 群聊、输入状态、Markdown、代码块及复制、消息/媒体显示 | `components/Room`、`core/chat` | 双向群聊及上游单元测试通过；修复组合输入确认回车误发送，含 WebKit 229 标记与多行输入回归。物理输入法未人工实测 |
| 成员私聊、关闭后保留私聊记录、身份校验和公钥 | `components/Shell/PeerListItem`、`core/chat/ChatRoomRegistry` | 三端浏览器私聊隔离、键盘进入、关闭重开保留记录通过；核心单元测试通过 |
| 公开房间历史补发；私密房间不补发 | 原版 ChatRoom 与历史动作 | 第三端晚加入收到公开历史；私密隔离已实测 |
| 摄像头、设备选择、视频布局 | `useRoomVideo`、`useLocalMedia` | 独立浏览器收到视频、停止后消失；失败换设备保留原采集，单元测试通过 |
| 麦克风、设备选择、按键通话、对端音量/静音 | `useRoomAudio`、`RoomAudioControls`、成员列表 | 接收端音频字节增长；停止、拒绝权限、迟到采集回收单元测试通过；物理设备待验 |
| 屏幕共享及共享声音 | `useRoomScreenShare` | 独立浏览器收到画面、停止后消失；原生 WebView2 采集及停止通过。使用合成画面，不代表物理屏幕选择与系统声音已验 |
| 多文件分享、加密传输、撤销、接收进度 | `services/FileTransfer`、`core/chat/fileOffers` | 独立浏览器下载内容逐字节一致；撤销后入口消失；原生 StreamSaver 下载通过 |
| 行内图片/视频、文件拖放 | 原版 Room 文件事件及 `RoomFileUploadControls` | 源码保留；真实资源管理器拖放待验 |
| 昵称、声音/通知、输入状态偏好 | `pages/Settings` 与原版 SettingsManager | 原始测试通过，页面保留全部设置；真实系统通知待验 |
| 资料导入/导出、密钥、设置迁移 | 原版 Bootstrap、序列化与存储适配器 | 真实文件导出→新浏览器上下文导入→刷新→再导出，逐字段一致。修复导入昵称被旧值覆盖；取消删除保留身份 |
| 社区房间、二维码、复制、全屏、关于与免责声明 | 原版 Home/Shell 页面 | 二维码开关、全屏进入退出、复制及中英、深浅、560/1000 宽检查通过 |
| 嵌入代码与 SDK | `pages/Home/EmbedCodeDialog`、`sdk/sdk.ts` | SDK 编译打包；邀请和嵌入使用公开站点地址，不泄露本机地址 |
| 公共发现、STUN/TURN、增强连接、连接状态 | 原版 Trystero、`config/network.ts`、`publicRelay.ts` | 电脑当前代理/TUN保持不变，iPhone Safari 使用移动数据和同版页面，双方真实收到文字。官网页面同条件未通过；用户明确本次交付客户端互通、不发布网页 |
| 自定义 tracker / RTC 配置 | `pages/Settings/NetworkSettings` | 保留旧 WinOne 能力；聊天与文件共用配置；无效配置和保存测试通过 |
| WinOne 中英文、主题、窗口缩放 | `config/host.ts`、`config/i18n.ts`、`useShellTheme` | 浏览器四组组合与真实 WebView2 主题语言同步、窄窗无横溢通过 |
| 切页保持，关闭释放 | `completePage.SetAside/Close`、`server.go` | 真实 WebView2 保持同一个 Session；关闭释放 WebView 和本地端口 |
| 本地资源、权限、下载和外链 | `complete.go`、`server.go` | 精确 origin 权限、DNS 重绑定 Host 检查、下载路径及内容验证通过 |

## 可复现入口

- 前端：使用本地 Node 运行 `build/build-flashchat.mjs --test`，包括许可、类型、单元测试、页面及 SDK 构建。
- 浏览器：`tests/flashchat-complete/run.cjs <根目录 _temp 内输出>`。默认本机 tracker；`--public-trackers` 使用真实公开服务。`WINONE_TEST_RTC_CONFIG` 可显式要求 relay，仅用于选定的联网验收。
- 原生宿主：`build/test-flashchat-native-windows.ps1 -OutputDirectory <根目录 _temp 内输出>`。仅在私有桌面运行，不操作真实安装与配置。
- 原生完整流程：同一脚本加 `-TestName TestNativeFlashChatFlows`，启动三个独立资料目录的窗口，核对群聊、私聊、历史、文件、媒体、资料与生命周期；加 `-PublicRelay` 使用公共发现并强制中继路径。
- 手机跨网：同一脚本加 `-LiveRoom <随机测试房间>`，记录收到手机消息、实际选中候选和收发字节。必须双方确认后才算成功。

## 2026-11-10 跨网与构建证据

- `complete-browser-local-11/result.json`：22 项完整浏览器行为通过，无页面错误。使用真实本机 WebTorrent 配对服务，媒体为合成设备。
- `complete-browser-final-2/result.json`：23 项通过，新增实际粘贴邀请加入与公开房间刷新重连。`relay-browser-final/result.json`：公开发现 + 强制公共中转 24 项全部通过；消息选中路径两端均为 relay 且收发字节增长，包含媒体、文件和私密房间。
- `native-final/tests.log`：真实 WebView2 权限、双轨采集、屏幕采集、下载、主题语言、切页保持与关闭释放通过。
- `native-pair/tests.log`、`native-pair/profile/native-pair-paths.json`：两个独立资料目录的原生窗口经公共中转双向收发消息，14.50 秒通过；两端路径均为 relay，收发字节均大于零。这是同机双客户端验证，不冒充两台实体 Windows 异网测试。
- `live-native-8/profile/mobile-reply-observed.txt`：电脑收到手机发送的“手机已经连接”。用户另行确认手机看到电脑消息。`mobile-safari-connected.json` 与电脑追踪均显示连接成立并打开数据通道。
- 手机使用临时 HTTPS 地址访问同一份本地构建，未注册账号或改变网络。公开页面经单独授权开放，完成后已关闭。此证据不证明原官网邀请已修好，也不证明公共服务在所有网络永久可用。
- 该次在线夹具原本只匹配“手机已连接”，因此没有自动结束；按实际消息记录人工判定双向成功，随后关闭隔离进程。保留未自动通过的原始日志，没有伪造自动成功；夹具已接受两种表达。
- `build-compile-final.log`、`bundle-runtime-verification.log`：统一完整包编译、来源许可、运行时与包清单检查通过。`installer-ui-probe.log`：隔离桌面安装启动握手通过，维护标记与用户输入桌面保持不变。
- 根 Go 全测首次出现日志锁、端口复用和短超时失败；失败的 Host、AI、启动探针与进程输入包逐组复验通过，`root-go-vet.log` 静态检查通过。保留初次失败记录，不降低断言。
- `live-native-8/resources.json`：包含 WebView2 的诊断进程组在已连接时采样 15 秒，私有工作集峰值约 212.7 MiB，平均 CPU 约 0.60%。诊断保留了连接追踪对象，此值不是无诊断正式程序的性能结论。

用户确认本次不发布配套网页。另一台 WinOne 可粘贴房间链接或输入同一房间标识，使用本地同版通信实现；原官网是否能直接加入仍取决于其自身配置，不将该结果列为已验收。原版二维码、链接与 SDK 能力保留。

隔离首装、覆盖、两处失败回退、卸载和配置/额外文件保留已通过。包内文件、安装辅助程序和最终安装/卸载器的安全扫描通过，产物与包清单的摘要关联校验通过。交付证据在 `dist/reports/flashchat-complete-20261110/`，不入库；正式安装器为 `dist/release/WinOne-setup.exe`。详见 [三轮核查](../flashchat-full-integration-20261110.md)。

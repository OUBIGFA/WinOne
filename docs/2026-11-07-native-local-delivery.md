# 2026-11-07 本地原生迁移与追加整改验收

## 交付范围

Chat、自动化、OneBot（含 Bot 模式）与本地工作面板使用 MyGo 原生 UI，不再由 React 承载。插件自有网页、真正浏览器内容、MCP App 与邮件 HTML 继续使用隔离 WebView；权限、密码、确认、导航与普通表单使用原生控件。

- MyGo **v0.8.1**，原始提交 `8f07b6f282873fd526a4f8153d727063b00dcdeb`；Pi **1.5.0**。
- Cindy 固定 `2d8d3c3605ee9073613dcd4069bc59f799be61ae`。不宣称跟进了该提交之后的 main。
- 完整本地能力对照：[本地插件平台](mygo-parity/local-plugins.md)。18 个原生追踪模块、9 份同步规格的固定基线 strict 检查通过；两个旧功能 pending 已依据实际行为证据核销，未移动上游基准以掩盖差异。
- 排除范围仍仅为用户已指定的 Cindy 云服务、远程电脑、Chat Server、非 Pi 引擎。直连 OAuth、本机浏览器扩展、邮件、MCP Apps、Library 与文件 handoff 没有被删减。

## 三项追加整改

| 用户要求 | 实际变化与验证 |
| --- | --- |
| 设置保存卡顿，优先程序管理 | 正常事务只投影变化模块；同一 ConfigurationJob 支持有界等待真实完成，不重发保存。100 程序/7 份 fsync 投影平均 **69.948→16.147ms**；21 次真实 Host 保存/回读平均 **158.434→39.453ms**，P95 **163.859→42.472ms**。失败补偿、启动/恢复完整投影、旧客户端与未保存保护保留。 |
| 闪聊首页首次显示/返回首帧居中 | 去掉上一帧 Bounds 高度依赖；有限视口中自然居中。中英、1000/560 宽首次和返回第一帧的生产控件坐标回归红→绿，不用延时、动画或隐藏第一帧。 |
| 提示/警告可关闭 | kit Notice/InlineError/Status/Toast、结果窗与保留网页 Notice/rich status 增加关闭。内容身份记忆不随兄弟控件行号变化；中英键盘验证，关闭不清草稿、不解除权限或验证限制、不绕过未保存确认。 |

保存数字不是物理点击到屏幕呈现或真实系统自启测量；首帧坐标回归不冒称物理显示器逐帧采样。可复用测试留在源码及 `tests/`，临时数据留在 `_temp/`。

## 行为与安全证据

证据根：`_temp/plugin-task-close/resume-20261106/`。旧的已通过且未受影响证据保留，新改动按影响范围复验：

- 专用真实 SDK 系统入口 **139 PASS / 0 FAIL，无 skips**；源码同步 339 文件。普通构建的系统 skips 不计入该数字。
- 原邮件 provider **10/0**、原生邮件三组合，覆盖 HTML/quote/原 serializer/净化、CID/附件、autosave/三方 merge、10 秒准备/撤销/立即发送、晚回执及 unknown 不重发、失效只读/Reader/关闭释放。
- 本机浏览器桥 **4/0**，实际 protocol-v3 配对、签名 relay、首次 send/wake 等待 SDK ready、已交付/unknown 不重投；登记命令只作用于隔离 registry fixture。
- 富内容 **216 个原生窗口案例**，26 类组件及图表，中英/深浅/宽窄、真实绑定动作、实际图片/公式/diagram；没有把 alt 或源码当成绘制完成。
- 当前 v0.8.1 原生 MCP 官方 App 全链、Frame2/Frame7 子孙 iframe 最终 OS 导航闸、旧 View 403、typed COM ABI/checkptr=2、普通/生产 vet 通过。App provenance 不提升为新主人授权，实际 Host/Pi 每代准入、每工具 block 与 private PeerPID 验证通过。
- 最终页面新增实窗验收包括四边/独窗、单/多实例、位置与尺寸重开/停用恢复、固定/自适应高度、真实焦点/OS 鼠标与 F8、3 秒过期、外链失焦拒绝及原生取消。
- 媒体实际 ledger URI→原生预览→用户确认→真实附件导入→保留原文字/已有附件的持久草稿→ACK 关闭；没有自动发送。HTTP PNG 已走 Host 原导入/只读 preview，Chat/OneBot Markdown/rich 与大于旧 64MiB 的合法 PNG 实际解码；工作区 lightbox 同步保持原 **100MiB** 预算，超过预算明确失败，关闭释放读取上下文。
- 自动化真实绑定 Pi 终态、原通知失败历史、删除排除手工 owner、恢复为 paused 并保留历史/错误/通知/绑定、窄窗默认选中任务打开详情、保存/继续编辑/放弃保护通过。原模板/项目精确预览批准的未受影响真实证据保留。

正式测试入口：`build/test-native-ghost-checks.ps1 -RunSystemChecks`、`build/test-native-ui.ps1`、`build/test-native-ui-matrix.ps1`、`build/test-onebot-host.ps1`、`build/test-configuration-save.ps1`。系统交互始终显式选择。前台组每次有不超过30秒的 wall-clock watchdog，结束已核对回到原输入桌面。

## 当前构建、退出与安装

- 完整统一 `build/build.ps1 -SkipInstall` 通过；随后最终 desktop-mygo 全包测试、普通/生产 vet、生产 EXE/许可与 `verify-built-bundle.ps1 -RunStartupProbes` 通过。
- 并行全包曾出现 PDF/permission 退出超时，未忽略失败或放宽退出预算；两包与最终全部原生包以 `GOFLAGS=-p=2` 有界编译/测试并发复验通过，正式断言不变。
- 原生生产窗口关闭的7类场景通过，覆盖旧常驻字段、草稿、忙任务、慢释放等；设置窗口关闭不代表 Host/整个 WinOne 退出。
- 最终候选隔离安装矩阵通过：5863 文件逐项哈希、回滚/中断拒启/修复/覆盖/历史恢复/残缺自包含修复/卸载恢复、专属登记/快捷方式/配置及额外文件保留、匹配导出卸载器。没有安装到用户真实目录。
- 当前载荷、帮助程序、测试/正式候选安装器和卸载器 Defender 非修复自定义扫描明确未检出；实时/行为保护保持开启。同一已验收 ZIP 的正式候选已编译、原产物契约校验通过；固定文件/摘要/元数据最后原子更新。
- Git 提交/推送及正式固定产物以 `dist/release/release-info.json` 的最终记录为准；本文件不把候选编译等同已交付。

## 资源测量与明确未达标项

完整进程组测量（当前候选、i7-12700K/20逻辑核/约64GiB/Windows build26220）：

| 场景 | 完整样本/CPU区间 | 私有工作集峰值 MiB | Private Bytes 峰值 MiB | 平均 CPU |
| --- | --- | ---: | ---: | ---: |
| 禁用全部模块，Host 空闲 | 59/58 | 10.832 | 55.566 | 0% |
| 全部模块启用、无任务（含 Capture/OCR Python/Stroke 辅助进程） | 54/53 | 134.410 | 208.020 | 0.008% |
| 原生设置窗口打开、模块禁用（Host+UI） | 26/25 | 48.684 | 182.648 | 0.025% |

窗口关闭后观察的 UI 进程均退出，资源场景回到 Host-only；没有通过清空工作集、隐藏常驻或省掉 Worker 降低数字。它们是短时空闲/窗口测量，不证明长期/模型运行峰值。

**启动300ms目标仍未达标。** 21 次真实新进程启动/正常关闭，20 次同配置重开 P95 **805.1264ms**。分段定位到首次 GPU 呈现；无输入桌面诊断首次 present 为366.9ms，真实前台两次诊断明显更快，但后者不是完整 P95，不混入正式样本。

用户于 **2026-11-07** 明确选择本次优先交付已验收功能、如实保留启动未达标记录并后续优化。300ms目标保留，不再作为本次正式安装器的硬交付门槛；其他功能、权限/草稿、来源/许可、安装与安全检查没有被放宽。复现入口 `build/benchmark-native-startup.ps1`（默认私有桌面不切前台）；原始21次与分段证据分别为 `startup-final-081-3/measurements/`、`startup-frame-stats-081/`、`startup-foreground-diagnostic-081/`。

## 未自动验证的外部条件

真实邮箱/市场/IM/OAuth账号、生产浏览器 NativeMessaging 登记、物理麦克风/扬声器/IME、系统挂起与用户实际安装没有被安全夹具冒称通过。未修改用户真实配置、安装、账号、模型、自启、浏览器登记、Explorer或安全设置；没有签名、发布 tag 或对外发布授权。

# AI服务流式输出与思考显示

2026-10-06。当前完整包为 `_temp/repository-hygiene-20261008-9513b969/archived-dist/results-ai-20261006-r5`，已更新 `_temp/repository-hygiene-20261008-9513b969/archived-dist/trial-0.0.1`。3295 文件、配置及管理员授权校验通过。安装位置 OCR、翻译及关闭思考三条完整流程通过；思考只在后续对话显示。新增 Markdown、图表及资源边界见 [结果内容与图表](结果内容与图表.md)。

## 当前链路

1. `modules/ai-magpie/main.go` 对流式生成请求启用 magpie SSE；`stream.go` 将正文、思考和完成事件转换为逐行 JSON。错误正文不透传，缺少完成标记、异常动作和超量输出均报错。
2. `internal/ai/stream_windows.go` 从受控子进程管道逐条接收，并分别传递正文与思考片段。停止沿用 Windows Job Object 终止请求及其子进程。账号管理与原有非流式调用保留原接口。
3. `apps/host/ai_windows.go` 持有请求缓冲区，按 UTF-8 字节偏移返回新片段，每类单次最多 32 KiB；大结果不会反复传送全文，也不会在最后一段读取前报告完成。
4. `apps/result-ui/client_windows.go` 仅在任务活动时每 100 ms 查询新增内容，Controller 使用请求代次拒绝迟到片段。Panel 直接显示已收到的内容，不增加模拟打字延迟。

更新前四层均等待完整结果；此次贯通整条链路，不只是修改供应商的 `stream` 字段。正文与思考总量有 1 MiB 上限，帧大小和标准输出另有边界。模型返回的思考不写入诊断日志，也不混入正式回答或后续消息历史。

## 思考显示

- AI服务提供“显示思考过程”开关，默认开启。旧配置缺少该字段时仍开启；关闭后保存和重读保持关闭，旧调用方省略字段不覆盖已保存选择。
- OCR 与译文结果不显示思考。后续对话中每条“思考过程”默认折叠；用户可点击展开、收起，也可在固定输入区收起全部展开思考。流式片段和完成事件不改变用户已选择的展开状态。
- 全局关闭时不显示入口与内容，不修改模型推理方式。仅展示供应商接口实际返回的思考文本或摘要，不编造未提供的内部过程。
- 正式回答始终独立显示；停止和失败保留已收到的草稿并标明未完成。重新生成失败保留此前完整答案。

## 已核对的成熟实现

- [magpie chat encoder](https://github.com/yetone/magpie/blob/8c71cd27f95432362b91fe140bbffacc550def8b/internal/gateway/chat.go)：将 `KText` 转为 `delta.content`，区分错误事件，结束输出 `finish_reason` 和 `[DONE]`。同提交 `sse.go` 在写出事件后 Flush。适合继续由 magpie 统一多供应商协议。
- [LobeHub fetchSSE](https://github.com/lobehub/lobehub/blob/d0ab946b00ac4560b1682e3b8fef8ff09de0c05b/packages/fetch-sse/src/fetchSSE.ts)：区分文字、结束原因、推理、用量等事件；接口分别提供 onAbort、onFinish、onErrorHandle 与 onMessageHandle。文本增量与 UI 平滑动画是不同层，WinOne 不必引入打字动画。
- [LobeHub server stream sink](https://github.com/lobehub/lobehub/blob/d0ab946b00ac4560b1682e3b8fef8ff09de0c05b/apps/server/src/modules/AgentRuntime/adapters/serverCallLlmStreamSink.ts)：累积增量并合并推送，避免每个 token 都产生完整快照传输；终态须完成末次刷新。其 Redis/Gateway 与 300 ms 批次属于 Web 服务实现，不直接照搬到本地桌面进程。
- [LobeHub StreamSink 契约](https://github.com/lobehub/lobehub/blob/d0ab946b00ac4560b1682e3b8fef8ff09de0c05b/packages/agent-runtime/src/transport/stream.ts)：文本块、运行事件和错误分开，使用任务标识关联。仅参考设计；该提交使用 LobeHub Community License，不复制其代码。

LobeHub 调查提交来自 canary 分支，不能称为稳定发布版本。以上证明源码提供机制，不替代 WinOne 的端到端验证。

## 共享界面与验收

统一生成事件契约由 AI服务提供：请求标识、供应商/模型/图标快照、字节偏移、文字增量及终态。结果 UI 只维护一个进行中的回答，合并文字时保留先后顺序；取消或重试后，旧请求不能写回新回答。

- LLM 翻译、AI OCR 与后续对话走同一流式链路。截图与划词来源复用同一结果组件，不另建流式 UI。
- 本地 OCR、当前 Google 翻译等一次性来源保留其真实完成方式，不将完整结果拆字伪装为网络流式。
- 停止保留已收到文本并标为停止；中断或失败不得当作完整答案。重新生成失败时保留原有完整结果，并明确提示状态。
- 界面显示首段后仍可拖动、缩放、选择和复制已收到内容；用户向上滚动后不被新片段强制拉回底部。
- 无需常驻网关。沿用现有按请求启动的 magpie 子进程、账号隔离、进程组取消和会话鉴权。
- 测试必须由分段服务在收到界面确认前暂停最后一段，以证明文本在请求结束前已可见。另测 UTF-8 分段、异常中断、停止、旧片段拒绝、重试、输出上限、两供应商和最后窗口退出。

当前不包含网络自动续传和工具调用。首段可见、完整耗时与进程组资源增量分开测量；成功状态或最终文本不能替代流式验收。

结果布局参考上述 LobeHub 提交的 `DESIGN.md`：居中阅读列、克制的元数据、正文与输入区分离。复用 mygo `SplitVertical` 和 `TextArea`，用户拖动分隔线可增高输入框，Enter 发送、Shift+Enter 换行，输入法组合期间不发送。模型入口与发送/停止位于输入区底部。

供应商图标使用 [LobeHub Icons](https://github.com/lobehub/lobe-icons/tree/82e641b4fece9d1028a127149af9ded00df5ac0c) 与锁定 magpie 的 29 个本地资产。浅色优先官方彩色版本，深色统一单色轮廓；原本只有黑白版本的官方标识不另造品牌颜色。图标根据供应商地址或登录账号识别，不根据所选模型误判服务商；未知自定义服务用现有 Lucide Server 图标。逐文件哈希及来源见 `third_party/provider-icons.json`，两种 UI 共用同一资产目录。

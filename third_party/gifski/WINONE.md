# WinOne gifski 适配

上游：ImageOptim/gifski 1.34.0，提交 `1060eab4500a20f27e2fa3ab7e85473d0e921cbd`；来源为 Cargo 锁定发行包，保留完整库/CLI 源码与原 Cargo 清单。原始 LICENSE 由该提交补齐，未改写许可。WinOne 只静态链接库，禁用默认 CLI、PNG、FFmpeg 特性。

局部修改仅在 `src/collector.rs` 与 `src/lib.rs`：

- 新增可选 `add_frame_rgba_recyclable`：固定尺寸输入在 denoiser 不再读取后归还原 Vec；捕获与 GIF 不再每帧复制一个完整 RGBA 输入。发生编码错误时输入按 Rust 所有权释放，不伪造完成回调。
- 新增 `set_final_timestamp`：Collector 结束前提交真实停止时间，Writer 使用一帧前瞻确定末帧延迟；静止尾部不再按平均帧距猜测，不追加虚构帧。
- 完全不透明输入跳过无意义的 alpha-dither 暂存数组。透明图像保留原算法。`src/encoderust.rs` 的 GIF 量化专用 import 跟随 `gifsicle` 特性，避免库模式的无用导入告警。

没有移除原有公开 API、编码功能、授权文字或默认行为选项。上游量化、去噪与编码仍有自己的有界临时内存；池化不是零内存开销声明。

构建入口 `build/build-recording.ps1`；真实编码/解码测试 `modules/recording/examples/encode_probe.rs`，缓冲归还与末帧回归位于录制模块测试。升级时先重新比较以上改动，再更新锁、来源和全部许可，运行 `build/verify-diagram-notices.ps1 -Runtime recording -Update` 与 `node build/verify-import-notices.mjs --update` 后只读复验。

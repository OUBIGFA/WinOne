# MyGo Windows 文件拖入兼容

上游：`github.com/egoist/mygo v0.8.3`，提交与 Go h1 校验在 `source.json`。
MIT 许可全文：`../licenses/desktop-go-github.com-egoist-mygo-v0.8.3-LICENSE.txt`。

`build/prepare-mygo-filedrop.ps1` 校验锁定模块和许可，把**完整**上游复制到仓库生成缓存 `dist/resources/sources/mygo-winone-v0.8.3/`，再应用 `source.json` 声明的文件拖入、异步公式重排、表格换行与嵌入网页焦点/手势适配。保存并逐文件校验缓存摘要；不手改系统/用户的 Go 模块缓存。构建 `desktop-mygo`、`result-ui` 和 `test-native-ui` 时自动准备，直接 Go 调用前须先执行此入口。

兼容方式直接移植 WinOne 原 Rust `component_file_drop.rs`：只为管理员进程建立 Shell `WM_DROPFILES` 接收窗口，仅放行文件转移消息，外部拖动期间才显示；普通 OLE 路径原样保留。不自建 UI 命中派送：消息立即调用上游 `surface.dropFiles`，沿用 MyGo 已有的剪裁、模态层、嵌套入口与文件事件。不改证书、代理、系统设置、持久化服务或原生输入行为。

升级 MyGo 时重新查上游、核对基线/钩子并运行真实普通/管理员拖放，以及共享结果窗、设置窗口全套。上游正式修复该兼容路径后删除本适配，恢复直接模块依赖。

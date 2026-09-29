# WinTray 模块

基于 WinTray `dea17c108357109d6c2c04e80c9df45279bfdc51`，保留原 MIT 许可及已有测试。

`internal/` 为既有实现；`worker/` 为 WinOne 接入层。原设置界面仅随上游代码保留，不由 Worker 创建。系统任务、命名对象及数据目录使用 WinOne 独立命名。

默认验证 `worker`、`internal/config`、`internal/stringutil`；完整原生测试需在专用环境执行，部分原测试会创建自己的系统任务。不要对用户真实程序自动运行原生测试。

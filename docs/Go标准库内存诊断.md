# Go 标准库静态缓冲与 WinOne 内存记录

2026-10-07，本机 Go 1.26.2、Windows 11。本文解释统一安装验证包的内存超标来源，不表示正式版性能验收通过。

2026-10-08 更新：用户已接受 Host 约 43 MiB，并要求优先完成正式版。当前约 42 MiB 不再因原 40 MiB 目标阻断交付；停止此项非阻断微调。下文保留历史归因与实验边界，生产包继续使用原版标准库。

## 已核实的结果

- Windows `PROCESS_MEMORY_COUNTERS_EX2` 与现有 WMI 计数交叉核对一致。私有工作集是驻留物理内存；Private Bytes 是私有提交量，两者不能互相替代。
- 实际 Host 的 Go 垃圾回收记录显示回收后对象约 1 MiB。`go tool nm -size -sort size` 同时定位到 `crypto/internal/fips140/drbg.memory`，大小恰为 33554432 字节。
- Go 源码将它定义为静态 `entropy.ScratchBuffer`，即 `[1 << 25]byte`。即使 FIPS 关闭，Windows 上仍有约 32 MiB 的提交成本；驻留量会随首次和后续运行变化。
- 原版安装包的 Host 在四模块任务结束后，测得私有工作集峰值 40.83 MiB、Private Bytes 峰值 53.55 MiB。重复运行同一构建时也观察到约 9 MiB 工作集，但提交量仍约 54 MiB。因此，较低的重复运行工作集不能证明这项成本已消除。

## 隔离实验

只将该缓冲改为首次调用 `getEntropy` 时通过 `sync.OnceValue` 分配。未改变缓冲大小、熵算法、TLS 或随机数接口；通过 Go overlay 构建，不修改工具链原文件，也未进入交付包。

| 标准库最小程序 | FIPS | 两轮私有提交量 | 私有工作集观测 |
| --- | --- | --- | --- |
| 原版 Go | 关闭 | 44.10–44.12 MiB | 首轮 33.75 MiB，第二轮 1.49 MiB |
| 延迟分配实验 | 关闭 | 11.68–12.07 MiB | 1.41–1.75 MiB |
| 原版 Go | 开启 | 约 44.11 MiB | 约 33.50 MiB |
| 延迟分配实验 | 开启 | 45.00–45.23 MiB | 约 34.39 MiB |

将实验应用于隔离 Host 后，四模块联动、禁用、恢复与退出测试通过；退出任务后的 Host 私有工作集峰值 9.10 MiB、Private Bytes 峰值 22.16 MiB。FIPS 开启时的标准库 `crypto/rand` 和 `crypto/tls` 测试通过；`crypto/internal/fips140/drbg` 在此版本没有可运行测试，只完成编译。

这些结果支持成本归因，尚不足以交付密码库构建变体。没有验证完整平台矩阵、认证模式、所有加密调用及后续工具链兼容性；不宣称保留任何 FIPS 认证资格。当前安装器继续使用原版标准库。

## 可复现入口

`tests/resource-metrics/measure-crypto-memory.ps1 -GoExe <指定的 Go> -OutputDirectory <新的忽略目录> -RunSystemChecks`

该脚本只构建标准库最小程序，用 FIPS 开关各运行两轮，记录实际编译版本、可执行文件摘要、私有提交量、工作集及正常退出。它不使用构建 overlay，不读取用户配置，也不把一次编译后的首轮称为操作系统冷缓存。原始记录与实验源码保留在 `_temp/repository-hygiene-20261008-9513b969/archived-dist/host-memory-diagnostics-20261007/`。

后续应对正式包的新文件首次运行和重复运行分别测量，同时保留 Private Bytes。优先跟进上游支持的改动；标准库覆盖如要进入产品，需要单独完成维护方案和兼容性评审。其他本地功能、稳定性和安装工作可以继续。

## 来源

- [Go 1.26.2 DRBG 源码](https://github.com/golang/go/blob/go1.26.2/src/crypto/internal/fips140/drbg/rand.go)
- [Windows 同类报告 golang/go#81956](https://github.com/golang/go/issues/81956)：查询时仍开放，未作为已发布修复使用。
- [上游配置提案 golang/go#81639](https://github.com/golang/go/issues/81639)：查询时仍开放，不能把提案参数当作现有工具链选项。
- [Microsoft PROCESS_MEMORY_COUNTERS_EX2](https://learn.microsoft.com/en-us/windows/win32/api/psapi/ns-psapi-process_memory_counters_ex2)

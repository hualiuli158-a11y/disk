# Disk / gin_cpp 性能基线

本报告是指定环境下的实测基线。负载端和 API 共享 Windows 主机，存储节点位于同机 WSL，网络为回环链路。
这些数据用于比较相同条件下的版本变化，不代表独立部署的容量上限。

## 版本与环境

- Disk：`438c7b06ab8414969746240c8aef52aff2e8b9e2`；工作树：有修改，见 provenance.json 和源码哈希。
- gin_cpp：`a1e0491b0b373c3e6cbd786e1f48577b20680826`。
- MouduAsio：`9a8eba2b0749c44774b494e3df7e4801412e0e73`；工作树有修改，以 network-sha256.json 记录的实际网络源码为准。
- 系统：`Windows-11-10.0.26200-SP0`；逻辑处理器：24。
- CPU：12th Gen Intel(R) Core(TM) i7-12800HX；物理内存：15.7 GiB。
- API 构建：Release；存储构建：Release。
- API 编译器：`E:/mingw64/bin/g++.exe`。
- gRPC 依赖：`E:/Libraries/grpc-mingw-debug/lib/cmake/grpc`；本机安装目录的静态库为 Debug 构建。
- 每项 HTTP/API 预热 2 秒，正式负载 10 秒，重复 3 轮。
- gin_cpp 微基准不启用逐请求 Logger；真实 Disk API 保留其现有日志和业务中间件。
- API、MySQL 与存储使用独立测试数据库和对象目录；数据库、缓存和日志不在重复轮次之间重置。

## HTTP 与业务 API

RPS 为成功目标请求数除以实际测试时长。表中 RPS、P50/P95/P99 均取各轮统计值的中位数；RPS 范围列出各轮最小值和最大值。
分位数中位数不等于合并所有请求后的分位数。错误率包含非成功状态码和正文校验失败。

Windows 计时可能将极短请求记录为 0 ms；该值表示计时分辨率下未测得正时长，不能解释为零延迟。

| 场景 | 路由 | 配置 VU | 轮数 | 成功 RPS 中位数（范围） | P50 ms | P95 ms | P99 ms | 失败/请求 | 丢弃迭代 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|

## 文件传输

文件场景包含首次传输与客户端哈希校验，不单独预热。每次上传后立即软删除、查回收站并永久删除；这些清理请求占用容量，未计入上传延迟，但计入吞吐分母。
下载逐次检查长度和完整 SHA-256。文件延迟分位数只统计成功操作；失败尝试及其耗时单独保存在原始 JSON。MiB/s 只计成功传输。

| 场景 | 并发 | 轮数 | 成功操作/s | MiB/s | 成功 P95 ms | 成功 P99 ms | 失败/尝试 | 清理失败 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| upload-duplicate-b1048576-v1 | 1 | 3 | 14.84 | 14.84 | 61.339 | 74.811 | 0/452（0.00%） | 0 |
| upload-duplicate-b1048576-v4 | 4 | 3 | 21.51 | 21.51 | 209.532 | 265.621 | 0/842（0.00%） | 0 |
| upload-unique-b1048576-v1 | 1 | 3 | 11.05 | 11.05 | 109.398 | 185.268 | 0/358（0.00%） | 0 |
| upload-unique-b1048576-v4 | 4 | 3 | 52.08 | 52.08 | 71.618 | 79.287 | 0/1584（0.00%） | 0 |
| download-duplicate-b1048576-v1 | 1 | 3 | 46.96 | 46.96 | 44.869 | 97.151 | 0/1407（0.00%） | 0 |
| download-duplicate-b1048576-v4 | 4 | 3 | 63.98 | 63.98 | 103.994 | 130.877 | 0/1908（0.00%） | 0 |

## 进程资源

每秒采样；CPU 以平均占用核心数表示，1.0 表示占用一个逻辑核心。内存为测量轮次期间观察到的峰值，无法覆盖采样间隔内的瞬时尖峰。
这里只列服务进程；客户端、MySQL 和 Linux 存储节点的原始采样另行保留。

| 场景 | 进程 | 平均 CPU 核 | 峰值工作集 MiB | 峰值私有内存 MiB |
|---|---|---:|---:|---:|
| upload-duplicate-b1048576-v1 | api | 0.326 | 30.84 | 12.45 |
| upload-duplicate-b1048576-v4 | api | 0.859 | 35.52 | 17.82 |
| upload-unique-b1048576-v1 | api | 0.280 | 34.85 | 17.03 |
| upload-unique-b1048576-v4 | api | 1.775 | 37.27 | 19.53 |
| download-duplicate-b1048576-v1 | api | 1.399 | 37.63 | 19.68 |
| download-duplicate-b1048576-v4 | api | 2.063 | 37.49 | 19.57 |

## 正确性与限制

压测后的数据库与物理对象核验：

```json
{
  "quota_mismatches": 0,
  "unreferenced_physical_records": 0,
  "missing_objects": [],
  "orphaned_objects": []
}
```

业务回归执行原分支测试的副本：仅将 HTTP 头读取改为大小写不敏感，并将子进程日志持续写入日志文件，避免未消费的 PIPE 阻塞。
全部业务断言保留；原始与执行脚本哈希见 integration-adaptation.json。

- 本轮状态：已完成；已记录场景未出现失败或丢弃迭代。
- 本轮正式负载每项 10 秒；容量评估应采用 60 秒以上轮次、到达速率阶梯与持续负载。
- 并发 VU 表示负载端的并行虚拟用户，不代表服务端最大连接数。
- rate 场景采用固定迭代到达速率，配置 VU 为预分配值，最大允许扩展到 200 VU；短连接场景固定为 200 次迭代/秒。
- 文件场景的 Python 客户端、哈希和清理请求会限制负载发起能力。
- 本轮不涵盖断网、进程崩溃、磁盘满或对象清理重试，也未据此证明长期无内存泄漏。
- 以下场景有轮次少于 1000 个样本，P99 易波动：upload-duplicate-b1048576-v1、upload-duplicate-b1048576-v4、upload-unique-b1048576-v1、upload-unique-b1048576-v4、download-duplicate-b1048576-v1、download-duplicate-b1048576-v4。

## 复现

构建与运行步骤见 `tests/performance/README.md`。本轮原始命令如下；更换机器后替换源码、工具及构建目录。

```text
C:\Users\徐念念\AppData\Local\Programs\Python\Python313\python.exe tests/performance/run.py --source E:/Clion/disk_gin_cpp --framework-source "E:/Clion/gin_ cpp" --build build/performance-build/disk-release --http-build build/performance-build/gin-release --storage-build build/performance-build/storage-release --stages files --duration 10 --warmup 2 --repeats 3 --accounts 4 --dataset-size 100 --file-bytes 1048576 --recovery 5 --output build/performance/fixed-files-20261119
```

原始结果目录：`E:\Clion\disk\build\performance\fixed-files-20261119`。
保留 metadata.json、provenance.json、各场景 JSON、resources.csv、*sha256.json、构建与功能回归日志。
fixtures.json 含测试账号与令牌，不应随公开报告发布。

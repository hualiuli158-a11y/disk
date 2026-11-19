# Disk / gin_cpp 性能基线

本报告是指定环境下的实测基线。负载端和 API 共享 Windows 主机，存储节点位于同机 WSL，网络为回环链路。
这些数据用于比较相同条件下的版本变化，不代表独立部署的容量上限。

## 版本与环境

- Disk：`438c7b06ab8414969746240c8aef52aff2e8b9e2`；工作树：干净。
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
| api-profile-v1 | GET /api/profile | 1 | 3 | 2194.73（2165.91–2245.09） | 0.000 | 0.802 | 0.935 | 0/66060（0.00%） | 0 |
| api-profile-v8 | GET /api/profile | 8 | 3 | 8476.22（8253.63–8978.32） | 0.563 | 1.662 | 2.166 | 0/257108（0.00%） | 0 |
| api-contents-v1 | GET /api/folders/:id/contents | 1 | 3 | 530.16（524.25–530.56） | 1.846 | 2.220 | 2.572 | 0/15851（0.00%） | 0 |
| api-contents-v8 | GET /api/folders/:id/contents | 8 | 3 | 2062.74（2012.92–2088.89） | 3.691 | 4.485 | 4.949 | 0/61666（0.00%） | 0 |
| api-search-v1 | GET /api/search | 1 | 3 | 480.37（478.96–482.91） | 2.035 | 2.380 | 2.689 | 0/14424（0.00%） | 0 |
| api-search-v8 | GET /api/search | 8 | 3 | 1648.97（1628.86–1674.28） | 4.643 | 5.776 | 6.864 | 0/49544（0.00%） | 0 |
| api-profile-rate100 | GET /api/profile | 8 | 1 | 100.00（100.00–100.00） | 0.980 | 1.865 | 2.125 | 0/1000（0.00%） | 0 |

## 文件传输

文件场景包含首次传输与客户端哈希校验，不单独预热。每次上传后立即软删除、查回收站并永久删除；这些清理请求占用容量，未计入上传延迟，但计入吞吐分母。
下载逐次检查长度和完整 SHA-256。文件延迟分位数只统计成功操作；失败尝试及其耗时单独保存在原始 JSON。MiB/s 只计成功传输。

| 场景 | 并发 | 轮数 | 成功操作/s | MiB/s | 成功 P95 ms | 成功 P99 ms | 失败/尝试 | 清理失败 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| upload-duplicate-b1048576-v1 | 1 | 3 | 16.17 | 16.17 | 51.849 | 55.774 | 0/485（0.00%） | 0 |
| upload-duplicate-b1048576-v4 | 4 | 3 | 32.51 | 32.51 | 149.408 | 203.287 | 83/1147（7.24%） | 0 |
| upload-unique-b1048576-v1 | 1 | 3 | 8.57 | 8.57 | 123.678 | 143.192 | 0/283（0.00%） | 0 |
| upload-unique-b1048576-v4 | 4 | 3 | 55.45 | 55.45 | 60.469 | 67.868 | 0/1667（0.00%） | 0 |
| download-duplicate-b1048576-v1 | 1 | 3 | 46.37 | 46.37 | 43.138 | 61.780 | 0/1403（0.00%） | 0 |
| download-duplicate-b1048576-v4 | 4 | 3 | 64.88 | 64.88 | 103.144 | 136.131 | 0/1958（0.00%） | 0 |

## 进程资源

每秒采样；CPU 以平均占用核心数表示，1.0 表示占用一个逻辑核心。内存为测量轮次期间观察到的峰值，无法覆盖采样间隔内的瞬时尖峰。
这里只列服务进程；客户端、MySQL 和 Linux 存储节点的原始采样另行保留。

| 场景 | 进程 | 平均 CPU 核 | 峰值工作集 MiB | 峰值私有内存 MiB |
|---|---|---:|---:|---:|
| api-profile-v1 | api | 0.630 | 28.90 | 10.32 |
| api-profile-v8 | api | 2.286 | 30.39 | 11.86 |
| api-contents-v1 | api | 0.420 | 30.80 | 12.52 |
| api-contents-v8 | api | 1.750 | 33.71 | 15.42 |
| api-search-v1 | api | 0.261 | 32.46 | 14.04 |
| api-search-v8 | api | 1.020 | 33.89 | 15.43 |
| api-profile-rate100 | api | 0.069 | 33.75 | 15.33 |
| upload-duplicate-b1048576-v1 | api | 0.308 | 34.39 | 15.84 |
| upload-duplicate-b1048576-v4 | api | 1.214 | 41.14 | 22.93 |
| upload-unique-b1048576-v1 | api | 0.212 | 40.83 | 22.22 |
| upload-unique-b1048576-v4 | api | 1.667 | 43.22 | 24.77 |
| download-duplicate-b1048576-v1 | api | 1.471 | 42.88 | 24.52 |
| download-duplicate-b1048576-v4 | api | 2.062 | 44.17 | 25.61 |

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

- 本轮状态：存在失败、丢弃迭代或未完成阶段，见原始结果；不可作为全成功基线。
- 本轮正式负载每项 10 秒；容量评估应采用 60 秒以上轮次、到达速率阶梯与持续负载。
- 并发 VU 表示负载端的并行虚拟用户，不代表服务端最大连接数。
- rate 场景采用固定迭代到达速率，配置 VU 为预分配值，最大允许扩展到 200 VU；短连接场景固定为 200 次迭代/秒。
- 文件场景的 Python 客户端、哈希和清理请求会限制负载发起能力。
- 本轮不涵盖断网、进程崩溃、磁盘满或对象清理重试，也未据此证明长期无内存泄漏。
- 以下场景有轮次少于 1000 个样本，P99 易波动：upload-duplicate-b1048576-v1、upload-duplicate-b1048576-v4、upload-unique-b1048576-v1、upload-unique-b1048576-v4、download-duplicate-b1048576-v1、download-duplicate-b1048576-v4。

## 复现

构建与运行步骤见 [测试说明](../../tests/performance/README.md)。本轮原始命令如下；更换机器后替换源码、工具及构建目录。

```text
C:\Users\徐念念\AppData\Local\Programs\Python\Python313\python.exe tests/performance/run.py --source E:/Clion/disk_gin_cpp --framework-source "E:/Clion/gin_ cpp" --build build/performance-build/disk-release --http-build build/performance-build/gin-release --storage-build build/performance-build/storage-release --stages api,files --duration 10 --warmup 2 --repeats 3 --api-vus 1,8 --cases profile,contents,search --accounts 4 --dataset-size 100 --file-bytes 1048576 --rates 100 --recovery 5 --skip-integration --output build/performance/resume-api-20261119
```

原始结果目录：`E:\Clion\disk\build\performance\resume-api-20261119`。
保留 metadata.json、provenance.json、各场景 JSON、resources.csv、*sha256.json、构建与功能回归日志。
fixtures.json 含测试账号与令牌，不应随公开报告发布。

公开核验材料：[原始 JSON、资源采样和版本证据](evidence/2026-11-19/disk-api/)。

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
| http-plaintext-c1 | GET /benchmark/plaintext | 1 | 3 | 8102.23（7347.37–8122.20） | 0.000 | 0.517 | 0.544 | 0/235725（0.00%） | 0 |
| http-plaintext-c16 | GET /benchmark/plaintext | 16 | 3 | 40183.62（38751.69–41000.18） | 0.000 | 1.007 | 1.508 | 0/1199426（0.00%） | 0 |
| http-json-c1 | GET /benchmark/json | 1 | 3 | 7653.34（7442.79–7654.92） | 0.000 | 0.517 | 0.549 | 0/227523（0.00%） | 0 |
| http-json-c16 | GET /benchmark/json | 16 | 3 | 41751.74（41417.61–42083.76） | 0.000 | 1.007 | 1.507 | 0/1252586（0.00%） | 0 |
| http-echo-c1 | POST /benchmark/echo | 1 | 3 | 7743.11（6646.74–7917.80） | 0.000 | 0.522 | 0.551 | 0/223082（0.00%） | 0 |
| http-echo-c16 | POST /benchmark/echo | 16 | 3 | 39857.05（38697.59–40953.64） | 0.000 | 1.007 | 1.511 | 0/1195171（0.00%） | 0 |
| http-plaintext-short | GET /benchmark/plaintext | 10 | 1 | 200.06（200.06–200.06） | 0.535 | 0.947 | 1.271 | 0/2001（0.00%） | 0 |

## 文件传输

文件场景包含首次传输与客户端哈希校验，不单独预热。每次上传后立即软删除、查回收站并永久删除；这些清理请求占用容量，未计入上传延迟，但计入吞吐分母。
下载逐次检查长度和完整 SHA-256。文件延迟分位数只统计成功操作；失败尝试及其耗时单独保存在原始 JSON。MiB/s 只计成功传输。

| 场景 | 并发 | 轮数 | 成功操作/s | MiB/s | 成功 P95 ms | 成功 P99 ms | 失败/尝试 | 清理失败 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|

## 进程资源

每秒采样；CPU 以平均占用核心数表示，1.0 表示占用一个逻辑核心。内存为测量轮次期间观察到的峰值，无法覆盖采样间隔内的瞬时尖峰。
这里只列服务进程；客户端、MySQL 和 Linux 存储节点的原始采样另行保留。

| 场景 | 进程 | 平均 CPU 核 | 峰值工作集 MiB | 峰值私有内存 MiB |
|---|---|---:|---:|---:|
| http-plaintext-c1 | http-server | 0.297 | 6.53 | 1.86 |
| http-plaintext-c16 | http-server | 2.089 | 8.66 | 3.96 |
| http-json-c1 | http-server | 0.288 | 6.84 | 2.11 |
| http-json-c16 | http-server | 2.327 | 8.72 | 3.99 |
| http-echo-c1 | http-server | 0.306 | 6.84 | 2.08 |
| http-echo-c16 | http-server | 2.208 | 9.03 | 4.29 |
| http-plaintext-short | http-server | 0.104 | 7.11 | 2.35 |

## 正确性与限制

本轮未执行数据库与对象一致性检查。

业务回归执行原分支测试的副本：仅将 HTTP 头读取改为大小写不敏感，并将子进程日志持续写入日志文件，避免未消费的 PIPE 阻塞。
全部业务断言保留；原始与执行脚本哈希见 integration-adaptation.json。

本轮中断原因：Command '['C:\\Users\\徐念念\\AppData\\Local\\Programs\\Python\\Python313\\python.exe', 'E:\\Clion\\disk\\tests\\performance\\api_regression.py', 'E:\\Clion\\disk_gin_cpp', 'E:\\Clion\\disk\\build\\performance-build\\disk-release\\disk_api.exe', 'E:\\Clion\\disk\\build\\performance\\resume-baseline-20261119-135004']' returned non-zero exit status 1.

- 本轮状态：存在失败、丢弃迭代或未完成阶段，见原始结果；不可作为全成功基线。
- 本轮正式负载每项 10 秒；容量评估应采用 60 秒以上轮次、到达速率阶梯与持续负载。
- 并发 VU 表示负载端的并行虚拟用户，不代表服务端最大连接数。
- rate 场景采用固定迭代到达速率，配置 VU 为预分配值，最大允许扩展到 200 VU；短连接场景固定为 200 次迭代/秒。
- 文件场景的 Python 客户端、哈希和清理请求会限制负载发起能力。
- 本轮不涵盖断网、进程崩溃、磁盘满或对象清理重试，也未据此证明长期无内存泄漏。

## 复现

构建与运行步骤见 [测试说明](../../tests/performance/README.md)。本轮原始命令如下；更换机器后替换源码、工具及构建目录。

```text
C:\Users\徐念念\AppData\Local\Programs\Python\Python313\python.exe E:\Clion\disk\tests\performance\run.py --source E:\Clion\disk_gin_cpp --framework-source "E:\Clion\gin_ cpp" --build E:\Clion\disk\build\performance-build\disk-release --http-build E:\Clion\disk\build\performance-build\gin-release --storage-build E:\Clion\disk\build\performance-build\storage-release --k6 E:\Clion\disk\build\tools\k6-v2.4.0-windows-amd64\k6.exe --distro Ubuntu --output E:\Clion\disk\build\performance\resume-baseline-20261119-135004 --stages http,api,files --duration 10 --warmup 2 --repeats 3 --connections 1,16 --api-vus 1,8 --cases profile,contents,search --accounts 4 --dataset-size 100 --file-bytes 1048576 --rates 100 --recovery 5
```

原始结果目录：`E:\Clion\disk\build\performance\resume-baseline-20261119-135004`。
保留 metadata.json、provenance.json、各场景 JSON、resources.csv、*sha256.json、构建与功能回归日志。
fixtures.json 含测试账号与令牌，不应随公开报告发布。

公开核验材料：[原始 JSON、资源采样和版本证据](evidence/2026-11-19/gin-http/)。

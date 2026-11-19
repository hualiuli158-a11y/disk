# 后端性能测试

## Disk / gin_cpp 简历项目基线

`baseline.ps1` 为独立 gin_cpp 框架和使用它的 Disk API 提供统一入口。显式指定 Disk 源码目录，避免将旧 `cpp-backend` 的数据归到 `cpp-backend-gin`。

```powershell
./tests/performance/baseline.ps1 -DiskSource E:/Clion/disk_gin_cpp -GinSource 'E:/Clion/gin_ cpp' -Duration 15 -Warmup 3 -Repeats 3
```

脚本将全部构建和输出写到本工作区 `build/`，不修改传入的源码目录。需要 MinGW、Boost、已安装的 MySQL 8、Ubuntu WSL 的 gRPC/Protobuf 开发依赖和 k6。通过 `-CompilerRoot`、`-BoostRoot`、`-K6`、`-MouduSource`、`-Distro` 指定本机位置；依赖位置应与项目 CMake 配置一致。

默认每项正式测量 60 秒、预热 30 秒、重复三轮。上述 15 秒参数用于初始基线；每个 HTTP/API 场景在正式轮次前预热一次，文件场景包含首次传输且不单独预热。构建使用 Release，API 的 gRPC 静态依赖实际配置仍需结合报告中的 CMake 缓存解释。

入口先运行 Disk 与 gin_cpp 的 CTest，再执行真实 MySQL + gRPC 业务回归和以下矩阵：

- gin_cpp 固定文本、动态 JSON、1 KiB POST 回显，1 / 16 VU。
- Disk 用户信息、100 项目录列表和名称搜索，1 / 8 VU；用户信息另测 100 次/秒到达速率。
- 1 MiB 文件重复与不同内容上传，1 / 4 并发；1 / 4 并发下载并校验完整 SHA-256。
- 压测结束后检查配额、物理记录引用、缺失对象和孤儿对象。

独立 gin_cpp 基准沿用 Disk 的 MinGW GCC 16 编译兼容选项 `_GLIBCXX_ATOMIC_WORD_BUILTINS=1`。业务回归使用源码测试的副本，修正 HTTP 头名大小写读取和未消费日志 PIPE 两处测试运行问题，保留全部业务断言，并记录原始与执行副本的哈希；不修改 Disk 或 gin_cpp 源码。

`-SkipBuild` 复用已经构建的二进制；`-SkipIntegration` 仅用于已经有同版本业务回归证据的重复压测。每轮创建新输出目录。输出 `baseline-zh.md`、`baseline-summary.json`、原始 JSON、资源采样、构建缓存和源码/二进制 SHA-256。已有结果可重新生成报告：

```powershell
python tests/performance/summarize.py build/performance/<本轮目录>
```

多轮结果按场景汇总 RPS 中位数与范围、各轮 P50/P95/P99 的中位数、失败数及丢弃迭代数；分位数不会被当成可直接合并的原始样本。资源按相同进程和场景的相邻采样计算，避免将启动或其他场景的 CPU 时间算入目标场景。k6 校验或阈值失败会立即停止并保留失败场景。文件专项记录每次失败尝试和清理失败，继续采集矩阵，结束后返回非零退出码；文件延迟分位数只统计成功操作，失败耗时单独保留。统计口径回归：

```powershell
python -m unittest discover -s tests/performance -p test_summarize.py -v
```

原始 `fixtures.json` 含测试账号与令牌，仅保留在被 Git 忽略的 `build/`；公开报告只分享汇总、无认证信息的采样和版本证据。

测试入口覆盖当前 MouduAsio TCP、`disk::HttpServer` 和真实 API。测试结果、账号、数据库、对象和便携工具保存在 `build/` 下。

## 构建

```powershell
./scripts/build.ps1 -BuildDirectory build/mingw-release -Configuration Release -Benchmarks
ctest --test-dir build/mingw-release --output-on-failure
```

`-Configuration` 默认仍为 `Debug`。`-Benchmarks` 启用两个独立程序。

- `disk_tcp_benchmark.exe` 提供 Echo 服务和异步负载客户端。
- `disk_http_benchmark.exe` 提供固定文本、动态 JSON 和 POST 回显，经过项目实际 HTTP 调度与响应路径。

记录实际链接的依赖配置。当前工作站的 gRPC 安装目录为 `E:/Libraries/grpc-mingw-debug`，项目 Release 构建仍使用其中的 Debug 静态库。

## 运行环境

当前自动环境使用 Windows MySQL 和 Ubuntu WSL 中的 C++ 存储节点。

- `--mysql-bin` 指定 MySQL 的 `bin` 目录，默认 `C:/Program Files/MySQL/MySQL Server 8.0/bin`。
- `--distro` 默认 `Ubuntu`。
- 存储程序位于 `build/storage-linux/grpc_storage_node`，可在 WSL 中执行 `cmake -S /mnt/e/Clion/disk/storage -B /mnt/e/Clion/disk/build/storage-linux`，再执行 `cmake --build /mnt/e/Clion/disk/build/storage-linux --parallel 2`。
- `--storage-build` 可选择其他存储构建目录。运行元数据记录 API 和存储节点各自的构建类型。本工作站现有存储节点为 Debug 构建。
- `--k6` 指向 k6 可执行文件。默认使用项目内经官方 SHA-256 校验的 `build/tools/k6-v2.4.0-windows-amd64/k6.exe`。其他位置的 k6 可通过参数指定。

运行器在 `build/performance/runtime/mysql` 初始化专用 MySQL 数据目录，绑定回环地址和动态端口。每次新建数据库及对象目录，并应用现有迁移。账号和令牌保存在本轮输出的 `fixtures.json`。退出时停止本轮启动的 API、存储和 MySQL，保留数据和报告供核验。

## 短时验证

```powershell
python tests/performance/run.py --stages tcp,http,api,files,mixed --duration 5 --warmup 1 --repeats 1 --connections 1,10 --io-threads 1,2 --message-bytes 64,65536 --api-vus 1,4 --accounts 4 --dataset-size 10 --file-bytes 0,1048576 --rates 10 --recovery 5
```

## 分阶段基线

以下数字为可调的测试参数。默认预热 30 秒、正式测量 60 秒、重复三次。完整矩阵运行时间随组合数量增长，可通过参数选择本轮维度。

```powershell
python tests/performance/run.py --stages tcp --connections 1,10,100,1000 --io-threads 1,2,4,8 --message-bytes 64,1024,16384,65536
python tests/performance/run.py --stages http --connections 1,10,100
python tests/performance/run.py --stages api --api-vus 1,10,50 --accounts 10 --dataset-size 100 --rates 100,500
python tests/performance/run.py --stages files --accounts 4 --file-bytes 0,1048576,33554432,268435456
python tests/performance/run.py --stages mixed,soak --api-vus 10 --soak-seconds 1800
```

API 阶段先运行 `tests/api_integration.py`，包括真实 MySQL、存储、bcrypt、并发配额和物理引用核验。`--skip-integration` 可以复用已完成的功能回归结论。查询接口逐个运行，写接口通过完整的创建、重命名、移动、下载、软删除、恢复和永久删除流程测量，结果按路由汇总。`--accounts 1` 和多个虚拟用户可观察同用户行锁竞争。

文件阶段覆盖重复内容和不同内容、单项及四项上传，并校验完整下载 SHA-256。上传完成后执行清理。清理请求带 `cleanup` 标签，消耗服务端容量，目标上传指标单独汇总。混合负载允许预期的 429，仍将其列为未成功准入的请求。全部阶段结束后检查配额、物理引用、对象缺失和磁盘孤儿对象。

文件专项使用 Python 线程和 64 KiB 分块读写，每个工作线程复用 HTTP 连接。每项完整传输都会结束后再统计，文件阶段按实际运行时长计算吞吐。其首次传输和哈希计算均计入测量。`--tcp-extras` 增加 TCP 反复建连、延迟读取和 1000 连接场景，`--client-threads` 设置 TCP 客户端线程数，`--http-threads` 设置 HTTP 基准 I/O 线程数，`--short-vus` 设置短连接场景并发数。`--cases` 可缩小 API 场景，例如 `--cases profile,contents,workflow`。

上传及随后清理共用该工作线程的 HTTP 连接。混合负载以最多四个虚拟用户持续上传，其余虚拟用户执行下载、列表、搜索和用户信息请求。下载使用初始化的 `seed-file.bin`，上传使用 `UPLOAD_FILE` 指定的数据。每份 k6 JSON 保存实际脚本的 SHA-256。

混合阶段还测量登录与普通查询的相互影响，并暂停本轮存储节点，记录 `/readyz` 超时期间 `/livez` 的延迟，随后恢复节点。故障观察保存在 `health-storage-stall.json`。

持续测试超过十分钟时，各虚拟用户按需重新登录。重新登录使用独立的 `authentication` 标签。

## TCP 参数

```powershell
./build/mingw-release/disk_tcp_benchmark.exe server --port 18888 --threads 2 --duration 600
./build/mingw-release/disk_tcp_benchmark.exe client --port 18888 --threads 2 --connections 100 --bytes 1024 --warmup 30 --duration 60
./build/mingw-release/disk_tcp_benchmark.exe client --port 18888 --connections 10 --bytes 1024 --duration 30 --reconnect 1 --reconnect-delay-ms 20
./build/mingw-release/disk_tcp_benchmark.exe client --port 18888 --connections 10 --bytes 65536 --duration 30 --read-delay-ms 10
```

客户端每连接保持一个在途消息，完整接收并校验回显后继续发送，使用单调时钟。延迟直方图粒度为 0.1 ms，溢出桶返回该组观测最大值。预热开始的请求排除在测量之外，截止时的未完成请求单独记录。`connections_established` 包含预热期间的连接。吞吐按收到的有效负载计算，双向吞吐可由对称回显推算。Echo 保留现有示例的单连接待发送数据超过 2 MiB 时关闭的策略。

`connects_per_second` 和 `connect_p50/p95/p99_ms` 记录测量窗口内完成的建连。`last_error_code` 保存最后一个连接错误码。自动反复建连场景使用每连接 20 ms 间隔，HTTP 短连接场景采用每秒 200 次迭代的发起速率，报告保留这些负载条件。本工作站的 TCP 动态端口范围为 1024～15000，高频建连测试需要结合临时端口及 TIME_WAIT 状态解释结果。

## k6 单独执行

在运行中的 API 上可以直接测公共健康接口。

```powershell
$env:BASE_URL = 'http://127.0.0.1:8080'
$env:CASE = 'livez'
$env:VUS = '1'
$env:DURATION = '60s'
$env:SUMMARY_FILE = 'E:/Clion/disk/build/performance/profile.json'
./build/tools/k6-v2.4.0-windows-amd64/k6.exe run tests/performance/routes.js
```

账号和受保护接口场景通过 `FIXTURES` 指向运行器生成的 `fixtures.json`，其中的测试账号、资源 ID 与正在运行的 API 所用数据库配套。`CASE=profile` 测量用户信息接口，其他场景名称见脚本。

`RATE` 启用固定迭代到达速率，`MAX_VUS` 控制最大虚拟用户数。单接口测试每次迭代一个目标请求。`workflow` 包含多个请求，需同时查看迭代和请求指标。`SHORT_CONNECTIONS=1` 禁用连接复用。`P99_MS` 可设置目标请求的 P99 阈值。默认校验所有响应，性能阈值由业务目标确定。

运行器的 `--rate-vus` 控制到达速率场景预分配的虚拟用户数。公共健康检查直接请求 `/livez` 和 `/readyz`。接口饱和及客户端连接错误均记录在结果中，可结合状态码、错误日志和 `dropped_iterations` 判断本轮负载条件。

## 输出与指标口径

- `report.md` 和 `routes.csv` 汇总逐场景、逐路由吞吐和延迟。
- 每个 k6 JSON 保留路由分位数、状态码、目标校验结果及 `dropped_iterations`。
- `resources.csv` 每秒记录 Windows 子进程累计 CPU 时间、工作集和私有内存。CPU 时间相邻差值除以采样间隔得到使用的核心数。
- Windows MySQL 的实际工作进程以 `mysql-worker` 记录，启动监控进程沿用 `mysql` 标签。
- `storage-resources.stdout.log` 每秒记录 Linux 存储节点的 CPU ticks、驻留页数和线程数，首行给出时钟频率与页大小。
- `environment.json` 保存本轮专用服务地址与配置，`metadata.json` 保存代码版本和参数。
- `invariants.json` 保存压测后的数据与对象一致性核验。
- `api-integration.log` 保存功能回归结果，服务与负载程序分别记录 stdout/stderr。

HTTP `target_duration_ms` 包含发送、等待和完整接收。`target_complete_ms` 额外计入连接阻塞阶段。API 目标吞吐依据成功目标请求和实际运行时长计算。客户端正文验证与清理会影响发起负载的能力，应结合客户端 CPU、实际吞吐及丢弃迭代数解释结果。

当前本机模式下负载端和 API 共享 Windows 机器，存储在 WSL 中运行。Windows 和 Linux 资源分别采样。测试矩阵中的不同数据规模、连接策略和构建配置应在比较时保持一致。

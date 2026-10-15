# 使用 C++ gRPC 对象存储节点

Go 文件服务通过 `internal/storage.Storage` 使用 `Save`、`Open`、`Delete` 和
`List`。本地和 gRPC 是两个可选实现；用户权限、目录、数据库元数据、
配额和 SHA-256 去重仍由 Go 负责。gRPC 客户端位于
`internal/storage/grpcstore`，协议定义和 C++ 服务端位于
本仓库的 `storage/`。服务自带 `storage/src/storage/` 中的 `LocalStore`，无需独立 TCP 项目。

Linux 上先构建并启动 gRPC 服务端：

```sh
cd storage  # 从统一仓库根目录执行
cmake -S . -B build
cmake --build build -j
./build/grpc_storage_node 127.0.0.1:50051 /home/ubuntu/object-data
```

Go 也在这台 Linux 主机运行时，设置：

```dotenv
STORAGE_BACKEND=grpc
STORAGE_ADDRESS=127.0.0.1:50051
STORAGE_TIMEOUT=30m
```

Go 在 Windows、gRPC 节点在 Linux 时，可以保持 C++ 节点只监听
`127.0.0.1`，在 Windows 的单独 PowerShell 终端建立 SSH 隧道：

```powershell
ssh -N -L 50051:127.0.0.1:50051 ubuntu@服务器地址
```

保持该终端运行。Go 端仍使用 `STORAGE_ADDRESS=127.0.0.1:50051`。
PowerShell 临时设置环境变量的语法是
`$env:STORAGE_BACKEND = "grpc"`；`.env` 文件使用上面的 `KEY=value` 语法。
Go 的 MySQL 连接独立于对象存储；如果 MySQL 也在 Linux，需要单独保证
`DB_HOST` 对 Windows 上的 Go 进程可达。

Go 提供无需登录的 `GET /livez` 和 `GET /readyz`。前者检查 HTTP 进程，后者
检查 MySQL 和 C++ 节点的标准 gRPC health 服务；依赖不可用时返回 503。
Go 启动时如果 gRPC 存储不可用，会返回明确错误并停止启动；运行过程中
依赖断线则由 `/readyz` 返回 503。部署新版本后需重新编译并重启
C++ 服务，使 health 服务和分页协议可用。

直接跨主机连接时，C++ 服务必须启用双向 TLS。Go 设置 CA 文件和服务端证书名称：

```dotenv
STORAGE_TLS_CA=/path/to/ca.pem
STORAGE_TLS_SERVER_NAME=storage.example.internal
```

同时设置客户端证书：

```dotenv
STORAGE_TLS_CLIENT_CERT=/path/to/client.pem
STORAGE_TLS_CLIENT_KEY=/path/to/client.key
```

无 TLS 的 Go 连接只接受回环地址，用于本机或 SSH 隧道。`JWT_SECRET` 至少
需要 32 字节；更换密钥后须重新登录。认证令牌只从
`Authorization: Bearer <token>` 或兼容用的 `token` 请求头读取，URL 查询
参数不再接受令牌。

切换后不会自动迁移原后端的对象数据：数据库保存的
storage key 必须仍能在选定后端找到。上传、下载按 64 KiB 块传输；单对象
最多 1 GiB。`List` 供孤儿对象清理使用，不代表用户目录列表。后台清理
按最多 1000 条分页并记录进程内游标；重启后从头扫描。孤儿清理失败时
仍会尝试清理过期回收站。

运行 Go 测试：

```sh
go test ./...
```

真实跨语言测试应使用 C++ 节点的临时存储根目录，并设置
`STORAGE_TEST_GRPC_ADDRESS=127.0.0.1:50051`，然后运行
`go test ./internal/storage/grpcstore -run TestCppServerInteroperability -v`。
也可以在 Linux 上从 gRPC 服务端项目运行
`bash scripts/test-go-interoperability.sh ../backend`，脚本
会自动构建并启动临时 C++ 节点。

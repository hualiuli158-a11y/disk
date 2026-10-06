# 使用 C++ gRPC 对象存储节点

Go 文件服务通过 `internal/storage.Storage` 使用 `Save`、`Open`、`Delete` 和
`List`。本地和 gRPC 是两个可选实现；用户权限、目录、数据库元数据、
配额和 SHA-256 去重仍由 Go 负责。gRPC 客户端位于
`internal/storage/grpcstore`，协议定义和 C++ 服务端位于
`E:/Clion/grpcService/disk`。C++ 服务端复用 `objectRestore` 的 `LocalStore`。

Linux 上先构建并启动 gRPC 服务端：

```sh
cd /home/ubuntu/rpcdisk
cmake -S . -B build -DOBJECT_RESTORE_ROOT=/home/ubuntu/objectrestore
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

Go 客户端和 C++ 服务端当前使用未加密的 gRPC 连接，请使用本机、SSH 隧道
或受信任的私有网络。切换后不会自动迁移原后端的对象数据：数据库保存的
storage key 必须仍能在选定后端找到。上传、下载按 64 KiB 块传输；单对象
最多 1 GiB。`List` 供孤儿对象清理使用，不代表用户目录列表。

运行 Go 测试：

```sh
go test ./...
```

真实跨语言测试应使用 C++ 节点的临时存储根目录，并设置
`STORAGE_TEST_GRPC_ADDRESS=127.0.0.1:50051`，然后运行
`go test ./internal/storage/grpcstore -run TestCppServerInteroperability -v`。

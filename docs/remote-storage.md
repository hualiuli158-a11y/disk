# 使用 C++ 对象存储节点

Go 网盘默认仍使用本地文件存储。选择远程模式后，上传、下载、永久删除和孤儿清理通过 `internal/storage/remote_storage.go` 访问独立运行的 C++ `storage_node`。用户、目录、配额、数据库事务和 SHA-256 去重留在 Go 服务中。

先在 Linux 或 WSL 中编译并运行 `objectRestore` 项目的 C++ 节点（以下命令在该项目目录执行）：

```bash
cmake -S . -B build -DBUILD_TESTING=ON
cmake --build build --target storage_node -j
./build/storage_node 127.0.0.1 9000 ./storage-data
```

在 Go 项目的环境配置中设置：

```dotenv
STORAGE_BACKEND=remote
STORAGE_REMOTE_ADDRESS=127.0.0.1:9000
STORAGE_REMOTE_TIMEOUT=30m
```

然后按项目原有方式启动 Go 服务。两端通过 `ORST` V1 二进制协议通信：单连接单操作，帧头 24 字节，数据帧最多 64 KiB；`Open` 返回按帧读取的 `io.ReadCloser`。如果 Go 与 C++ 不在同一台机器，`127.0.0.1` 指向各自所在的机器，必须改为 C++ 节点可达的地址，并自行确定受信任的网络边界。

本地模式设 `STORAGE_BACKEND=local`（默认）；只使用 `STORAGE_ROOT`，无需启动 C++ 节点。切换后不会自动搬迁本地已有对象：数据库保存的旧 storage key 仍指向原对象，部署迁移前应安排数据复制或保持旧后端可读。

完整 Go 测试：

```bash
go test ./...
go vet ./...
```

如需运行真实跨语言测试，先启动测试专用的 C++ 节点和空目录，再设置 `STORAGE_NODE_TEST_ADDRESS=127.0.0.1:9000`，运行 `go test -run TestRemoteStorageAgainstCppNode ./internal/storage`。测试会创建随机 key，并尝试清理它们；不要指向生产节点。

当前尚未实现连接池或自动重试上传。上传完成响应丢失时，结果可能不确定，依靠 Go 的对象枚举和孤儿清理处理无数据库引用的候选对象。C++ 节点仍需补充读写超时、崩溃后临时文件清扫和更完整的在途任务停机。

# Repository 的 MySQL 集成测试

`internal/repository` 的集成测试使用 Go `testing`、Testcontainers for Go 和真实 MySQL。默认跳过数据库测试；设置 `DISK_MYSQL_INTEGRATION=1` 后，测试包的 `TestMain` 自动准备并清理环境。本次整理了 30 个 `FilesRepository` 测试函数，覆盖上传、内容替换、版本、访问控制、重命名、移动和并发操作。

## 前置条件与运行方式

确保 Docker Desktop 正在运行。在项目根目录的 PowerShell 中执行：

```powershell
$env:DISK_MYSQL_INTEGRATION = '1'
go test ./internal/repository -count=1 -v
```

`-count=1` 禁用测试结果缓存，`-v` 显示每项测试及跳过信息。只运行用户仓储测试时，可以使用：

```powershell
go test ./internal/repository -run '^TestUserRepositoryCreateRootMySQL$' -count=1 -v
```

只运行文件仓储测试：

```powershell
go test ./internal/repository -run '^TestFilesRepository' -count=1 -v
```

重复运行并发用例，检查不同执行顺序下的结果：

```powershell
go test ./internal/repository -run 'Concurrent' -count=3 -timeout=3m
```

未设置 `DISK_MYSQL_INTEGRATION=1` 时，普通 `go test` 不会启动 Docker，MySQL 集成测试会跳过。运行结束后，如需恢复当前 PowerShell 会话的默认行为：

```powershell
Remove-Item Env:DISK_MYSQL_INTEGRATION
```

## 自动准备与清理

在 [`main_test.go`](../internal/repository/main_test.go) 中，`TestMain` 按以下顺序管理测试环境：

1. 读取 `DISK_MYSQL_INTEGRATION`。值不是 `1` 时直接运行普通测试，不启动 Docker。
2. 启动临时 `mysql:8.0.36` 容器和 `disk_repository_test` 数据库，解析容器提供的 DSN。
3. 通过 `database.OpenMySQL` 建立连接，运行 `database.NewMigrationRunner(...).Up(ctx)`，确保表结构与正式迁移一致。
4. 把 GORM 连接提供给测试；`m.Run()` 结束后关闭连接并销毁容器。Testcontainers 还会启动用于清理资源的 Ryuk 容器。

`openRepositoryTestDB` 使用 `TestMain` 提供的连接；不需要手动建库、执行迁移命令，或配置 `DB_HOST`、`DB_USER`、`DB_NAME`、`DB_PASSWORD`。`TestMain` 只管理 `internal/repository` 这个 Go 测试包。

### 测试数据与隔离

- 普通文件用例通过 [`newFilesTestFixture`](../internal/repository/FilesCreateUploadedFile_test.go) 直接插入用户和目录，并在各自的外层事务中运行；`t.Cleanup` 回滚数据。这些用例不依赖 `UserRepository.CreateRoot` 的正确性。
- `TestFilesRepositoryCreateUploadedFileAfterUserRegistration` 单独验证“创建用户及根目录后上传”的跨仓储流程，因此有意调用 `UserRepository.CreateRoot`。
- 并发用例需要两个连接都看见前置数据，因此先提交夹具，再用 GORM `Connection` 为每个操作占用独立连接；两个操作同时开始。结束后按版本、逻辑文件、目录、用户、物理文件的顺序删除夹具。
- 断言同时检查方法返回值与重新查询的数据库状态：物理引用、版本来源和数量、存储键、额度。写操作的错误用例还检查失败前后的状态是否一致。

## 本次测试范围

以下是本次文件仓储测试对应的行为与断言。测试函数统一采用 `TestFilesRepository<方法><场景>` 命名。

| 优先级 | 行为 | 主要断言 |
| --- | --- | --- |
| P0 | 首次上传；相同 SHA-256 再次上传 | 首次生成一条逻辑文件、一条物理记录和一条 `upload` 版本，`physicalCreated=true`；再次上传复用物理 ID 与原存储键，`physicalCreated=false`，额度仍按两份逻辑文件累计。 |
| P0 | 上传参数、权限与额度边界 | 不存在的用户或目录、其他用户的目录分别返回对应错误；同目录同名、超额、负数大小不留下新记录或额度变化；恰好用满额度成功。 |
| P0 | 替换文件内容 | 变大、变小和复用已有 SHA-256 时切换当前引用，额度按大小差变化。仅 createVersion=true 创建 edit 快照；自动保存不增加版本。同 SHA-256 手动保存可为已自动保存的内容补建快照，与最新快照相同则去重。 |
| P0 | 替换失败 | 预期 SHA-256 不匹配、文件不属于用户、增长后超额分别返回 `ErrFileVersionConflict`、`ErrFileNotFound`、`ErrQuotaExceeded`；当前引用、版本和额度不变。 |
| P0 | 版本列表与历史下载 | 最新版本排在前面，预加载的物理 ID、SHA-256、大小和存储键正确；其他用户不可读，其他文件的版本 ID 不可借用。 |
| P0 | 恢复旧版本 | 当前物理引用指向目标版本，新增带 `restored_from_version_id` 的 `restore` 版本，额度按大小差调整；SHA-256 冲突、版本缺失、增长后超额保持原状态；恢复当前内容不新增版本。 |
| P1 | 版本保留上限 | 写入第 51 条版本后只保留最新 50 条，顺序正确，被清理的版本无法按 ID 下载。 |
| P1 | 文件列表与当前下载 | 只返回用户在当前目录下的直接文件及完整物理信息；跨用户访问失败；软删除文件不可见。 |
| P1 | 重命名与移动 | 成功后字段正确；同名冲突和跨用户访问返回对应错误；重命名为现名、移动到现目录成功。 |
| P1 | 并发上传与替换 | 两个用户并发上传相同 SHA-256 最终只有一条物理记录；同一用户并发上传不会超过额度；基于同一旧 SHA-256 的并发替换只有一个成功。 |

对应文件：[上传用例](../internal/repository/FilesCreateUploadedFile_test.go)、[替换与版本用例](../internal/repository/file_versions_mysql_integration_test.go)、[列表与文件操作用例](../internal/repository/file_access_mysql_integration_test.go)、[并发用例](../internal/repository/file_concurrency_mysql_integration_test.go)。

仓储包还有两项相关测试：

| 测试 | 验证内容 |
| --- | --- |
| `TestUserRepositoryCreateRootMySQL` | 创建用户及根目录；按用户名和 ID 查回用户；确认根目录存在；查询不存在的用户时返回 `ErrUserNotFound`。 |
| `TestFolderContentsMySQLProjection` | 创建子目录；检查目录列表的数量、类型及父目录 ID；检查搜索能找到该目录。目前未用文件记录验证搜索的文件分支。 |

### 本次执行结果与范围边界

2026-10-12 使用真实 MySQL 执行 `go test ./internal/repository -count=1 -timeout=3m`，仓储包测试通过；`-run 'Concurrent' -count=3 -timeout=3m` 也通过。负向用例可能让 GORM 打印 `record not found` 或 `duplicated key`；对应测试显示 `PASS` 时属于预期日志。

这些测试验证仓储层的数据库元数据与事务结果。文件字节写入、对象存储和 HTTP 上传路径不在本文测试范围内，需要查看服务层与处理器层的测试。

## 查看覆盖率

保持 `DISK_MYSQL_INTEGRATION=1`，直接在终端查看 `repository` 包的覆盖率：

```powershell
go test ./internal/repository -cover -count=1
```

要查看各函数和代码行的覆盖情况，可以生成覆盖率文件：

```powershell
go test ./internal/repository -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out
go tool cover -html=coverage.out
```

`-coverprofile` 会创建 `coverage.out`；该文件已被项目的 `.gitignore` 忽略。未开启集成测试开关时，MySQL 集成测试会跳过，覆盖率也不会包含它们执行过的路径。覆盖率只表示代码执行情况，不等于所有功能和边界情况都已得到验证。

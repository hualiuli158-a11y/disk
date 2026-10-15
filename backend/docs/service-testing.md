# Service 层测试

## 测试分工

| 位置 | 依赖 | 验证重点 |
| --- | --- | --- |
| `internal/service/*_test.go` | fake/stub 仓储和存储 | 输入校验、错误映射、调用顺序、候选对象清理、版本与回收站业务分支。普通 `go test` 即可运行。 |
| `internal/integration/*_test.go` | 真实 MySQL、真实文件或清理仓储、本地存储 | 服务调用后数据库记录与磁盘字节是否一致，以及孤儿对象是否被清理。设置 `DISK_MYSQL_INTEGRATION=1` 后运行。 |

测试函数统一按 `Test<服务类型><方法><场景>` 命名，例如 `TestUploadFileServiceUploadFileMapsStorageTooLarge`。每个行为使用独立用例；同一行为的输入变体使用 `t.Run`。

## 运行

在项目根目录运行 service 单元测试：

```powershell
go test ./internal/service -count=1
```

一次运行 service 单元测试和真实 MySQL、本地存储的跨层测试，需要先启动 Docker Desktop：

```powershell
$env:DISK_MYSQL_INTEGRATION = '1'
go test ./internal/service ./internal/integration -count=1 -v -timeout=3m
Remove-Item Env:DISK_MYSQL_INTEGRATION
```

[`internal/integration/main_test.go`](../internal/integration/main_test.go) 每轮只启动一次临时 MySQL，并执行项目迁移。三个上传用例和一个孤儿清理用例各自使用独立用户、数据库事务和 `t.TempDir()`；结束时回滚记录并清理本地对象。未设置开关时，跨层用例跳过，不启动 Docker。

## 当前用例

| 服务 | 已验证的行为 |
| --- | --- |
| 注册、登录 | 成功流程、无效输入或凭据、用户已存在、查询与令牌错误。 |
| 上传 | 存储拒绝、声明大小不符；跨层用例还验证上传后下载、同名冲突后的候选对象清理、相同内容去重。 |
| 文件内容与版本 | 新版本保存、旧版本冲突、候选对象清理、无效或过大文本、历史版本列表与下载、恢复和错误映射。 |
| 孤儿清理、回收站清理 | 数据库候选与本地对象处理、错误返回、分页扫描、近期对象跳过、到期清理与取消；跨层用例还验证真实数据库和本地存储中的孤儿被删除、引用中的对象被保留。 |

Service 包中的清理用例使用 fake/stub 验证服务分支；[`cleanup_service_test.go`](../internal/integration/cleanup_service_test.go) 使用真实 [`repository/cleanup.go`](../internal/repository/cleanup.go) 和本地存储验证基本清理流程。候选对象删除本身失败后的孤儿回收仍需单独测试。

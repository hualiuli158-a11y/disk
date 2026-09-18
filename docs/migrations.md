# 数据库迁移

应用启动时只检查数据库迁移版本，不再执行 `AutoMigrate`。

## 新数据库

```powershell
go run ./cmd/migrate up
go run .
```

## 接管已有数据库

先备份数据库，并确认现有结构已经包含当前模型需要的表和索引：

```sql
SHOW CREATE TABLE users;
SHOW CREATE TABLE folders;
SHOW CREATE TABLE files;
SHOW CREATE TABLE physical_files;
SHOW CREATE TABLE trash_entries;
```

重点确认存在：

```text
idx_active_folder_name
idx_active_file_name
```

然后标记当前基线：

```powershell
go run ./cmd/migrate baseline
go run ./cmd/migrate version
```

`baseline` 不执行建表 SQL，只在验证必要表和索引后写入版本号。

## 常用命令

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate down 1
go run ./cmd/migrate version
go run ./cmd/migrate baseline
```

MySQL DDL 可能隐式提交，因此迁移失败后数据库会标记为 `dirty`。此时应检查已经执行的 SQL，人工修复结构后再继续，不要直接反复运行迁移。

## 新增迁移

每次修改数据库结构时增加一对文件：

```text
internal/database/migrations/000002_feature.up.sql
internal/database/migrations/000002_feature.down.sql
```

已提交并在环境中执行过的迁移文件不得修改。

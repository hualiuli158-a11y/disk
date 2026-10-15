# 统一目录与迁移说明

整理日期：2026-10-15。目标目录：`E:\Maintable`。

| 来源 | 新目录 |
| --- | --- |
| `E:\newproject\disk` | `backend/` |
| `E:\react\disk_front` | `frontend/` |
| `E:\Clion\grpcService\disk` | `storage/` |

独立的 `E:\Clion\objectRestore` TCP 项目未复制。
gRPC 项目自带 `storage/src/storage/`，无需依赖该 TCP 项目。

复制当前工作区的源码、测试、迁移、锁文件、协议、Dockerfile、
配置示例、脚本和文档，包括未提交文件。
未复制旧 `.git`、IDE 设置、依赖与构建缓存、测试输出、
真实环境配置、证书私钥、归档备份和运行数据。
`backend/docs/file/` 是原项目已忽略的本地资料，也未复制。
原 Go 目录的 `$coveragePath` 覆盖率产物未复制。

三个原目录与提交历史保留。统一仓库沿用 GitHub `hualiuli158-a11y/disk` 的 `master` 分支，
保留已有远程和本地 Go 提交历史；前端与 C++ 以当前源码快照加入，未合并这两个组件的旧历史。
新仓库没有嵌套 `.git` 或子模块。

## 路径约定

- Docker 构建上下文为 `backend/`、`frontend/`、`storage/`。
- Go 测试与迁移命令从 `backend/` 执行。
- npm 命令从 `frontend/` 执行。
- C++ 构建从 `storage/` 执行，跨语言测试的 Go 路径为 `../backend`。
- 协议位于 `storage/proto/`，生成的 Go 文件位于
  `backend/internal/storage/grpcstore/pb/`。

历史部署报告保留原始路径，便于审计；它们不是新布局说明。
本次整理不会修改服务器 `/home/ubuntu/disk`、已有数据或镜像。

统一仓库采用后，后续修改在本目录下进行。继续修改旧目录不会自动同步到这里。
跨服务接口变更建议在同一次提交中包含前后端及协议修改。

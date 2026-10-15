# 栖盘（Disk）

Go HTTP API、React 前端与 C++ gRPC 对象存储的统一源码仓库。
三个服务分别构建镜像、分别运行，统一记录接口变更与发布版本。

## 目录

| 目录 | 内容 |
| --- | --- |
| [backend/](backend/) | Go API、迁移、生成的 Go protobuf、测试及后端文档 |
| [frontend/](frontend/) | React + TypeScript + Vite 浏览器文件工作空间 |
| [storage/](storage/) | C++ gRPC、协议和本地对象存储实现 |
| [deploy/](deploy/) | 统一构建与部署说明、现有配置索引 |
| [docs/](docs/) | 文档索引、统一目录说明和历史运维记录入口 |

不包含独立的 C++ TCP 项目。`storage/src/storage/` 已包含 gRPC 服务需要的存储实现。

## 环境要求

- Go：以 `backend/go.mod` 的版本要求为准。
- 前端：Node.js 22.12+、npm，依赖版本由 `package-lock.json` 固定。
- C++：Linux、CMake、C++ 编译器、gRPC/Protobuf 开发包，或使用 Dockerfile 构建。
- 完整服务还需要 MySQL、TLS/JWT 配置以及 HTTPS 入口。

## 本地开发

只查看前端演示页面：

```sh
cd frontend
npm ci
npm run dev
```

演示数据保存在内存中，刷新后重置。连接真实后端使用 `npm run dev:api`。
Go 配置示例为 [`backend/.env.example`](backend/.env.example)。
在 `backend/` 中创建自己的 `.env`，填写数据库、JWT、运行环境及存储配置；
不要使用占位密码。真实联调需要 HTTPS 或符合代码限制的本地开发配置。
数据库初始化与已有库升级见 [迁移说明](backend/docs/migrations.md)。

```sh
cd backend
go run ./cmd/migrate up
go run .
```

gRPC 构建、TLS 与跨语言测试见 [`storage/README.md`](storage/README.md)。
本地开发可选本地存储；完整容器部署使用 gRPC 存储并配置双向 TLS。

## 检查

分别在对应子目录执行：

```sh
# backend/
go test ./...

# frontend/
npm ci
npm run build

# storage/，Linux 环境
cmake -S . -B build
cmake --build build --parallel 2
bash scripts/test-go-interoperability.sh ../backend
```

## 部署与运维

入口：[部署说明](deploy/README.md)、[文档索引](docs/README.md)。
`backend/compose.yaml` 是早期样例，不代表现有完整线上配置，不应直接覆盖生产配置。
服务器目录、证书、用户数据不随本次源码整理移动。
推送 GitHub 本身不会更新线上镜像。

## Git 与敏感文件

2026-10-15 从三个本地工作区复制当前文件，包含未提交的源码、测试和文档。
统一仓库沿用 [hualiuli158-a11y/disk](https://github.com/hualiuli158-a11y/disk) 的 `master` 分支，
保留已有 GitHub 和本地 Go 提交历史；前端与 C++ 以当前源码快照加入。
未复制子目录的 `.git`，三个原目录及其提交历史保留。

`.env`、`go.env`、证书与私钥、数据库、用户文件、依赖缓存和构建产物不提交。
允许提交脱敏的 `.env.example`；首次提交前仍需审查 `git diff --cached`。
公开仓库保留运维文档中的服务器地址与目录，不包含登录凭据。

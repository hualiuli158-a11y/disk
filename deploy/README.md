# 统一部署入口

本文提供统一源码布局下的构建路径。
现有生产 Compose 和环境配置仍在服务器 `/home/ubuntu/disk`。
本次整理未下载真实配置、证书或用户数据，也未更新生产环境。

## 从仓库根目录构建镜像

先构建前端静态资源，再构建三个镜像。以下为 Linux shell 示例：

```sh
cd frontend
npm ci
npm run build
cd ..

# 每次发布使用不同且明确的版本号
VERSION=20261015-monorepo1
docker build -t "go_docker:$VERSION" ./backend
docker build -t "disk_front:$VERSION" ./frontend
docker build -t "cpp_docker:$VERSION" ./storage
```

前端 Dockerfile 复制预先生成的 `dist/`，不能省略 `npm run build`。
`dist/` 不提交 Git。Go、C++ Dockerfile 自行编译源码。
构建需要访问基础镜像和依赖源。

## 已有配置

| 文件 | 用途 |
| --- | --- |
| [`../frontend/deploy/hualiuli-nginx.conf`](../frontend/deploy/hualiuli-nginx.conf) | 现有域名的宿主机 HTTPS 入口参考 |
| [`../frontend/nginx.conf`](../frontend/nginx.conf) | 前端镜像内静态资源与 SPA 路由 |
| [`../backend/deploy/nginx.conf`](../backend/deploy/nginx.conf) | 后端上传入口约束参考 |
| [`../backend/.env.example`](../backend/.env.example) | 后端变量说明及占位示例 |
| [`../backend/compose.yaml`](../backend/compose.yaml) | 历史样例，不是当前完整生产配置 |

`frontend/deploy/apply_frontend_compose.py` 是早期服务器的一次性迁移脚本，
假设旧部署布局和固定旧镜像版本，保留作历史参考，不应在本仓库直接执行。
不要使用历史 Compose 样例覆盖服务器配置。
下次上线应根据当前生产 Compose 准备版本更新，再检查配置、备份和发布。

## 现有生产连接关系

浏览器 HTTPS → 宿主机 Nginx：

- 页面与资源 → `127.0.0.1:8082` → 前端容器。
- `/login`、`/register`、`/api/`、`/livez`、`/readyz`
  → `127.0.0.1:8081` → Go 容器。
- Go → 私有 Docker 网络中的 MySQL 与 C++ gRPC。
- Go 与 C++ 使用挂载证书进行双向 TLS，证书不进入镜像或仓库。

服务器 SSH、巡检和回滚参考
[运维手册](../backend/docs/cloud-server-access-and-operations.md) 及
[发布报告](../backend/docs/deployment-report-2026-10-15.md)。
本地构建镜像不会自动修改线上服务。

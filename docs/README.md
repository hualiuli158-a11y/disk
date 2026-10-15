# 项目文档

- [统一目录与迁移说明](project-layout.md)
- [镜像构建与部署配置](../deploy/README.md)
- [Go / gRPC 存储接入](../backend/docs/grpc-storage.md)
- [数据库迁移](../backend/docs/migrations.md)
- [Go 仓储层测试](../backend/docs/repository-testing.md)
- [Go 服务层测试](../backend/docs/service-testing.md)
- [前端说明](../frontend/README.md)
- [前端变更记录](../frontend/docs/CHANGELOG.md)
- [C++ 存储服务说明](../storage/README.md)

## 线上运维记录

- [2026-10-15 部署报告](../backend/docs/deployment-report-2026-10-15.md)
- [HTTP 请求生命周期图](../backend/docs/images/http-request-lifecycle-2026-10-15.png)
- [发布清单](../backend/docs/deployment-manifest-2026-10-15.json)
- [服务器连接与状态检查](../backend/docs/cloud-server-access-and-operations.md)

组件文档保留在组件目录中，避免同一份说明存在多份副本。
历史报告中的原路径、镜像标签和状态属于当时快照；
新源码目录以根 README 为准，服务器状态以实时查询为准。

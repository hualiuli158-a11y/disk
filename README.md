# Disk C++

基于 MouduAsio 的网盘 C++ 后端。浏览器 HTTP 入口使用自有 TcpServer，HTTP 解析使用 Boost.Beast，元数据使用 MySQL，文件内容通过 gRPC 访问 storage/ 中的 C++ 存储节点。

## 代码组织

```text
src/
  application.*                 HTTP 路由、请求转换、JSON 响应、服务组装
  http_server.*                 连接会话、Beast 解析、背压和线程池
  http_types.*                  HTTP 请求与响应结构、JSON 响应编码
  services/
    account_service.*           账号校验、密码与令牌、用户信息和配额
    folder_service.*            目录业务、重命名与移动防循环校验、列表查询
    file_service.*              上传下载流程、重命名与移动、哈希与候选对象清理
    search_service.*            搜索文本校验、类型过滤与排序分页
    trash_service.*             回收站软删除、恢复、永久删除与底层对象清理
    query_validation.h          分页与参数范围校验辅助
  repositories/
    user_repository.*           用户与根目录的数据库操作
    folder_repository.*         目录树查询、创建、重命名与移动事务
    file_repository.*           文件元数据、去重配额、重命名与移动事务
    search_repository.*         文件名与目录名多条件搜索
    trash_repository.*          软删除、回收站树恢复、级联硬删除与引用清理
    repository_support.*        公共 SQL 排序、树遍历与用户行级锁辅助
  models.h                      服务与仓储之间的业务数据结构
  database.*                    Boost.MySQL 连接池、执行环境和事务
  storage.*                     对象存储接口、gRPC 客户端和传输队列
  auth.*                        bcrypt 与 JWT
  common.*                      输入校验、JSON 解析和字符处理
```

请求经 Application 进入 AccountService、FolderService、FileService、SearchService 或 TrashService。业务服务通过对应 Repository 访问 MySQL，Repository 返回业务数据结构，再由 Application 编码为 HTTP 响应。FileService 与 TrashService 通过 ObjectStorage 完成文件内容读写和物理对象清理。

注册与根目录创建在 UserRepository 的同一个事务中执行。上传的配额检查、内容去重、文件和初始版本记录、容量更新在 FileRepository 的同一个事务中执行。候选对象清理通过事务内回调持有存储 Key 的锁，协调数据库提交结果与对象删除。

目录移动时递归向上检查目标目录的所有父节点，杜绝将父目录移动到其自身或子目录产生循环；回收站恢复时若原父目录已失效，自动将资源恢复至根目录，若恢复位置存在同名资源返回 409 冲突；永久删除时联动扣减用户已用配额，并仅在没有任何其他文件或历史版本引用时才删除底层物理对象。

## 当前接口

| 方法 | 路径 | 功能 |
| --- | --- | --- |
| POST | /register、/login | bcrypt 账号注册和登录、签发 JWT |
| GET | /api/profile、/api/quota | 用户信息及逻辑容量 |
| GET | /api/folders/root/children | 根目录与子目录 |
| POST | /api/folders | 创建子目录 |
| GET | /api/folders/:id/children | 子目录列表 |
| GET | /api/folders/:id/contents | 文件和目录混合分页、排序 |
| PATCH | /api/folders/:id | 文件夹重命名 |
| PATCH | /api/folders/:id/move | 文件夹移动（含循环移动校验） |
| DELETE | /api/folders/:id | 文件夹递归软删除至回收站 |
| POST | /api/files | 流式 multipart 上传、SHA-256 去重及初始版本记录 |
| GET | /api/files/:id/download | 流式下载及 ETag |
| PATCH | /api/files/:id | 文件重命名 |
| PATCH | /api/files/:id/move | 文件移动 |
| DELETE | /api/files/:id | 文件软删除至回收站 |
| GET | /api/search | 文件与文件夹搜索（支持按类型、排序与分页） |
| GET | /api/trash | 回收站条目分页列表 |
| POST | /api/trash/:id/restore | 资源从回收站恢复（同名冲突校验、父目录失效自动恢复至根目录） |
| DELETE | /api/trash/:id | 永久删除（级联清理、逻辑配额返还、无引用物理对象及文件释放） |
| GET | /livez、/readyz | 存活及数据库、存储可用性检查 |

上传使用四个 64 KiB HTTP 正文块的队列，队列满时暂停 Connection 读取。下载按写完成回调继续读取下一块。每个用户最多同时接受两项上传，传输线程池和业务线程池分别运行。请求断开会终止队列等待并取消存储 RPC。

目前已完成第二优先级（日常文件管理：重命名、移动、搜索、回收站全流程）。后续里程碑为第三优先级（后台周期性孤儿对象回收、回收站自动过期清理及失败重试）与第四优先级（在线文本编辑、`If-Match` 乐观锁冲突检查、历史版本列表及版本恢复）。现有 React 前端日常文件管理所有功能均已对齐并支持全链路操作。

## 构建

需要 C++20、CMake 3.24、Python 3、Boost 1.88 以上、gRPC/Protobuf 和 OpenSSL。将 MOUDU_ASIO_ROOT 指向 MouduAsio 源码目录，默认使用相邻的 ../MouduAsio。JsonCpp、BusinessPool 和 Boost.Charconv 复用该源码树中的实现。

```powershell
./scripts/build.ps1 -Test
```

脚本默认使用与本机 gRPC 匹配的 E:/mingw64 工具链，并设置项目内的临时目录。其他位置的工具链可以通过 -CompilerRoot 指定。通用 CMake 构建可以使用 -DMOUDU_ASIO_ROOT、-DGRPC_ROOT 和 -DBoost_ROOT 指定依赖。

Windows 默认检测本机 E:/Libraries 下的 Boost 和 gRPC，也可以设置 Boost_ROOT 与 GRPC_ROOT。本机较大的 MinGW gRPC Debug 归档会在 build 内生成去调试信息副本以完成链接。

Botan 3.10.0 和 jwt-cpp 0.7.1 在首次配置时下载，联网受限时可以通过 BOTAN_SOURCE_DIR、JWT_SOURCE_DIR 指定解压后的源码。流式 multipart 解析器来自 iafonov/multipart-parser-c 的 772639cf10db6d9f5a655ee9b7eb20b815fab396 版本，随源码保留原始许可证声明。

## 启动

在专用 MySQL 数据库中依次执行 migrations/ 下的两份 up.sql。已有 Go 数据库已经应用对应迁移时，可以直接复用表结构。

将 .env.example 中的值配置到进程环境。程序从环境变量读取参数。然后启动存储节点和 API。

```powershell
./build/mingw-debug/disk_api.exe
```

存储节点的 Linux 构建与证书配置见 storage/README.md。开发环境可以将存储节点绑定到本机回环地址，跨机器存储连接使用 TLS 配置。

```sh
cmake -S storage -B build/storage
cmake --build build/storage --parallel 2
./build/storage/grpc_storage_node 127.0.0.1:50051 ./data/objects
```

前端联调使用已有的集成模式，Vite 默认代理到 127.0.0.1:8080。

```sh
cd frontend
npm ci
npm run dev:api
```

## 验证

CTest 的传输测试覆盖连接复用、定长/分块/并发上传、32 MiB 下载、非法 HTTP、认证前置和中断上传。

在已迁移的独立测试数据库和运行中的存储节点上，设置 DB_*、JWT_*、STORAGE_ADDRESS、PORT 后运行真实接口测试。该测试创建随机名称的用户和测试文件，使用专用测试库与对象目录。

设置 MYSQL_CLIENT 为 mysql 客户端路径时，测试还会直接核验去重记录、bcrypt 跨库兼容以及并发配额，需安装 Python bcrypt 包。

```powershell
python tests/api_integration.py build/mingw-debug/disk_api.exe
```

frontend/、storage/ 和 migrations/ 取自 hualiuli158-a11y/disk 的 master 源码快照。C++ API 实现位于 src/。

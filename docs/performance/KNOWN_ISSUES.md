# 本次性能测试发现的问题

后续已完成本地修复和专项复测，见 [修复说明与对比数据](2026-11-19-fixes.md)。以下保留初始基线采集时的问题证据。

## 相同内容上传与永久删除之间的 MySQL 死锁

适用源码：Disk `438c7b06ab8414969746240c8aef52aff2e8b9e2`，分支 `cpp-backend-gin`。

短时验证使用四个账号、四个并发工作线程，反复上传相同的 1 MiB 内容，每次成功上传后立即软删除、查询回收站并永久删除。部分上传返回 HTTP 503，响应为 `Dependency unavailable`。

专用测试库的 `SHOW ENGINE INNODB STATUS` 记录了真实死锁：

- 上传事务执行 `INSERT INTO physical_files ... ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`，已持有 SHA-256 唯一索引记录的锁，等待物理文件主键记录锁。
- 永久删除事务已持有同一物理文件的主键记录锁，在删除记录时等待 SHA-256 唯一索引记录的锁。
- MySQL 回滚其中一个事务，API 将该依赖异常映射为 503。

这是同内容上传与同步清理混合执行时的争用场景。该结果不能解释成单独上传流程必然失败，也不能将失败尝试从性能结果里删除。正式报告同时统计成功吞吐、上传失败率、清理失败数，并在负载结束后核验配额和物理对象一致性。

相关实现：[上传物理文件去重](https://github.com/hualiuli158-a11y/disk/blob/cpp-backend-gin/src/repositories/file_repository.cpp)、[永久删除物理文件记录](https://github.com/hualiuli158-a11y/disk/blob/cpp-backend-gin/src/repositories/trash_repository.cpp)。

后续修复应验证统一物理对象加锁顺序，或针对可确认回滚的死锁实施有界事务重试。重试范围应覆盖元数据事务，并重新检查配额、引用与目录状态；直接重传文件内容会掩盖原始失败并增加负载。

复现入口：

```powershell
./tests/performance/baseline.ps1 -DiskSource E:/Clion/disk_gin_cpp -GinSource 'E:/Clion/gin_ cpp' -SkipBuild -Duration 10 -Warmup 2 -Repeats 3
```

初始基线采集时，此问题尚未修复。后续修复后的结果另建一轮数据，原始失败记录保持不变。

## 请求头阶段拒绝上传时的间歇性连接重置

同一组二进制在一次完整业务回归中通过了非法声明大小的前置拒绝检查，但在随后的一次复测中，`early_upload(folder, -1, 400)` 读取响应时出现 `ConnectionResetError: WinError 10054`，没有收到期望的 HTTP 400。

该测试在发出请求头后继续发送部分正文，而服务端会在请求头阶段提前拒绝。当前证据只确认该交互存在间歇性连接重置，尚未证明具体原因，不能将单次回归通过解释为此边界已稳定。后续需要对提前返回响应、未读正文、写完成与连接关闭的先后顺序做专项回归。

正式 HTTP 微基准阶段在这次业务回归失败之前已完整采集。API 和文件性能另建输出目录，显式使用 `--skip-integration` 继续采样，保留之前的完整回归通过记录和这次失败记录；性能结果不据此声称全部业务边界已稳定。

## 测试入口适配

以下调整属于测试与构建入口，未修改两个目标项目的业务或框架源码：

- 独立 gin_cpp 基准使用与 Disk 相同的 MinGW GCC 16 兼容编译定义 `_GLIBCXX_ATOMIC_WORD_BUILTINS=1`。初版独立基准没有沿用该定义，在并发阶段退出；统一选项后，短时并发验证通过。
- 原业务测试将响应头放入普通字典后读取 `ETag`，无法适配框架发送的 `etag`。测试副本使用大小写不敏感的头集合，保留原断言。
- 原业务测试把服务日志写入 PIPE，并在测试结束后才读取。启用逐请求 Logger 后，管道填满会阻塞服务线程。测试副本将日志持续重定向到本轮日志文件。
- 记录测试原始脚本与执行副本 SHA-256，并保留全部业务断言。详情见每轮的 `integration-adaptation.json`。

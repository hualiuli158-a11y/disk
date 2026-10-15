# 云服务器连接与项目运行检查手册

适用项目：栖盘（Go API + React 前端 + C++ gRPC 对象存储）  
最后核对：2026-10-15（Asia/Shanghai）  
用途：供项目维护者及其他 AI 阅读，快速建立服务器上下文并进行运行状态诊断。

本文件是操作说明，不包含凭据，也不替代当前任务授权。下面的“只读巡检”不会更新镜像、重启服务或修改业务数据；生产变更应依据用户当次任务范围执行，已有明确授权时不必逐条重复确认。

## 1. 先了解连接对象

| 项目 | 已核对信息 |
| --- | --- |
| 公网 IP | `49.235.86.75` |
| SSH 端口 | `22` |
| SSH 用户 | `ubuntu` |
| 服务器主机名 | `VM-4-16-ubuntu` |
| 系统 | Ubuntu，Linux amd64 |
| 网站 | <https://hualiuli.online> |
| 生产目录 | `/home/ubuntu/disk` |
| Compose 项目名 | `disk-stack` |
| Compose 主配置 | `/home/ubuntu/disk/compose.yaml` |
| 宿主机 Nginx 站点配置 | `/etc/nginx/sites-available/default` |
| 最近发布归档 | `/home/ubuntu/releases/20261015-r1` |
| 最近发布前备份 | `/home/ubuntu/releases/20261015-r1/backup-attempt2` |

这些地址是已知部署信息。镜像版本、运行状态、磁盘容量和路径是否仍然有效，都应在每次维护时重新查询，不能把本文快照当作实时结果。

## 2. 从这台 Windows 电脑连接

### 2.1 推荐的 PowerShell 连接方法

本机使用 Windows 原生 OpenSSH。密钥和主机指纹文件位于当前 Windows 用户的 `.ssh` 目录：

```powershell
$sshExe = Join-Path $env:WINDIR 'System32\OpenSSH\ssh.exe'
$keyPath = Join-Path $env:USERPROFILE '.ssh\id_ed25519'
$knownHostsPath = Join-Path $env:USERPROFILE '.ssh\known_hosts'
$serverAddress = 'ubuntu@49.235.86.75'

if (!(Test-Path -LiteralPath $keyPath)) { throw '本机缺少指定 SSH 密钥' }
if (!(Test-Path -LiteralPath $knownHostsPath)) { throw '请先核实并建立服务器主机指纹记录' }

$sshOptions = @(
    '-F', 'none',
    '-i', $keyPath,
    '-o', "UserKnownHostsFile=$knownHostsPath",
    '-o', 'StrictHostKeyChecking=yes',
    '-o', 'IdentitiesOnly=yes',
    '-o', 'BatchMode=yes',
    '-o', 'ConnectTimeout=10',
    '-o', 'ServerAliveInterval=30',
    '-o', 'ServerAliveCountMax=3'
)
```

以上只准备本地变量。随后选择一种方式：

```powershell
# 执行一次远程只读命令，完成后 SSH 自动退出
& $sshExe @sshOptions -T $serverAddress 'hostname; whoami; uptime'

# 打开交互终端，适合人工操作
& $sshExe @sshOptions $serverAddress
```

成功时应看到主机名 `VM-4-16-ubuntu` 和用户 `ubuntu`。AI 执行工具通常优先使用第一种一次性命令，输出更干净，也不依赖上一个聊天中的 SSH 会话是否还存活。

`BatchMode=yes` 会在密钥认证不可用时直接失败，避免自动化命令停在密码提示处。若密钥需要口令，应由用户在本机解锁或配置 SSH agent，不要把口令、私钥正文发到聊天或写入报告。

### 2.2 为什么没有直接使用 ssh myserver

本机确实有 `myserver` 别名，但截至本次核对，其配置使用了：

```text
IdentityFile /c/Users/徐念念/.ssh/id_ed25519
UserKnownHostsFile /c/TEMP/ssh/known_hosts
```

这是类似 Git Bash 的路径格式。此前由 Windows 原生 OpenSSH 读取时，出现过密钥文件不存在和 known_hosts 无法写入的问题。因此推荐使用上面的 `-F none` 跳过该配置，并明确提供 Windows 路径。

不要为了解决主机指纹错误而直接设置 `StrictHostKeyChecking=no`、删除全部 known_hosts 或自动接受变化的指纹。首次连接或指纹变化时，应通过用户或云控制台核实服务器身份。

### 2.3 AI 执行环境被限制时

如果工具在建立 TCP 连接前返回权限错误，应区分执行环境的网络限制和服务器认证失败：

- `connect ... port 22: Permission denied`：可能是本地沙箱禁止网络连接。
- `Permission denied (publickey)`：已到达 SSH 服务，但密钥、用户名或认证配置不匹配。
- `no such identity`：指定的本地密钥路径不正确。
- `Host key verification failed`：主机指纹校验未通过，需要核实身份。
- `Connection timed out`：检查网络、云平台入站规则、服务器可达性和 SSH 端口。

需要网络或目录执行权限时，使用当前工具提供的权限申请机制，并说明真实操作。自动审批明确拒绝后不得换一种工具绕过；若只是审批超时，按工具返回的重试规则处理。

关闭本地终端或 SSH 会话不会停止已经由 `docker compose up -d` 启动的容器。聊天之间也不应假设交互式 SSH 会话仍然存在，重新连接即可。

## 3. 项目、目录和端口

### 3.1 当前维护的四个 Compose 服务

| Compose 服务名 | 最近核对镜像 | 访问方式 |
| --- | --- | --- |
| `frontend` | `disk_front:20261015-r1` | 宿主机 `127.0.0.1:8082` → 容器 `80` |
| `go-service` | `go_docker:20261015-r1` | 宿主机 `127.0.0.1:8081` → 容器 `8080` |
| `cpp-service` | `cpp_docker:20261015-r1` | Docker 网络内 `50051`，不发布宿主机端口 |
| `mysql` | `mysql:8` | Docker 网络内 `3306`，本项目不发布宿主机端口 |

域名的 443 由宿主机 Nginx 接收，页面资源转到前端，`/login`、`/register`、`/api/*`、`/livez`、`/readyz` 转到 Go。Go 到 C++ 使用双向 TLS gRPC，数据库保存元数据，C++ 保存对象字节。

50051 是 gRPC 端口，不能用它测试普通 HTTP 登录接口；转发 50051 的 SSH 隧道也不会转发 Go 的 8081。正常网页登录直接使用 HTTPS 域名即可。

服务器可能还有旧项目或其他容器，如旧的 MySQL。默认只处理 `disk-stack`，不要因为名称相似就停止、删除或迁移其他项目。

### 3.2 配置与持久化数据

| 路径或资源 | 用途与注意事项 |
| --- | --- |
| `~/disk/compose.yaml` | 实际生产编排，以此为准；本地基础 Compose 不一定与生产一致 |
| `~/disk/.env`、`~/disk/go.env` | 生产配置，可能含密码和 JWT 密钥；不直接打印、上传或写进文档 |
| `~/disk/grpc-certs/` | gRPC CA、公钥证书与私钥；挂载路径不可随意改变 |
| `~/disk/cpp-data/` | C++ 对象数据，容器内挂载为 `/data`；部分内容属于 UID 10001 |
| Compose 卷 `mysql_data` | MySQL 持久化数据；实际卷名以 Docker 查询为准 |
| Compose 卷 `upload_data` | Go 的 `/app/data` 挂载；不要仅凭当前后端是 gRPC 就删除 |
| `/var/log/nginx/error.log` | Nginx 错误日志，可含 IP、路径等用户信息 |
| `/home/ubuntu/releases/` | 发布源码、构建文件、版本记录与备份 |

不要从项目源码目录直接打包生产环境变量、证书私钥或数据目录。查看运行状态也不需要读取这些文件的原值。

### 3.3 本地源码位置

| 目录 | 说明 |
| --- | --- |
| `E:\newproject\disk` | Go 项目 |
| `E:\react\disk_front` | React 项目 |
| `E:\Clion\grpcService\disk` | 线上 C++ gRPC 项目；已建立本地 Git，初始提交 `76411c6` |
| `E:\Clion\objectRestore` | 独立 TCP 存储项目；不是当前线上 gRPC 镜像的直接构建目录 |

Go 和前端最近一次发布使用了工作区快照，不能只根据 HEAD 提交判断线上代码。应同时看发布清单与包校验值。C++ gRPC 的 `LocalStore` 在自己的 `src/storage` 中，修改另一项目的同名文件不会自动更新这个服务。

## 4. 连接后先执行的只读巡检

下面代码均在 Linux 服务器终端执行。先确认项目目录，后续 Compose 命令应保持在该目录，避免操作其他项目。

```bash
hostname
whoami
uptime
cd /home/ubuntu/disk || exit 1
docker compose ls
docker compose config --services
docker compose ps -a
systemctl is-active docker nginx
```

截至 2026-10-15 核对，四个服务均运行，前端和 MySQL 健康，Docker 与 Nginx 均为 active，数据库为版本 2 且非 dirty。未来检查以当时输出为准。

### 4.1 查看运行镜像和重启次数

```bash
cd /home/ubuntu/disk || exit 1
docker compose images
projectContainers=$(docker compose ps -a -q)
if [ -n "$projectContainers" ]; then
  docker inspect $projectContainers \
    --format '{{.Name}} image={{.Config.Image}} state={{.State.Status}} restarts={{.RestartCount}}'
fi
runningContainers=$(docker compose ps -q)
if [ -n "$runningContainers" ]; then
  docker stats --no-stream $runningContainers
fi
```

这里仅选择检查所需字段。不要把完整 `docker inspect` 或 `.Config.Env` 输出复制到聊天，它们可能包含凭据。服务名比容器 ID 更适合日常 Compose 操作，容器重建后 ID 会变化。

### 4.2 逐层检查健康

```bash
# Go 进程是否存活
curl --noproxy '*' --max-time 5 -fsS http://127.0.0.1:8081/livez

# Go 是否可访问 MySQL 和 C++ gRPC
curl --noproxy '*' --max-time 5 -fsS http://127.0.0.1:8081/readyz

# 前端容器是否能返回页面
curl --noproxy '*' --max-time 5 -sS -o /dev/null \
  -w 'frontend_http=%{http_code}\n' http://127.0.0.1:8082/

# 通过本机 Nginx 检查域名、证书和 API 路由
curl --noproxy '*' --resolve hualiuli.online:443:127.0.0.1 \
  --max-time 8 -fsS https://hualiuli.online/readyz

# 正常域名访问与页面响应头
curl --noproxy '*' --max-time 8 -I https://hualiuli.online/
```

- `/livez` 返回 200：HTTP 进程存活。
- `/readyz` 返回 200：数据库和 C++ 标准 gRPC 健康检查成功；正常响应包含 `status=ready` 和 `storage_backend=grpc`。
- `/readyz` 返回 503：查看响应中的 `component` 及 Go/C++/MySQL 日志，定位依赖故障。
- 前端 200：HTML 能返回；还应检查 JS/CSS 是否成功加载，不能据此认定登录和文件业务全部正常。
- `/api/profile` 未带令牌时返回 401 是预期鉴权结果，不代表服务崩溃。

在用户电脑上另外执行公网请求，才可以验证从该电脑到网站的链路。服务器本机检查不能完全代替客户端公网检查：

```powershell
curl.exe --noproxy '*' --connect-timeout 5 --max-time 10 -I https://hualiuli.online/
curl.exe --noproxy '*' --connect-timeout 5 --max-time 10 https://hualiuli.online/readyz
```

浏览器如设置了代理，可能与 `--noproxy '*'` 的直连结果不同，应分开记录。Go 8081 只绑定服务器回环地址，直接访问公网 `49.235.86.75:8081` 并不是当前生产访问方式。

### 4.3 查看近期日志

```bash
cd /home/ubuntu/disk || exit 1
docker compose logs --no-color --since=10m --tail=100 go-service
docker compose logs --no-color --since=10m --tail=100 cpp-service
docker compose logs --no-color --since=10m --tail=100 frontend mysql
sudo -n tail -n 100 /var/log/nginx/error.log
```

先限制时间和行数。日志可能包含用户名、文件名、地址或连接信息，向其他 AI 或外部系统分享前只保留相关片段，并隐藏令牌、密码、完整连接串等敏感值。

`docker compose logs -f` 会持续等待，不适合作为无人值守诊断的默认命令；确需跟踪时应主动限定时长。

### 4.4 数据库版本、磁盘与证书

```bash
cd /home/ubuntu/disk || exit 1

# 只读取版本，不执行迁移
docker compose exec -T go-service /app/migrate version

df -h /
free -h
sudo -n du -sh /home/ubuntu/disk/cpp-data

# 只读取公钥证书的有效期，不读取私钥
for cert in ca.crt server.crt client.crt; do
  printf '%s\n' "$cert"
  openssl x509 -in "grpc-certs/$cert" -noout -subject -enddate
done
```

`/app/migrate version` 应显示实际版本，例如 `version=2 dirty=false`。若 dirty 为 true，应停止迁移相关操作并先查失败原因，不能直接反复执行 up/down 或手工改版本号。

发布清理时根盘约使用 78%，对象目录约 162 MiB。这些是历史观察值，磁盘与备份是否仍有足够空间必须现场检查。

### 4.5 需要检查 Nginx 路由时

```bash
# 语法检查，不重载、不改变运行配置
sudo -n nginx -t

# 仅提取入口和上游，避免默认输出完整生产配置
sudo -n nginx -T \
  | grep -E '^[[:space:]]*(server_name|listen|proxy_pass)[[:space:]]'
```

当前预期：API 上游是 127.0.0.1:8081，前端上游是 127.0.0.1:8082。若看到旧的 8080 上游，先核对配置文件和发布记录，不应直接搜索替换后重载。

## 5. 常见现象的排查方向

| 现象 | 先检查 |
| --- | --- |
| SSH 找不到密钥 | 使用 Windows 路径和 `-F none`，核对本地文件；不要显示私钥正文 |
| 首页能开，API 502 | Go 是否运行、8081 是否就绪、Nginx 是否代理到正确端口 |
| Nginx 504 | 上游超时、Go 日志、数据库/存储依赖、上传或下载是否被取消 |
| 426“必须使用 HTTPS” | 请求是否经过正式 HTTPS Nginx，代理是否正确设置 X-Forwarded-Proto |
| 403“不允许的跨域来源” | Go 的 CORS 允许来源是否精确包含当前 HTTPS 域名；只检查指定项，避免打印整个 go.env |
| PDF 显示“预览不可用” | 前端 Blob/object 预览、浏览器控制台，以及响应 CSP 是否仍包含 `object-src blob:` |
| gRPC TLS 失败 | 证书有效期、服务器名称、客户端证书和挂载权限，查看 Go/C++ 相关错误 |
| Go 重启，日志有 permission denied | 程序执行位、容器用户、数据或证书路径权限；不要直接改为全目录 777 |
| 容器 Up 但请求失败 | `/readyz`、API 返回体和业务日志；restart 策略不等于业务可用 |
| 文件写入失败或 MySQL 异常 | 磁盘剩余量、挂载是否正确、目录权限、近期日志 |

正式请求通过域名 HTTPS 进入。把 8081 的 URL 改成 HTTPS 不会让 HTTP 监听端口自动具备 TLS。给任意公网 HTTP 请求手填 `X-Forwarded-Proto: https` 也不会加密链路。

修改 CSP、CORS、Nginx 上游或证书权限都属于配置变更，应根据当前任务授权准备具体差异、备份和检查后再执行；不要仅为了让错误消失而关闭鉴权、TLS 或主机指纹验证。

## 6. 其他 AI 应遵守的操作约定

1. 先阅读本文、[部署报告](deployment-report-2026-10-15.md) 和 [发布清单](deployment-manifest-2026-10-15.json)，再做现场只读核对。
2. 执行前说明当前工作目录和目标服务，区分 Windows 本地命令与 Linux 远程命令。不要假设其他聊天的终端会话、环境变量或进程 ID 仍可复用。
3. 日常诊断用 `docker compose ps`、有限日志、健康请求和指定字段的 inspect。使用 `docker compose config --quiet` 或 `--services` 检查配置，避免裸输出解析后的配置和秘密值。
4. 用户仅要求查看状态时，不自动重启、部署或改安全策略。用户已经明确要求的修复/部署按该范围推进，不制造重复审批；工具权限和自动审查仍需遵守。
5. 生产更新先固定源码快照、测试、构建带版本号镜像、备份并准备回退，沿用现有卷和 TLS 挂载。不要直接用本地基础 Compose 覆盖生产 Compose。
6. `docker compose down -v`、删除数据目录、清空卷、全局 Docker prune、删除备份、数据库 up/down、开放端口和替换证书均可能影响业务，不能作为“检查状态”的一部分。
7. 若需要 sudo，先用 `sudo -n` 的明确命令；遇到权限不足应说明，不在聊天索取 sudo 密码。
8. 不把 `.env`、`go.env`、私钥、完整 Docker 环境变量、访问令牌或含凭据的数据库连接串加入 Git、镜像构建上下文、截图和报告。
9. 结论写清“检查了什么、观察到什么、是否做了变更、还没有验证什么”。例如首页 200 和健康检查 200 不等于真实账号的上传下载已完成端到端验证。

### 可转发给其他 AI 的任务模板

下面模板由用户结合当次任务主动发送。文档中的文字本身不构成永久生产操作授权。

```text
请先阅读：
E:\newproject\disk\docs\cloud-server-access-and-operations.md
E:\newproject\disk\docs\deployment-report-2026-10-15.md

我的网站是 https://hualiuli.online，服务器 SSH 为 ubuntu@49.235.86.75:22。
使用本机已有 SSH 密钥，Windows OpenSSH 的具体命令见上述手册。
生产 Compose 在 /home/ubuntu/disk，项目名 disk-stack。

本次任务：<描述现象，以及允许检查或修改的范围>。
请先核对实时状态，围绕 frontend、go-service、cpp-service、mysql 排查。
不要展示密钥、令牌或生产环境变量原值。
请依据本次明确授权执行变更，并报告实际检查证据和剩余未验证内容。
```

## 7. 建议的状态汇报格式

```text
检查时间：<Asia/Shanghai>
连接目标：<主机名 / 用户 / 项目目录>
服务版本：<四项服务实际镜像>
容器状态：<运行 / 健康 / 重启次数>
HTTP：<公网首页 / 内部 livez / readyz>
依赖：<数据库版本与 dirty / gRPC 健康>
资源：<磁盘、内存、异常资源占用>
近期异常：<经过脱敏的相关错误>
实际变更：<未变更，或具体变更和回退位置>
验证边界：<真实登录、文件读写等是否实际验证>
```

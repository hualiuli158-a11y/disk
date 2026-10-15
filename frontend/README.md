# 栖盘文件工作空间

浏览器端 React + TypeScript + Vite 页面，使用 Hana 的暖纸视觉风格，连接 `../backend/` Go HTTP 服务。没有 Electron 或 AI 依赖。

## 本地启动

只看前端界面，无须配置文件、登录或启动后端：

```powershell
cd E:\Maintable\frontend
npm ci
npm run dev
```

开发服务器默认进入离线预览；目录、搜索、上传、预览和编辑使用内存中的演示数据，刷新页面会重置。页面右上角显示“离线预览”。

需要联调真实接口时，先按 Go 服务自己的 `.env.example` 配置 MySQL、JWT 和存储后端。新数据库先迁移，已有数据库按 Go 项目 `docs/migrations.md` 的说明处理，然后启动服务：

```powershell
cd E:\Maintable\backend
go run ./cmd/migrate up
go run .
```

另开终端运行 `npm run dev:api`，此时使用真实登录页和后端数据。Go 服务默认地址是 `http://127.0.0.1:8080`；如果服务使用其他地址，可在启动前设置 `DISK_API_TARGET`，例如 `$env:DISK_API_TARGET='http://127.0.0.1:9000'`。Vite 会代理 `/login`、`/register` 和 `/api`。正式构建始终使用真实登录和接口。

生产部署时，把这三个路径反向代理到 Go 服务，或构建前设置 `VITE_API_BASE_URL`。前者更适合保持同源访问。

## Docker 生产部署

前端镜像只包含构建后的静态文件和 Nginx。先运行 npm ci、npm run build，再用 docker build -t disk_front:<发布版本> . 制作镜像。镜像中的 Nginx 使用 nginx.conf，支持单页应用的路径回退。

服务器的 Compose 将前端容器映射到 127.0.0.1:8082，将 Go 容器映射到 127.0.0.1:8081。统一构建与部署入口见 ../deploy/README.md；主机 Nginx 配置参考 deploy/hualiuli-nginx.conf：HTTPS 的网页请求转发到前端，/login、/register、/api/ 转发到 Go，并由 Nginx 设置 X-Forwarded-Proto。Go 的 go.env 需要设置 CORS_ALLOWED_ORIGINS=https://hualiuli.online，供浏览器同源请求使用。首次把前端服务加入服务器 Compose 时使用过 deploy/apply_frontend_compose.py；后续发布应更新 Compose 中的镜像版本并重建前端容器。

发布后检查网站首页、JS/CSS 资源、/readyz，以及 /login 能收到应用响应。Go 和 C++ 的数据目录、证书和数据库不包含在前端镜像中。

## 已接入

登录、注册、目录树、分页浏览、服务端排序、按文件或文件夹筛选搜索、文件选择与拖放上传（显示传输字节、批次进度、服务器处理和列表同步状态，支持取消与查看失败原因）、下载、内联创建和重命名、多选、右键菜单、文件与文件夹拖放移动、删除到回收站、到期清理提示、恢复、永久删除与配额显示。访问令牌仅保存在当前浏览器会话的 `sessionStorage`。操作错误提示和上传结束（成功、失败或取消）面板在 5 秒后自动关闭；悬停或焦点位于提示内时暂停计时，上传过程中保留进度显示。

预览层统一识别 Markdown、代码与纯文本、CSV/TSV、常见图片、视频、音频和 PDF；文本会先做二进制探测，媒体对象 URL 会在切换文件时释放，请求也会随预览切换取消。图片支持全屏、缩放、拖动、同目录媒体前后切换与键盘操作。不同类型采用独立体积上限，超限或浏览器无法解码时会明确降级为下载。Office 文档和压缩包当前只提供下载，等待后端转换或流式内容接口。

进入文件区或选择普通文件夹时中央显示空预览；点击“我的空间”或根面包屑会返回根目录管理列表，并同步目录选中状态；左侧目录同时展示文件与文件夹，点击文件后在中央标签页预览。Markdown 和 TXT 等纯文本文件默认进入阅读态，可主动切换编辑；纯文本保持原始字符，不应用 Markdown 格式，并支持行号、查找、自动保存与手动版本快照；采用多标签、900ms 防抖自动保存、`Ctrl/Cmd + S` 手动创建版本快照、文内查找、章节轨道、阅读位置恢复、格式工具条、任务列表和代码块复制。保存请求携带当前 SHA-256 版本，后端使用 `If-Match` 原子检测并发修改；发生冲突时保留本地草稿，用户可以复制草稿或明确放弃后重新加载。切换标签时草稿仍保留，关闭含未保存修改的标签或刷新页面前会提示。侧栏按展开的文件夹加载目录和文件，并读取后续分页；搜索和管理列表按后端分页。Markdown 阅读支持代码高亮、语言标记、复制、表格、任务列表、公式和脚注，渲染结果经 DOMPurify 清理。代码预览支持行号、换行与复制；PDF 使用浏览器内嵌查看器占满剩余高度。

文件工具栏提供版本历史抽屉。900ms 自动保存只更新当前内容，不产生历史版本；Ctrl/Cmd+S 才创建编辑快照，已经自动保存的内容也能建快照，相同内容连续手动保存不会重复建版本。保存进行中按 Ctrl/Cmd+S 会排队等待并沿用最新版本令牌。初始上传和历史恢复保留独立记录。恢复时已有快照仍保留，尚未创建快照的当前内容可能被覆盖。接口 `PUT /api/files/:id/content` 默认只更新内容，仅 `create_version=true` 创建快照；仍使用 `If-Match` 校验并发修改。前后端须一起更新。

左下角“管理文件”打开紧凑的文件管理页，保留上传、新建、多选和右键操作。详情栏默认收起，右上角“沉浸阅读”可隐藏两侧面板。工作区目录栏和详情栏可拖动调宽，最左侧“设置”入口提供宽度滑块与恢复默认；宽度保存在当前浏览器，窄屏会优先为文件区让出空间。逐轮改动见 `docs/CHANGELOG.md`。

Markdown 编辑态已移植专用高亮、语法装饰、格式切换命令、任务框、代码块工具栏、公式和可编辑表格；底部字数浮条已移除，查找入口在顶部工具栏及 Ctrl/Cmd+F。预览/编辑切换保留编辑器与撤销记录。来源及浏览器适配说明见 `docs/third-party/openhanako.md`。即时排版、语法标记隐藏与保留光标位置的文档同步参考并改编自 [OpenHanako PreviewPanel / PreviewEditor](https://github.com/liliMozi/openhanako/tree/main/desktop/src/react/components)，原项目采用 [Apache License 2.0](https://github.com/liliMozi/openhanako/blob/main/LICENSE)。

运行 `npm run build` 检查类型和生产构建。运行 `npm run test:e2e` 使用本机 Chrome 验证目录、Markdown、代码、PDF、管理页及手机布局；测试使用本地离线演示数据。

## 前端目录

`src/app` 只负责页面级状态与功能协调；`src/stores` 使用 Zustand 按预览、导航和反馈切片管理跨组件状态；`src/features` 按登录、目录树、文件列表、预览和详情面板组织。预览域继续拆分为加载器、标签栏、文档工具栏、版本历史、类型渲染器、Markdown 工作区/编辑器、Markdown 辅助控件和媒体查看器；`src/components` 存放通用界面组件；`src/services` 封装后端接口；`src/styles` 保存全局样式与 Hana 主题覆盖。`src/demo` 仅保留未接入当前页面的旧版演示数据。

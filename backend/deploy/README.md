# 上传入口部署约束

`nginx.conf` 把请求总大小限制为 1 GiB 文件加 1 MiB multipart 开销，并关闭请求体代理缓冲。应用使用流式 `MultipartReader`，不会调用 `ParseMultipartForm` 或 `FormFile`，因此不会生成 Go multipart 临时文件。

仍应把 Nginx 的 `/var/cache/nginx/client_temp` 挂载为独立的限额文件系统。容器部署建议使用 64 MiB tmpfs：

```yaml
tmpfs:
  - /var/cache/nginx/client_temp:size=64m,mode=0700,noexec,nosuid,nodev
```

应用容器的系统临时目录也应使用独立 64 MiB tmpfs，避免未来引入的其他功能占满根盘：

```yaml
environment:
  TMPDIR: /var/lib/disk-upload-tmp
tmpfs:
  - /var/lib/disk-upload-tmp:size=64m,mode=0700,noexec,nosuid,nodev
```

客户端应在上传请求中发送 `X-Upload-Size`（文件字节数）。应用会在读取 multipart 前检查单文件上限和用户剩余额度，并在上传结束时校验实际大小与声明一致。`folder_id` 字段必须位于 `file` part 之前。

公网入口只开放 Nginx 的 443；80 端口只做 308 HTTPS 跳转。Go 服务保持监听 `127.0.0.1` 或 RFC1918 私网地址，生产模式会拒绝 `0.0.0.0`、`::` 和公网地址。Nginx 证书及 JWT 密钥都通过 `/run/secrets` 挂载，不能写入镜像或仓库。

生产环境访问令牌最长 15 分钟。本项目未实现 refresh token；需要长期登录时，应先增加服务端持久化的轮换 refresh token、token family 重用检测和用户/设备级撤销，再延长会话，不能延长当前 access token。

Swagger 在生产环境默认关闭。若确需开启，必须同时配置 `SWAGGER_USERNAME` 和至少 16 字符的 `SWAGGER_PASSWORD`。CORS 使用 `CORS_ALLOWED_ORIGINS` 中的精确 HTTPS 来源，不支持 `*`。

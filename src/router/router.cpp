#include "router.h"
#include "swagger.h"
#include "middleware/jwt.h"
#include "middleware/errors.h"
#include "middleware/upload_limits.h"
#include "handlers/auth_handler.h"
#include "handlers/user_handler.h"
#include "handlers/folder_handler.h"
#include "handlers/file_handler.h"
#include "handlers/search_handler.h"
#include "handlers/trash_handler.h"

namespace disk {

std::unique_ptr<gin::Engine> SetupRouter(AppContext& app, const Config& config)
{
    auto engine = std::make_unique<gin::Engine>();
    engine->Use(gin::Logger(), middleware::Errors());
    engine->SetThreadCount(config.ioThreads);
    gin::Limits limits;
    limits.max_body_bytes = config.maxUploadBytes + 65536;
    engine->SetLimits(limits);
    gin::ServerOptions options;
    options.request_timeout = config.requestTimeout;
    options.body_idle_timeout = config.uploadIdleTimeout;
    options.max_buffered_body_bytes = 65536;
    options.cors_origin = config.corsOrigin;
    options.cors_allowed_headers = "Authorization,Content-Type,If-Match,X-Upload-Size";
    options.cors_exposed_headers = "ETag,X-File-SHA256,Last-Modified,Content-Disposition";
    options.response_headers = {{"X-Content-Type-Options", "nosniff"}, {"Cache-Control", "no-store"}};
    options.worker_pools["uploads"] = {4, 64};
    options.worker_pools["health"] = {1, 4};
    engine->SetServerOptions(std::move(options));

    if (config.swaggerEnabled)
        RegisterSwagger(*engine);

    auto authHandler = std::make_shared<handler::AuthHandler>(app.accounts);
    auto userHandler = std::make_shared<handler::UserHandler>(app.accounts);
    auto folderHandler = std::make_shared<handler::FolderHandler>(app.folders, app.trash);
    auto fileHandler = std::make_shared<handler::FileHandler>(app.files, app.trash);
    auto searchHandler = std::make_shared<handler::SearchHandler>(app.search);
    auto trashHandler = std::make_shared<handler::TrashHandler>(app.trash);

    // 1. 系统健康探针
    engine->GET("/livez", [](gin::Context& c) {
        c.JSON(200, gin::H{{"status", "alive"}});
    });

    engine->GET("/readyz", [&app](gin::Context& c) {
        app.database.Check();
        app.storage.Check();
        c.JSON(200, gin::H{{"status", "ready"}});
    });

    // 2. 公开认证接口
    engine->POST("/register", [authHandler](gin::Context& c) {
        authHandler->Register(c);
    });

    engine->POST("/login", [authHandler](gin::Context& c) {
        authHandler->Login(c);
    });

    // 3. 需 JWT 鉴权的业务接口分组 (/api)
    auto api = engine->Group("/api");
    api.Use(middleware::JWT(app.auth));
    {
        // 用户信息与配额
        api.GET("/profile", [userHandler](gin::Context& c) {
            userHandler->Profile(c);
        });

        api.GET("/quota", [userHandler](gin::Context& c) {
            userHandler->Quota(c);
        });

        // 文件夹操作
        api.GET("/folders/root/children", [folderHandler](gin::Context& c) {
            folderHandler->Root(c);
        });

        api.POST("/folders", [folderHandler](gin::Context& c) {
            folderHandler->Create(c);
        });

        api.GET("/folders/:id/contents", [folderHandler](gin::Context& c) {
            folderHandler->Contents(c);
        });

        api.GET("/folders/:id/children", [folderHandler](gin::Context& c) {
            folderHandler->Children(c);
        });

        api.PATCH("/folders/:id", [folderHandler](gin::Context& c) {
            folderHandler->Rename(c);
        });

        api.PATCH("/folders/:id/move", [folderHandler](gin::Context& c) {
            folderHandler->Move(c);
        });

        api.Delete("/folders/:id", [folderHandler](gin::Context& c) {
            folderHandler->Delete(c);
        });

        // 文件操作
        api.POST("/files", [fileHandler](gin::Context& c) {
            fileHandler->Upload(c);
        });

        api.GET("/files/:id/download", [fileHandler](gin::Context& c) {
            fileHandler->Download(c);
        });

        api.PATCH("/files/:id", [fileHandler](gin::Context& c) {
            fileHandler->Rename(c);
        });

        api.PATCH("/files/:id/move", [fileHandler](gin::Context& c) {
            fileHandler->Move(c);
        });

        api.Delete("/files/:id", [fileHandler](gin::Context& c) {
            fileHandler->Delete(c);
        });

        // 搜索操作
        api.GET("/search", [searchHandler](gin::Context& c) {
            searchHandler->Search(c);
        });

        // 回收站操作
        api.GET("/trash", [trashHandler](gin::Context& c) {
            trashHandler->List(c);
        });

        api.POST("/trash/:id/restore", [trashHandler](gin::Context& c) {
            trashHandler->Restore(c);
        });

        api.Delete("/trash/:id", [trashHandler](gin::Context& c) {
            trashHandler->PermanentlyDelete(c);
        });
    }

    engine->SetRouteOptions("POST", "/api/files", {
        true, "uploads", config.maxConcurrentUploads,
        {middleware::Errors(), middleware::JWT(app.auth),
         middleware::UploadLimits(config.maxUploadBytes)}
    });
    engine->SetRouteOptions("GET", "/livez", {false, "health"});
    engine->SetRouteOptions("GET", "/readyz", {false, "health"});
    engine->NoRoute([](gin::Context& context) {
        context.JSON(404, gin::H{{"error", "Endpoint not implemented"}});
    });
    return engine;
}

} // namespace disk

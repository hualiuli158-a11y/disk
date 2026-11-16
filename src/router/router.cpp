#include "router.h"
#include "middleware/jwt.h"
#include "handlers/auth_handler.h"
#include "handlers/user_handler.h"
#include "handlers/folder_handler.h"
#include "handlers/file_handler.h"
#include "handlers/search_handler.h"
#include "handlers/trash_handler.h"

namespace disk {

std::unique_ptr<gin::Engine> SetupRouter(AppContext& app)
{
    auto engine = std::make_unique<gin::Engine>(gin::Default());

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

    return engine;
}

} // namespace disk

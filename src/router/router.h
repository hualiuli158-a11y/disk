#pragma once

#include "gin/gin.h"
#include "database.h"
#include "storage.h"
#include "auth.h"
#include "services/account_service.h"
#include "services/folder_service.h"
#include "services/file_service.h"
#include "services/search_service.h"
#include "services/trash_service.h"

#include <memory>

namespace disk {

struct AppContext
{
    Database& database;
    ObjectStorage& storage;
    const Auth& auth;
    AccountService& accounts;
    FolderService& folders;
    FileService& files;
    SearchService& search;
    TrashService& trash;
};

std::unique_ptr<gin::Engine> SetupRouter(AppContext& app);

} // namespace disk

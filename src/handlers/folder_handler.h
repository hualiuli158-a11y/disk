#pragma once

#include "gin/gin.h"
#include "services/folder_service.h"
#include "services/trash_service.h"

namespace disk::handler {

class FolderHandler
{
public:
    FolderHandler(FolderService& folders, TrashService& trash)
        : folders_(folders), trash_(trash) {}

    void Root(gin::Context& c);
    void Create(gin::Context& c);
    void Contents(gin::Context& c);
    void Children(gin::Context& c);
    void Rename(gin::Context& c);
    void Move(gin::Context& c);
    void Delete(gin::Context& c);

private:
    FolderService& folders_;
    TrashService& trash_;
};

} // namespace disk::handler

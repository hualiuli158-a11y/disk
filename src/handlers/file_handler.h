#pragma once

#include "gin/gin.h"
#include "services/file_service.h"
#include "services/trash_service.h"

namespace disk::handler {

class FileHandler
{
public:
    FileHandler(FileService& files, TrashService& trash)
        : files_(files), trash_(trash) {}

    void Upload(gin::Context& c);
    void Download(gin::Context& c);
    void Rename(gin::Context& c);
    void Move(gin::Context& c);
    void Delete(gin::Context& c);

private:
    FileService& files_;
    TrashService& trash_;
};

} // namespace disk::handler

#pragma once

#include "gin/gin.h"
#include "services/trash_service.h"

namespace disk::handler {

class TrashHandler
{
public:
    explicit TrashHandler(TrashService& trash) : trash_(trash) {}

    void List(gin::Context& c);
    void Restore(gin::Context& c);
    void PermanentlyDelete(gin::Context& c);

private:
    TrashService& trash_;
};

} // namespace disk::handler

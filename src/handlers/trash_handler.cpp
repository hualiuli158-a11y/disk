#include "trash_handler.h"
#include "handler_utils.h"

namespace disk::handler {

void TrashHandler::List(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto page = c.DefaultQuery("page", "1");
    const auto pageSize = c.DefaultQuery("page_size", "50");

    const auto result = trash_.List(userId, page, pageSize);

    nlohmann::json value;
    value["items"] = nlohmann::json::array();
    for (const auto& entry : result.items)
    {
        nlohmann::json item;
        item["id"] = entry.id;
        item["type"] = entry.type;
        item["resource_id"] = entry.resourceId;
        if (entry.originalParentId)
            item["original_parent_id"] = *entry.originalParentId;
        else
            item["original_parent_id"] = nullptr;
        item["name"] = entry.name;
        item["trashed_at"] = entry.trashedAt;
        item["expires_at"] = entry.expiresAt;
        value["items"].push_back(item);
    }
    value["page"] = result.page;
    value["page_size"] = result.pageSize;
    value["total"] = result.total;
    value["total_pages"] = (result.total + result.pageSize - 1) / result.pageSize;

    c.JSON(200, value);
}

void TrashHandler::Restore(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto id = Id(c.Param("id"));

    trash_.Restore(userId, id);
    c.Status(204);
}

void TrashHandler::PermanentlyDelete(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto id = Id(c.Param("id"));

    trash_.PermanentlyDelete(userId, id);
    c.Status(204);
}

} // namespace disk::handler

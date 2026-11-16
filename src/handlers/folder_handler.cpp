#include "folder_handler.h"
#include "handler_utils.h"

namespace disk::handler {

void FolderHandler::Root(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto listing = folders_.Root(userId);
    c.JSON(200, FolderListingToJson(listing, "parent"));
}

void FolderHandler::Create(gin::Context& c)
{
    const auto userId = c.GetUserID();
    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("name") || !body.contains("parent_id") ||
        !body["name"].is_string() || !body["parent_id"].is_number_unsigned())
    {
        throw Error(400, "Invalid folder creation request");
    }

    const auto parentId = body["parent_id"].get<std::uint64_t>();
    const auto name = body["name"].get<std::string>();

    const auto folder = folders_.Create(userId, parentId, name);
    c.JSON(200, FolderToJson(folder));
}

void FolderHandler::Contents(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto folderId = c.ParamUInt64("id");

    ContentsQuery query{
        c.DefaultQuery("page", "1"),
        c.DefaultQuery("page_size", "50"),
        c.DefaultQuery("sort", "name"),
        c.DefaultQuery("order", "asc")
    };

    const auto contents = folders_.Contents(userId, folderId, query);
    c.JSON(200, ContentsPageToJson(contents));
}

void FolderHandler::Children(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto folderId = c.ParamUInt64("id");

    const auto listing = folders_.Children(userId, folderId);
    c.JSON(200, FolderListingToJson(listing, "parent"));
}

void FolderHandler::Rename(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto folderId = c.ParamUInt64("id");

    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("name") || !body["name"].is_string())
    {
        throw Error(400, "Name required");
    }

    const auto name = body["name"].get<std::string>();
    const auto normalized = folders_.Rename(userId, folderId, name);

    c.JSON(200, gin::H{
        {"id", folderId},
        {"name", normalized}
    });
}

void FolderHandler::Move(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto folderId = c.ParamUInt64("id");

    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("parent_id") || !body["parent_id"].is_number_unsigned())
    {
        throw Error(400, "Target folder required");
    }

    const auto target = body["parent_id"].get<std::uint64_t>();
    folders_.Move(userId, folderId, target);

    c.JSON(200, gin::H{
        {"id", folderId},
        {"parent_id", target}
    });
}

void FolderHandler::Delete(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto folderId = c.ParamUInt64("id");

    trash_.DeleteFolder(userId, folderId);
    c.Status(204);
}

} // namespace disk::handler

#pragma once

#include "gin/gin.h"
#include "models.h"
#include "common.h"

#include <string>

namespace disk::handler {

inline std::uint64_t UserID(const gin::Context& context)
{
    const auto* value = context.Get("user_id");
    const auto* user = value ? std::any_cast<std::uint64_t>(value) : nullptr;
    return user ? *user : 0;
}


inline nlohmann::json FolderToJson(const Folder& folder)
{
    nlohmann::json value;
    value["id"] = folder.id;
    if (folder.parentId)
        value["parent_id"] = *folder.parentId;
    else
        value["parent_id"] = nullptr;
    value["name"] = folder.name;
    value["created_at"] = folder.createdAt;
    return value;
}

inline nlohmann::json FolderListingToJson(const FolderListing& listing, const std::string& parentKey = "parent")
{
    nlohmann::json value;
    value[parentKey] = FolderToJson(listing.folder);
    value["children"] = nlohmann::json::array();
    for (const auto& child : listing.children)
    {
        value["children"].push_back(FolderToJson(child));
    }
    return value;
}

inline nlohmann::json ContentsPageToJson(const ContentsPage& contents)
{
    nlohmann::json value;
    value["items"] = nlohmann::json::array();
    for (const auto& item : contents.items)
    {
        nlohmann::json entry;
        entry["id"] = item.id;
        entry["type"] = item.type;
        entry["parent_id"] = item.parentId;
        entry["name"] = item.name;
        if (item.size)
            entry["size"] = *item.size;
        if (item.sha256)
            entry["sha256"] = *item.sha256;
        entry["created_at"] = item.createdAt;
        entry["updated_at"] = item.updatedAt;
        value["items"].push_back(entry);
    }
    value["page"] = contents.page;
    value["page_size"] = contents.pageSize;
    value["total"] = contents.total;
    value["total_pages"] = (contents.total + contents.pageSize - 1) / contents.pageSize;
    return value;
}

} // namespace disk::handler

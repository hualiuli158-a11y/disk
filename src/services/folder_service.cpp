#include "folder_service.h"
#include "common.h"

namespace disk
{
FolderListing FolderService::Root(std::uint64_t user)
{
    const auto root = folders_.FindRoot(user);
    if (!root)
        throw Error(404, "Root folder not found");
    return {*root, folders_.Children(user, root->id)};
}

Folder FolderService::Create(std::uint64_t user, std::uint64_t parent, const std::string &name)
{
    return folders_.Create(user, parent, NormalizeName(name));
}

FolderListing FolderService::Children(std::uint64_t user, std::uint64_t folder)
{
    const auto parent = folders_.FindOwned(user, folder);
    if (!parent)
        throw Error(404, "Folder not found");
    return {*parent, folders_.Children(user, folder)};
}

ContentsPage FolderService::Contents(std::uint64_t user, std::uint64_t folder, const ContentsQuery &query)
{
    // Ownership is checked before query validation, as in the original request flow.
    if (!folders_.FindOwned(user, folder))
        throw Error(404, "Folder not found");
    const auto page = Id(query.page);
    const auto pageSize = Id(query.pageSize);
    if (pageSize > 100 || page > 1000000000)
        throw Error(400, "Invalid pagination");
    if (query.sort != "name" && query.sort != "size" && query.sort != "created_at" &&
        query.sort != "updated_at")
        throw Error(400, "Invalid sort");
    if (query.order != "asc" && query.order != "desc")
        throw Error(400, "Invalid order");
    return folders_.Contents(user, folder, page, pageSize, query.sort, query.order);
}

std::string FolderService::Rename(std::uint64_t user, std::uint64_t folder, const std::string &name)
{
    const auto normalized = NormalizeName(name);
    folders_.Rename(user, folder, normalized);
    return normalized;
}
void FolderService::Move(std::uint64_t user, std::uint64_t folder, std::uint64_t parent)
{
    folders_.Move(user, folder, parent);
}
} // namespace disk

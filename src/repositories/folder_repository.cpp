#include "folder_repository.h"
#include "common.h"
#include "repository_support.h"
#include <set>
namespace disk
{
namespace mysql = boost::mysql;
namespace
{
const std::string folderColumns = "id,parent_id,name,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%fZ')";
Folder ReadFolder(mysql::row_view row)
{
    Folder folder;
    folder.id = row[0].as_uint64();
    if (!row[1].is_null())
        folder.parentId = row[1].as_uint64();
    folder.name = std::string(row[2].as_string());
    folder.createdAt = std::string(row[3].as_string());
    return folder;
}
} // namespace
std::optional<Folder> FolderRepository::FindRoot(std::uint64_t user)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params(
            mysql::runtime("SELECT " + folderColumns +
                           " FROM folders WHERE user_id={} AND parent_id IS NULL AND deleted_at IS NULL"),
            user));
    if (result.rows().empty())
        return std::nullopt;
    return ReadFolder(result.rows()[0]);
}
std::optional<Folder> FolderRepository::FindOwned(std::uint64_t user, std::uint64_t folder)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params(mysql::runtime("SELECT " + folderColumns +
                                          " FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL"),
                           folder, user));
    if (result.rows().empty())
        return std::nullopt;
    return ReadFolder(result.rows()[0]);
}
std::vector<Folder> FolderRepository::Children(std::uint64_t user, std::uint64_t parent)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params(
            mysql::runtime(
                "SELECT " + folderColumns +
                " FROM folders WHERE parent_id={} AND user_id={} AND deleted_at IS NULL ORDER BY name,id"),
            parent, user));
    std::vector<Folder> folders;
    for (auto row : result.rows())
        folders.push_back(ReadFolder(row));
    return folders;
}
Folder FolderRepository::Create(std::uint64_t user, std::uint64_t parent, const std::string &name)
{
    Transaction tx(database_);
    database_.Execute(tx.Connection(),
                      mysql::with_params("SELECT id FROM users WHERE id={} FOR UPDATE", user));
    const auto folder = database_.Execute(
        tx.Connection(),
        mysql::with_params("SELECT id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL", parent,
                           user));
    if (folder.rows().empty())
        throw Error(404, "Parent folder not found");
    const auto inserted = database_.Execute(
        tx.Connection(),
        mysql::with_params("INSERT INTO folders(user_id,parent_id,name,created_at,updated_at) "
                           "VALUES({},{},{},UTC_TIMESTAMP(3),UTC_TIMESTAMP(3))",
                           user, parent, name));
    const auto result = database_.Execute(
        tx.Connection(),
        mysql::with_params(mysql::runtime("SELECT " + folderColumns + " FROM folders WHERE id={}"),
                           inserted.last_insert_id()));
    auto folderResult = ReadFolder(result.rows()[0]);
    tx.Commit();
    return folderResult;
}
ContentsPage FolderRepository::Contents(std::uint64_t user, std::uint64_t folderId, std::uint64_t page,
                                        std::uint64_t pageSize, const std::string &sort,
                                        const std::string &order)
{
    auto connection = database_.Get();
    const std::string sql =
        "SELECT "
        "id,type,parent_id,name,size,sha256,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%fZ'),DATE_FORMAT("
        "updated_at,'%Y-%m-%dT%H:%i:%s.%fZ') FROM (SELECT id,'folder' AS type,parent_id,name,CAST(NULL AS "
        "SIGNED) AS size,NULL AS sha256,"
        "created_at,updated_at FROM folders WHERE parent_id={} AND user_id={} AND deleted_at IS NULL UNION "
        "ALL "
        "SELECT f.id,'file',f.folder_id,f.name,p.size,p.sha256,f.created_at,f.updated_at FROM files f JOIN "
        "physical_files p ON p.id=f.physical_file_id "
        "WHERE f.folder_id={} AND f.user_id={} AND f.deleted_at IS NULL) items ORDER BY type DESC," +
        ContentSort(sort, order) + ",id LIMIT {} OFFSET {}";
    const auto result =
        database_.Execute(connection.get(), mysql::with_params(mysql::runtime(sql), folderId, user, folderId,
                                                               user, pageSize, (page - 1) * pageSize));
    const auto count = database_.Execute(
        connection.get(),
        mysql::with_params(
            "SELECT (SELECT COUNT(*) FROM folders WHERE parent_id={} AND user_id={} AND deleted_at IS "
            "NULL)+(SELECT COUNT(*) FROM files WHERE folder_id={} AND user_id={} AND deleted_at IS NULL)",
            folderId, user, folderId, user));
    ContentsPage contents;
    contents.page = page;
    contents.pageSize = pageSize;
    contents.total = count.rows()[0][0].as_int64();
    for (auto row : result.rows())
        contents.items.push_back(ReadContentItem(row));
    return contents;
}

void FolderRepository::Rename(std::uint64_t user, std::uint64_t folder, const std::string &name)
{
    Transaction transaction(database_);
    LockUser(database_, transaction.Connection(), user);
    const auto source = database_.Execute(
        transaction.Connection(),
        mysql::with_params(
            "SELECT parent_id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL FOR UPDATE",
            folder, user));
    if (source.rows().empty())
        throw Error(404, "Folder not found");
    if (source.rows()[0][0].is_null())
        throw Error(400, "Cannot rename root folder");
    database_.Execute(transaction.Connection(),
                      mysql::with_params(
                          "UPDATE folders SET name={},updated_at=UTC_TIMESTAMP(3) WHERE id={} AND user_id={}",
                          name, folder, user));
    transaction.Commit();
}
void FolderRepository::Move(std::uint64_t user, std::uint64_t folder, std::uint64_t parent)
{
    Transaction transaction(database_);
    LockUser(database_, transaction.Connection(), user);
    const auto source = database_.Execute(
        transaction.Connection(),
        mysql::with_params(
            "SELECT parent_id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL FOR UPDATE",
            folder, user));
    if (source.rows().empty())
        throw Error(404, "Folder not found");
    if (source.rows()[0][0].is_null())
        throw Error(400, "Cannot move root folder");
    std::optional<std::uint64_t> ancestor = parent;
    std::set<std::uint64_t> visited;
    while (ancestor)
    {
        if (*ancestor == folder || !visited.insert(*ancestor).second)
            throw Error(400, "Cannot move folder into its subtree");
        const auto target = database_.Execute(
            transaction.Connection(),
            mysql::with_params(
                "SELECT parent_id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL", *ancestor,
                user));
        if (target.rows().empty())
            throw Error(404, "Target folder not found");
        if (target.rows()[0][0].is_null())
            ancestor.reset();
        else
            ancestor = target.rows()[0][0].as_uint64();
    }
    database_.Execute(
        transaction.Connection(),
        mysql::with_params(
            "UPDATE folders SET parent_id={},updated_at=UTC_TIMESTAMP(3) WHERE id={} AND user_id={}", parent,
            folder, user));
    transaction.Commit();
}
} // namespace disk

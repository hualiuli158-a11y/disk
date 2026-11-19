#include "trash_repository.h"
#include "common.h"
#include "repository_support.h"
#include <algorithm>
namespace disk
{
namespace mysql = boost::mysql;
namespace
{
TrashEntry ReadTrash(mysql::row_view row)
{
    TrashEntry entry;
    entry.id = row[0].as_uint64();
    entry.type = std::string(row[1].as_string());
    entry.resourceId = row[2].as_uint64();
    if (!row[3].is_null())
        entry.originalParentId = row[3].as_uint64();
    entry.name = std::string(row[4].as_string());
    entry.trashedAt = std::string(row[5].as_string());
    entry.expiresAt = std::string(row[6].as_string());
    return entry;
}
const std::string trashColumns =
    "id,resource_type,resource_id,original_parent_id,name,"
    "DATE_FORMAT(trashed_at,'%Y-%m-%dT%H:%i:%s.%fZ'),DATE_FORMAT(expires_at,'%Y-%m-%dT%H:%i:%s.%fZ')";
TrashEntry FindTrash(Database &database, mysql::any_connection &connection, std::uint64_t user,
                     std::uint64_t trash)
{
    const auto rows = database.Execute(
        connection,
        mysql::with_params(mysql::runtime("SELECT " + trashColumns +
                                          " FROM trash_entries WHERE id={} AND user_id={} FOR UPDATE"),
                           trash, user));
    if (rows.rows().empty())
        throw Error(404, "Trash entry not found");
    return ReadTrash(rows.rows()[0]);
}
std::uint64_t CreateEntry(Database &database, mysql::any_connection &connection, std::uint64_t user,
                          const std::string &type, std::uint64_t resource, std::uint64_t parent,
                          const std::string &name)
{
    const auto result = database.Execute(
        connection,
        mysql::with_params(
            "INSERT INTO "
            "trash_entries(user_id,resource_type,resource_id,original_parent_id,name,trashed_at,expires_at) "
            "VALUES({},{},{},{},{},UTC_TIMESTAMP(3),DATE_ADD(UTC_TIMESTAMP(3),INTERVAL 30 DAY))",
            user, type, resource, parent, name));
    return result.last_insert_id();
}
std::uint64_t RestoreParent(Database &database, mysql::any_connection &connection, std::uint64_t user,
                            const std::optional<std::uint64_t> &parent)
{
    if (parent)
    {
        const auto found = database.Execute(
            connection,
            mysql::with_params("SELECT id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL",
                               *parent, user));
        if (!found.rows().empty())
            return found.rows()[0][0].as_uint64();
    }
    const auto root = database.Execute(
        connection,
        mysql::with_params(
            "SELECT id FROM folders WHERE user_id={} AND parent_id IS NULL AND deleted_at IS NULL", user));
    if (root.rows().empty())
        throw Error(404, "Root folder not found");
    return root.rows()[0][0].as_uint64();
}
} // namespace
void TrashRepository::TrashFile(std::uint64_t user, std::uint64_t file)
{
    Transaction transaction(database_);
    auto &connection = transaction.Connection();
    LockUser(database_, connection, user);
    const auto source = database_.Execute(
        connection,
        mysql::with_params(
            "SELECT folder_id,name FROM files WHERE id={} AND user_id={} AND deleted_at IS NULL FOR UPDATE",
            file, user));
    if (source.rows().empty())
        throw Error(404, "File not found");
    const auto row = source.rows()[0];
    const auto trash = CreateEntry(database_, connection, user, "file", file, row[0].as_uint64(),
                                   std::string(row[1].as_string()));
    database_.Execute(connection, mysql::with_params("UPDATE files SET "
                                                     "deleted_at=UTC_TIMESTAMP(3),trash_id={},deletion_key="
                                                     "CONCAT({},':',id),updated_at=UTC_TIMESTAMP(3) "
                                                     "WHERE id={} AND user_id={}",
                                                     trash, trash, file, user));
    transaction.Commit();
}
void TrashRepository::TrashFolder(std::uint64_t user, std::uint64_t folder)
{
    Transaction transaction(database_);
    auto &connection = transaction.Connection();
    LockUser(database_, connection, user);
    const auto source = database_.Execute(
        connection,
        mysql::with_params(
            "SELECT parent_id,name FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL FOR UPDATE",
            folder, user));
    if (source.rows().empty())
        throw Error(404, "Folder not found");
    const auto row = source.rows()[0];
    if (row[0].is_null())
        throw Error(400, "Cannot delete root folder");
    const auto ids = FolderTree(database_, connection, user, folder, true);
    const auto trash = CreateEntry(database_, connection, user, "folder", folder, row[0].as_uint64(),
                                   std::string(row[1].as_string()));
    database_.Execute(connection,
                      mysql::with_params(mysql::runtime("UPDATE files SET "
                                                        "deleted_at=UTC_TIMESTAMP(3),trash_id={},deletion_"
                                                        "key=CONCAT({},':',id),updated_at=UTC_TIMESTAMP(3) "
                                                        "WHERE user_id={} AND folder_id IN (" +
                                                        IdList(ids) + ") AND deleted_at IS NULL"),
                                         trash, trash, user));
    database_.Execute(connection,
                      mysql::with_params(mysql::runtime("UPDATE folders SET "
                                                        "deleted_at=UTC_TIMESTAMP(3),trash_id={},deletion_"
                                                        "key=CONCAT({},':',id),updated_at=UTC_TIMESTAMP(3) "
                                                        "WHERE user_id={} AND id IN (" +
                                                        IdList(ids) + ") AND deleted_at IS NULL"),
                                         trash, trash, user));
    transaction.Commit();
}
TrashPage TrashRepository::List(std::uint64_t user, std::uint64_t page, std::uint64_t pageSize)
{
    auto connection = database_.Get();
    const auto count = database_.Execute(
        connection.get(), mysql::with_params("SELECT COUNT(*) FROM trash_entries WHERE user_id={}", user));
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params(
            mysql::runtime(
                "SELECT " + trashColumns +
                " FROM trash_entries WHERE user_id={} ORDER BY trashed_at DESC,id DESC LIMIT {} OFFSET {}"),
            user, pageSize, (page - 1) * pageSize));
    TrashPage output;
    output.page = page;
    output.pageSize = pageSize;
    output.total = count.rows()[0][0].as_int64();
    for (auto row : result.rows())
        output.items.push_back(ReadTrash(row));
    return output;
}
void TrashRepository::Restore(std::uint64_t user, std::uint64_t trash)
{
    Transaction transaction(database_);
    auto &connection = transaction.Connection();
    LockUser(database_, connection, user);
    const auto entry = FindTrash(database_, connection, user, trash);
    const auto parent = RestoreParent(database_, connection, user, entry.originalParentId);
    if (entry.type == "file")
    {
        const auto found = database_.Execute(
            connection, mysql::with_params("SELECT id FROM files WHERE id={} AND user_id={} AND trash_id={}",
                                           entry.resourceId, user, trash));
        if (found.rows().empty())
            throw Error(404, "Trash entry not found");
        database_.Execute(connection,
                          mysql::with_params("UPDATE files SET "
                                             "folder_id={},deleted_at=NULL,trash_id=NULL,deletion_key='',"
                                             "updated_at=UTC_TIMESTAMP(3) WHERE id={} AND user_id={}",
                                             parent, entry.resourceId, user));
    }
    else if (entry.type == "folder")
    {
        const auto found = database_.Execute(
            connection,
            mysql::with_params("SELECT id FROM folders WHERE id={} AND user_id={} AND trash_id={}",
                               entry.resourceId, user, trash));
        if (found.rows().empty())
            throw Error(404, "Trash entry not found");
        database_.Execute(connection,
                          mysql::with_params("UPDATE folders SET parent_id={} WHERE id={} AND user_id={}",
                                             parent, entry.resourceId, user));
        database_.Execute(
            connection,
            mysql::with_params(
                "UPDATE files SET deleted_at=NULL,trash_id=NULL,deletion_key='',updated_at=UTC_TIMESTAMP(3) "
                "WHERE trash_id={} AND user_id={}",
                trash, user));
        database_.Execute(connection,
                          mysql::with_params("UPDATE folders SET "
                                             "deleted_at=NULL,trash_id=NULL,deletion_key='',updated_at=UTC_"
                                             "TIMESTAMP(3) WHERE trash_id={} AND user_id={}",
                                             trash, user));
    }
    else
        throw Error(404, "Trash entry not found");
    database_.Execute(
        connection, mysql::with_params("DELETE FROM trash_entries WHERE id={} AND user_id={}", trash, user));
    transaction.Commit();
}
std::vector<std::string> TrashRepository::PermanentlyDelete(std::uint64_t user, std::uint64_t trash)
{
    return RetryDeadlockedTransaction([&] { return PermanentlyDeleteAttempt(user, trash); });
}
std::vector<std::string> TrashRepository::PermanentlyDeleteAttempt(std::uint64_t user, std::uint64_t trash)
{
    Transaction transaction(database_);
    auto &connection = transaction.Connection();
    LockUser(database_, connection, user);
    const auto entry = FindTrash(database_, connection, user, trash);
    std::vector<std::uint64_t> folders;
    mysql::results files;
    if (entry.type == "folder")
    {
        const auto found = database_.Execute(
            connection,
            mysql::with_params("SELECT id FROM folders WHERE id={} AND user_id={} AND trash_id={}",
                               entry.resourceId, user, trash));
        if (found.rows().empty())
            throw Error(404, "Trash entry not found");
        folders = FolderTree(database_, connection, user, entry.resourceId, false);
        files = database_.Execute(
            connection, mysql::with_params(
                            mysql::runtime("SELECT f.id,p.size FROM files f JOIN physical_files p ON "
                                           "p.id=f.physical_file_id WHERE f.user_id={} AND f.folder_id IN (" +
                                           IdList(folders) + ") ORDER BY f.id"),
                            user));
    }
    else if (entry.type == "file")
    {
        files = database_.Execute(
            connection,
            mysql::with_params("SELECT f.id,p.size FROM files f JOIN physical_files p ON "
                               "p.id=f.physical_file_id WHERE f.id={} AND f.user_id={} AND f.trash_id={}",
                               entry.resourceId, user, trash));
        if (files.rows().empty())
            throw Error(404, "Trash entry not found");
    }
    else
        throw Error(404, "Trash entry not found");
    std::vector<std::uint64_t> fileIds;
    std::int64_t quota = 0;
    for (auto row : files.rows())
    {
        fileIds.push_back(row[0].as_uint64());
        quota += row[1].as_int64();
    }
    std::vector<std::uint64_t> physicalIds;
    if (!fileIds.empty())
    {
        const auto ids = IdList(fileIds);
        // Include prior versions so their objects can also be reclaimed after the files are deleted.
        const auto candidates = database_.Execute(
            connection,
            mysql::with_params(
                mysql::runtime(
                    "SELECT physical_file_id FROM files WHERE user_id={} AND id IN (" + ids +
                    ") UNION SELECT v.physical_file_id "
                    "FROM file_versions v JOIN files f ON f.id=v.file_id WHERE f.user_id={} AND f.id IN (" +
                    ids + ") ORDER BY physical_file_id"),
                user, user));
        for (auto row : candidates.rows())
            physicalIds.push_back(row[0].as_uint64());
        for (const auto id : physicalIds)
            database_.Execute(connection,
                              mysql::with_params("SELECT id FROM physical_files WHERE id={} FOR UPDATE", id));
        database_.Execute(connection,
                          mysql::with_params(mysql::runtime("DELETE FROM trash_entries WHERE user_id={} AND "
                                                            "resource_type='file' AND resource_id IN (" +
                                                            ids + ")"),
                                             user));
        database_.Execute(
            connection,
            mysql::with_params(mysql::runtime("DELETE FROM files WHERE user_id={} AND id IN (" + ids + ")"),
                               user));
        database_.Execute(connection, mysql::with_params(
                                          "UPDATE users SET used_quota=GREATEST(used_quota-{},0) WHERE id={}",
                                          quota, user));
    }
    std::vector<std::string> keys;
    for (const auto id : physicalIds)
    {
        // Locking reads observe references committed while this transaction waited for the object lock.
        const auto live = database_.Execute(
            connection,
            mysql::with_params("SELECT id FROM files WHERE physical_file_id={} LIMIT 1 FOR SHARE", id));
        const auto versions = database_.Execute(
            connection, mysql::with_params(
                            "SELECT id FROM file_versions WHERE physical_file_id={} LIMIT 1 FOR SHARE", id));
        if (!live.rows().empty() || !versions.rows().empty())
            continue;
        const auto object = database_.Execute(
            connection, mysql::with_params("SELECT storage_key FROM physical_files WHERE id={}", id));
        if (!object.rows().empty())
        {
            keys.emplace_back(object.rows()[0][0].as_string());
            database_.Execute(connection, mysql::with_params("DELETE FROM physical_files WHERE id={}", id));
        }
    }
    if (!folders.empty())
    {
        database_.Execute(connection,
                          mysql::with_params(mysql::runtime("DELETE FROM trash_entries WHERE user_id={} AND "
                                                            "resource_type='folder' AND resource_id IN (" +
                                                            IdList(folders) + ")"),
                                             user));
        // FolderTree is breadth-first. Children are removed before their parents to satisfy the FK.
        for (auto folder = folders.rbegin(); folder != folders.rend(); ++folder)
            database_.Execute(connection, mysql::with_params("DELETE FROM folders WHERE id={} AND user_id={}",
                                                             *folder, user));
    }
    database_.Execute(
        connection, mysql::with_params("DELETE FROM trash_entries WHERE id={} AND user_id={}", trash, user));
    transaction.Commit();
    return keys;
}
} // namespace disk

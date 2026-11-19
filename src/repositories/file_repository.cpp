#include "file_repository.h"
#include "common.h"
#include "repository_support.h"
namespace disk
{
namespace mysql = boost::mysql;
void FileRepository::CheckUpload(std::uint64_t user, std::uint64_t folder, std::optional<std::uint64_t> size)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params("SELECT u.used_quota,u.quota_limit,f.id FROM users u LEFT JOIN folders f "
                           "ON f.user_id=u.id AND f.id={} AND f.deleted_at IS NULL WHERE u.id={}",
                           folder, user));
    if (result.rows().empty())
        throw Error(401, "User not found");
    const auto row = result.rows()[0];
    if (row[2].is_null())
        throw Error(404, "Upload folder not found");
    const auto used = row[0].as_int64();
    const auto limit = row[1].as_int64();
    if (size && (used < 0 || used > limit || *size > static_cast<std::uint64_t>(limit - used)))
        throw Error(507, "Storage quota exceeded");
}
UploadCommit FileRepository::CreateUploadedFile(const UploadedObject &object)
{
    return RetryDeadlockedTransaction([&] { return CreateUploadedFileAttempt(object); });
}
UploadCommit FileRepository::CreateUploadedFileAttempt(const UploadedObject &object)
{
    const auto user = object.user;
    const auto folderId = object.folder;
    const auto &name = object.name;
    const auto &hash = object.sha256;
    const auto size = object.size;
    const auto &key = object.storageKey;
    Transaction tx(database_);
    const auto quota = database_.Execute(
        tx.Connection(),
        mysql::with_params("SELECT used_quota,quota_limit FROM users WHERE id={} FOR UPDATE", user));
    if (quota.rows().empty())
        throw Error(401, "User not found");
    const auto used = quota.rows()[0][0].as_int64();
    const auto limit = quota.rows()[0][1].as_int64();
    if (used < 0 || used > limit || size > static_cast<std::uint64_t>(limit - used))
        throw Error(507, "Storage quota exceeded");
    const auto folder = database_.Execute(
        tx.Connection(),
        mysql::with_params("SELECT id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL",
                           folderId, user));
    if (folder.rows().empty())
        throw Error(404, "Upload folder not found");
    // LAST_INSERT_ID makes an existing hash available without overwriting its object key.
    const auto physical = database_.Execute(
        tx.Connection(),
        mysql::with_params("INSERT INTO physical_files(sha256,size,storage_key,created_at) "
                           "VALUES({},{},{},UTC_TIMESTAMP(3)) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)",
                           hash, size, key));
    const auto physicalId = physical.last_insert_id();
    const auto chosen = database_.Execute(
        tx.Connection(),
        mysql::with_params("SELECT storage_key,size FROM physical_files WHERE id={} FOR SHARE", physicalId));
    if (chosen.rows().empty() || chosen.rows()[0][1].as_int64() != static_cast<std::int64_t>(size))
        throw Error(409, "Content hash conflict");
    const auto logical = database_.Execute(
        tx.Connection(),
        mysql::with_params("INSERT INTO files(user_id,folder_id,physical_file_id,name,created_at,updated_at) "
                           "VALUES({},{},{},{},UTC_TIMESTAMP(3),UTC_TIMESTAMP(3))",
                           user, folderId, physicalId, name));
    const auto fileId = logical.last_insert_id();
    database_.Execute(
        tx.Connection(),
        mysql::with_params("INSERT INTO file_versions(file_id,physical_file_id,source,created_at) VALUES({}, "
                           "{},'upload',UTC_TIMESTAMP(3))",
                           fileId, physicalId));
    database_.Execute(
        tx.Connection(),
        mysql::with_params("UPDATE users SET used_quota=used_quota+{} WHERE id={}", size, user));
    const auto created = database_.Execute(
        tx.Connection(),
        mysql::with_params("SELECT DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%fZ') FROM files WHERE id={}",
                           fileId));
    UploadCommit result;
    result.file = {fileId, folderId, name, size, hash, std::string(created.rows()[0][0].as_string())};
    tx.Commit();
    result.adopted = chosen.rows()[0][0].as_string() == key;
    return result;
}
std::optional<StoredFile> FileRepository::FindForDownload(std::uint64_t user, std::uint64_t file)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params(
            "SELECT f.name,p.storage_key,p.sha256,p.size FROM files f JOIN physical_files p ON "
            "p.id=f.physical_file_id WHERE f.id={} AND f.user_id={} AND f.deleted_at IS NULL",
            file, user));
    if (result.rows().empty())
        return std::nullopt;
    const auto row = result.rows()[0];
    return StoredFile{std::string(row[0].as_string()), std::string(row[1].as_string()),
                      std::string(row[2].as_string()), static_cast<std::uint64_t>(row[3].as_int64())};
}
void FileRepository::WithUnreferencedCandidate(const std::string &key, const std::function<void()> &cleanup)
{
    Transaction transaction(database_);
    // A lost COMMIT reply has an unknown outcome. Keep this lock through object cleanup.
    const auto adopted = database_.Execute(
        transaction.Connection(),
        mysql::with_params("SELECT id FROM physical_files WHERE storage_key={} FOR UPDATE", key));
    if (adopted.rows().empty())
        cleanup();
    transaction.Commit();
}

void FileRepository::Rename(std::uint64_t user, std::uint64_t file, const std::string &name)
{
    Transaction transaction(database_);
    LockUser(database_, transaction.Connection(), user);
    const auto source = database_.Execute(
        transaction.Connection(),
        mysql::with_params(
            "SELECT id FROM files WHERE id={} AND user_id={} AND deleted_at IS NULL FOR UPDATE", file, user));
    if (source.rows().empty())
        throw Error(404, "File not found");
    database_.Execute(
        transaction.Connection(),
        mysql::with_params("UPDATE files SET name={},updated_at=UTC_TIMESTAMP(3) WHERE id={} AND user_id={}",
                           name, file, user));
    transaction.Commit();
}
void FileRepository::Move(std::uint64_t user, std::uint64_t file, std::uint64_t folder)
{
    Transaction transaction(database_);
    LockUser(database_, transaction.Connection(), user);
    const auto source = database_.Execute(
        transaction.Connection(),
        mysql::with_params(
            "SELECT id FROM files WHERE id={} AND user_id={} AND deleted_at IS NULL FOR UPDATE", file, user));
    if (source.rows().empty())
        throw Error(404, "File not found");
    const auto target = database_.Execute(
        transaction.Connection(),
        mysql::with_params("SELECT id FROM folders WHERE id={} AND user_id={} AND deleted_at IS NULL", folder,
                           user));
    if (target.rows().empty())
        throw Error(404, "Target folder not found");
    database_.Execute(
        transaction.Connection(),
        mysql::with_params(
            "UPDATE files SET folder_id={},updated_at=UTC_TIMESTAMP(3) WHERE id={} AND user_id={}", folder,
            file, user));
    transaction.Commit();
}
} // namespace disk

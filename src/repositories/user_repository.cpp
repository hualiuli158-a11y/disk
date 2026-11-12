#include "user_repository.h"

namespace disk
{
namespace mysql = boost::mysql;

std::uint64_t UserRepository::CreateWithRoot(const std::string &username, const std::string &passwordHash)
{
    Transaction transaction(database_);
    const auto result = database_.Execute(
        transaction.Connection(),
        mysql::with_params("INSERT INTO users(username,password,create_time) VALUES({},{},UTC_TIMESTAMP(3))",
                           username, passwordHash));
    const auto user = result.last_insert_id();
    database_.Execute(transaction.Connection(),
                      mysql::with_params("INSERT INTO folders(user_id,parent_id,name,created_at,updated_at) "
                                         "VALUES({},NULL,'root',UTC_TIMESTAMP(3),UTC_TIMESTAMP(3))",
                                         user));
    transaction.Commit();
    return user;
}

std::optional<UserCredentials> UserRepository::FindByUsername(const std::string &username)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(), mysql::with_params("SELECT id,password FROM users WHERE username={}", username));
    if (result.rows().empty())
        return std::nullopt;
    const auto row = result.rows()[0];
    return UserCredentials{row[0].as_uint64(), std::string(row[1].as_string())};
}

std::optional<UserProfile> UserRepository::FindById(std::uint64_t user)
{
    auto connection = database_.Get();
    const auto result = database_.Execute(
        connection.get(),
        mysql::with_params("SELECT id,username,used_quota,quota_limit FROM users WHERE id={}", user));
    if (result.rows().empty())
        return std::nullopt;
    const auto row = result.rows()[0];
    return UserProfile{row[0].as_uint64(), std::string(row[1].as_string()), row[2].as_int64(),
                       row[3].as_int64()};
}
} // namespace disk

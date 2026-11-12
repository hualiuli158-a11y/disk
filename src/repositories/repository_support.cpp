#include "repository_support.h"
#include "common.h"
#include <map>
#include <set>
namespace disk
{
namespace mysql = boost::mysql;
void LockUser(Database &database, mysql::any_connection &connection, std::uint64_t user)
{
    const auto result =
        database.Execute(connection, mysql::with_params("SELECT id FROM users WHERE id={} FOR UPDATE", user));
    if (result.rows().empty())
        throw Error(401, "User not found");
}
std::string IdList(const std::vector<std::uint64_t> &ids)
{
    if (ids.empty())
        throw std::logic_error("An identifier list must contain at least one value");
    std::string result;
    for (const auto id : ids)
    {
        if (!result.empty())
            result += ',';
        result += std::to_string(id);
    }
    return result;
}
std::vector<std::uint64_t> FolderTree(Database &database, mysql::any_connection &connection,
                                      std::uint64_t user, std::uint64_t root, bool activeOnly)
{
    std::vector<std::uint64_t> result{root};
    std::vector<std::uint64_t> level{root};
    std::set<std::uint64_t> visited{root};
    while (!level.empty())
    {
        const auto query = "SELECT id FROM folders WHERE user_id={} AND parent_id IN (" + IdList(level) +
                           ")" + (activeOnly ? " AND deleted_at IS NULL" : "") + " ORDER BY id";
        const auto children = database.Execute(connection, mysql::with_params(mysql::runtime(query), user));
        level.clear();
        for (auto row : children.rows())
        {
            const auto id = row[0].as_uint64();
            if (!visited.insert(id).second)
                throw Error(409, "Folder tree contains a cycle");
            result.push_back(id);
            level.push_back(id);
        }
    }
    return result;
}
ContentItem ReadContentItem(mysql::row_view row)
{
    ContentItem item;
    item.id = row[0].as_uint64();
    item.type = std::string(row[1].as_string());
    item.parentId = row[2].as_uint64();
    item.name = std::string(row[3].as_string());
    if (!row[4].is_null())
        item.size = row[4].as_int64();
    if (!row[5].is_null())
        item.sha256 = std::string(row[5].as_string());
    item.createdAt = std::string(row[6].as_string());
    item.updatedAt = std::string(row[7].as_string());
    return item;
}
std::string ContentSort(const std::string &sort, const std::string &order)
{
    static const std::map<std::string, std::string> columns{
        {"name", "name"}, {"size", "size"}, {"created_at", "created_at"}, {"updated_at", "updated_at"}};
    static const std::map<std::string, std::string> directions{{"asc", "asc"}, {"desc", "desc"}};
    return columns.at(sort) + " " + directions.at(order);
}
} // namespace disk

#pragma once
#include "database.h"
#include "models.h"
namespace disk
{
void LockUser(Database &database, boost::mysql::any_connection &connection, std::uint64_t user);
std::string IdList(const std::vector<std::uint64_t> &ids);
std::vector<std::uint64_t> FolderTree(Database &database, boost::mysql::any_connection &connection,
                                      std::uint64_t user, std::uint64_t root, bool activeOnly);
ContentItem ReadContentItem(boost::mysql::row_view row);
std::string ContentSort(const std::string &sort, const std::string &order);
} // namespace disk

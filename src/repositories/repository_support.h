#pragma once
#include "database.h"
#include "models.h"
#include <boost/mysql/common_server_errc.hpp>
#include <boost/system/system_error.hpp>
#include <chrono>
#include <thread>
namespace disk
{
// 1213 rolls back the whole InnoDB victim transaction. Never retry unknown
// COMMIT outcomes, lock timeouts, or storage RPCs here.
template <class Operation> auto RetryDeadlockedTransaction(Operation operation)
{
    for (unsigned attempt = 0;; ++attempt)
    {
        try { return operation(); }
        catch (const boost::system::system_error &error)
        {
            if (error.code() != boost::mysql::common_server_errc::er_lock_deadlock || attempt == 6)
                throw;
            std::this_thread::sleep_for(std::chrono::milliseconds(2u << attempt));
        }
    }
}
void LockUser(Database &database, boost::mysql::any_connection &connection, std::uint64_t user);
std::string IdList(const std::vector<std::uint64_t> &ids);
std::vector<std::uint64_t> FolderTree(Database &database, boost::mysql::any_connection &connection,
                                      std::uint64_t user, std::uint64_t root, bool activeOnly);
ContentItem ReadContentItem(boost::mysql::row_view row);
std::string ContentSort(const std::string &sort, const std::string &order);
} // namespace disk

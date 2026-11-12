#include "trash_service.h"
#include "query_validation.h"
#include <iostream>
namespace disk
{
void TrashService::DeleteFile(std::uint64_t user, std::uint64_t file)
{
    trash_.TrashFile(user, file);
}
void TrashService::DeleteFolder(std::uint64_t user, std::uint64_t folder)
{
    trash_.TrashFolder(user, folder);
}
TrashPage TrashService::List(std::uint64_t user, const std::string &page, const std::string &pageSize)
{
    const auto [number, size] = ParsePagination(page, pageSize);
    return trash_.List(user, number, size);
}
void TrashService::Restore(std::uint64_t user, std::uint64_t trash)
{
    trash_.Restore(user, trash);
}
void TrashService::PermanentlyDelete(std::uint64_t user, std::uint64_t trash)
{
    const auto keys = trash_.PermanentlyDelete(user, trash);
    for (const auto &key : keys)
    {
        try
        {
            storage_.Delete(key);
        }
        catch (const std::exception &error)
        {
            std::cerr << "Trash object cleanup deferred: " << error.what() << '\n';
        }
    }
}
} // namespace disk

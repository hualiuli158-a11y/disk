#pragma once
#include "repositories/trash_repository.h"
#include "storage.h"
namespace disk
{
class TrashService
{
  public:
    TrashService(TrashRepository &trash, ObjectStorage &storage) : trash_(trash), storage_(storage) {}
    void DeleteFile(std::uint64_t user, std::uint64_t file);
    void DeleteFolder(std::uint64_t user, std::uint64_t folder);
    TrashPage List(std::uint64_t user, const std::string &page, const std::string &pageSize);
    void Restore(std::uint64_t user, std::uint64_t trash);
    void PermanentlyDelete(std::uint64_t user, std::uint64_t trash);

  private:
    TrashRepository &trash_;
    ObjectStorage &storage_;
};
} // namespace disk

#pragma once
#include "database.h"
#include "models.h"
namespace disk
{
class TrashRepository
{
  public:
    explicit TrashRepository(Database &database) : database_(database) {}
    void TrashFile(std::uint64_t user, std::uint64_t file);
    void TrashFolder(std::uint64_t user, std::uint64_t folder);
    TrashPage List(std::uint64_t user, std::uint64_t page, std::uint64_t pageSize);
    void Restore(std::uint64_t user, std::uint64_t trash);
    std::vector<std::string> PermanentlyDelete(std::uint64_t user, std::uint64_t trash);

  private:
    Database &database_;
};
} // namespace disk

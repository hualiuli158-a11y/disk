#pragma once
#include "database.h"
#include "models.h"

namespace disk
{
class FolderRepository
{
  public:
    explicit FolderRepository(Database &database) : database_(database) {}
    std::optional<Folder> FindRoot(std::uint64_t user);
    std::optional<Folder> FindOwned(std::uint64_t user, std::uint64_t folder);
    std::vector<Folder> Children(std::uint64_t user, std::uint64_t parent);
    Folder Create(std::uint64_t user, std::uint64_t parent, const std::string &name);
    void Rename(std::uint64_t user, std::uint64_t folder, const std::string &name);
    void Move(std::uint64_t user, std::uint64_t folder, std::uint64_t parent);
    ContentsPage Contents(std::uint64_t user, std::uint64_t folder, std::uint64_t page,
                          std::uint64_t pageSize, const std::string &sort, const std::string &order);

  private:
    Database &database_;
};
} // namespace disk

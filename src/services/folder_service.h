#pragma once
#include "repositories/folder_repository.h"

namespace disk
{
class FolderService
{
  public:
    explicit FolderService(FolderRepository &folders) : folders_(folders) {}
    FolderListing Root(std::uint64_t user);
    Folder Create(std::uint64_t user, std::uint64_t parent, const std::string &name);
    std::string Rename(std::uint64_t user, std::uint64_t folder, const std::string &name);
    void Move(std::uint64_t user, std::uint64_t folder, std::uint64_t parent);
    FolderListing Children(std::uint64_t user, std::uint64_t folder);
    ContentsPage Contents(std::uint64_t user, std::uint64_t folder, const ContentsQuery &query);

  private:
    FolderRepository &folders_;
};
} // namespace disk

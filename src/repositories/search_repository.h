#pragma once
#include "database.h"
#include "models.h"
namespace disk
{
class SearchRepository
{
  public:
    explicit SearchRepository(Database &database) : database_(database) {}
    ContentsPage Search(std::uint64_t user, const SearchQuery &query, std::uint64_t page,
                        std::uint64_t pageSize);

  private:
    Database &database_;
};
} // namespace disk

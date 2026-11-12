#pragma once
#include "repositories/search_repository.h"
namespace disk
{
class SearchService
{
  public:
    explicit SearchService(SearchRepository &search) : search_(search) {}
    SearchResult Search(std::uint64_t user, SearchQuery query);

  private:
    SearchRepository &search_;
};
} // namespace disk

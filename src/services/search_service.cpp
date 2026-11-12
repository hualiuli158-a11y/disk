#include "search_service.h"
#include "query_validation.h"
namespace disk
{
SearchResult SearchService::Search(std::uint64_t user, SearchQuery query)
{
    query.text = Trim(query.text);
    if (query.text.empty() || Utf8Length(query.text) > 255)
        throw Error(400, "Invalid search query");
    if (query.type.empty())
        query.type = "all";
    if (query.contents.sort.empty())
        query.contents.sort = "name";
    if (query.contents.order.empty())
        query.contents.order = "asc";
    if (query.type != "all" && query.type != "file" && query.type != "folder")
        throw Error(400, "Invalid search type");
    if (query.contents.sort != "name" && query.contents.sort != "size" &&
        query.contents.sort != "created_at" && query.contents.sort != "updated_at")
        throw Error(400, "Invalid sort");
    if (query.contents.order != "asc" && query.contents.order != "desc")
        throw Error(400, "Invalid order");
    const auto [page, size] = ParsePagination(query.contents.page, query.contents.pageSize);
    auto contents = search_.Search(user, query, page, size);
    return {std::move(contents), std::move(query)};
}
} // namespace disk

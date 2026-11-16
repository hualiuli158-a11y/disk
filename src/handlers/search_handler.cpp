#include "search_handler.h"
#include "handler_utils.h"

namespace disk::handler {

void SearchHandler::Search(gin::Context& c)
{
    const auto userId = c.GetUserID();

    SearchQuery query;
    query.text = c.DefaultQuery("q", "");
    query.type = c.DefaultQuery("type", "all");
    query.contents = {
        c.DefaultQuery("page", "1"),
        c.DefaultQuery("page_size", "50"),
        c.DefaultQuery("sort", "name"),
        c.DefaultQuery("order", "asc")
    };

    const auto result = search_.Search(userId, std::move(query));
    auto value = ContentsPageToJson(result.contents);
    value["query"] = result.query.text;
    value["type"] = result.query.type;
    value["sort"] = result.query.contents.sort;
    value["order"] = result.query.contents.order;

    c.JSON(200, value);
}

} // namespace disk::handler

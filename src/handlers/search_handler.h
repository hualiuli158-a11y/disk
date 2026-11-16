#pragma once

#include "gin/gin.h"
#include "services/search_service.h"

namespace disk::handler {

class SearchHandler
{
public:
    explicit SearchHandler(SearchService& search) : search_(search) {}

    void Search(gin::Context& c);

private:
    SearchService& search_;
};

} // namespace disk::handler

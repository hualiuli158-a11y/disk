#pragma once
#include "storage.h"
#include <json/json.h>
#include <map>

namespace disk
{
struct Request
{
    std::string method, path, body;
    std::map<std::string, std::string> query, headers;
    std::uint64_t user = 0;
};

struct Response
{
    int status = 200;
    std::map<std::string, std::string> headers;
    std::string body;
    std::shared_ptr<ObjectReader> reader;
};

Response JsonResponse(const Json::Value &value, int status = 200);
Response ErrorResponse(int status, const std::string &message);
} // namespace disk

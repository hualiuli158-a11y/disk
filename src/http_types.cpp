#include "http_types.h"
#include "common.h"
namespace disk
{
Response JsonResponse(const Json::Value &value, int status)
{
    Response response;
    response.status = status;
    response.body = JsonText(value);
    response.headers["Content-Type"] = "application/json; charset=utf-8";
    return response;
}
Response ErrorResponse(int status, const std::string &message)
{
    Json::Value value;
    value["code"] = status;
    value["msg"] = message;
    return JsonResponse(value, status);
}
} // namespace disk

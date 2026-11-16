#include "auth_handler.h"
#include "common.h"

namespace disk::handler {

void AuthHandler::Register(gin::Context& c)
{
    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("username") || !body.contains("password") ||
        !body["username"].is_string() || !body["password"].is_string())
    {
        throw Error(400, "Username and password required");
    }

    const std::string username = body["username"].get<std::string>();
    const std::string password = body["password"].get<std::string>();

    const std::string token = accounts_.Register(username, password);
    c.JSON(200, gin::H{{"access_token", token}});
}

void AuthHandler::Login(gin::Context& c)
{
    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("username") || !body.contains("password") ||
        !body["username"].is_string() || !body["password"].is_string())
    {
        throw Error(400, "Username and password required");
    }

    const std::string username = body["username"].get<std::string>();
    const std::string password = body["password"].get<std::string>();

    const std::string token = accounts_.Login(username, password);
    c.JSON(200, gin::H{{"access_token", token}});
}

} // namespace disk::handler

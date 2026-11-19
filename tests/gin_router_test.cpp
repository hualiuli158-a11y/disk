#include "gin/gin.h"
#include "common.h"
#include "middleware/jwt.h"
#include "middleware/errors.h"
#include "handlers/handler_utils.h"
#include <openssl/evp.h>
#include <limits>
#include <iostream>
#include <stdexcept>
#include <vector>

namespace {

void Check(bool condition, const char* message)
{
    if (!condition)
        throw std::runtime_error(message);
}

gin::Request MakeRequest(std::string method, std::string path, std::uint64_t user = 0)
{
    gin::Request request;
    request.method = std::move(method);
    request.path = std::move(path);
    if (user) request.headers["x-test-user"] = std::to_string(user);
    return request;
}

gin::Response Dispatch(const gin::Engine& router, const gin::Request& request)
{
    gin::Response response;
    router.ServeHTTP(request, response);
    return response;
}

void CheckJWT()
{
    disk::Config config;
    config.jwtSecret = std::string(48, 't');
    disk::Auth auth(config);
    const auto token = "Bearer " + auth.Token(7);
    auto router = gin::Default();
    router.Use(disk::middleware::Errors());
    auto api = router.Group("/api", disk::middleware::JWT(auth));
    int expectedStatus = 200;
    bool called = false;
    api.GET("/business", [&](gin::Context& c) {
        called = true;
        Check(disk::handler::UserID(c) == 7, "JWT user ID missing");
        if (expectedStatus != 200)
            throw disk::Error(expectedStatus, "Business failure");
        c.String(200, "ok");
    });
    auto request = MakeRequest("GET", "/api/business");
    request.headers["authorization"] = token;
    Check(Dispatch(router, request).body == "ok", "valid JWT rejected");
    for (int status : {400, 404, 409, 500, 503, 507})
    {
        expectedStatus = status;
        const auto response = Dispatch(router, request);
        Check(response.status == status, "JWT changed a business error status");
        Check(nlohmann::json::parse(response.body)["error"] == "Business failure",
              "JWT replaced a business error message");
    }
    for (const auto& authorization : {std::string{}, std::string("Bearer invalid"), token + "!",
                                      std::string("Bearer W10.e30.e30"), std::string("Bearer e30.e30.e30")})
    {
        called = false;
        request.headers["authorization"] = authorization;
        Check(Dispatch(router, request).status == 401 && !called,
              "invalid JWT reached the business handler");
    }
#if defined(OPENSSL_VERSION_MAJOR) && OPENSSL_VERSION_MAJOR >= 3
    request.headers["authorization"] = token;
    Check(EVP_set_default_properties(nullptr, "provider=disk_test_missing_provider") == 1,
          "could not simulate an unavailable OpenSSL provider");
    const auto response = Dispatch(router, request);
    Check(EVP_set_default_properties(nullptr, nullptr) == 1, "could not restore OpenSSL defaults");
    Check(response.status == 500 && !called, "JWT crypto failure did not reach server error handling");
#endif
}

void CheckIdentifiersAndQueries()
{
    auto router = gin::Default();
    router.Use(disk::middleware::Errors());
    bool called = false;
    router.Delete("/files/:id", [&](gin::Context& c) {
        const auto id = disk::Id(c.Param("id"));
        called = true;
        c.String(200, std::to_string(id));
    });
    for (const std::string id : {"42junk", "+42", "-1", "0", "abc", " 42",
                                 "18446744073709551616"})
    {
        called = false;
        Check(Dispatch(router, MakeRequest("DELETE", "/files/" + id)).status == 400 && !called,
              "invalid identifier reached a mutation");
    }
    for (const auto id : {std::uint64_t{42}, std::numeric_limits<std::uint64_t>::max()})
        Check(Dispatch(router, MakeRequest("DELETE", "/files/" + std::to_string(id))).body ==
                  std::to_string(id), "valid identifier rejected");

    router.GET("/query", [](gin::Context& c) {
        c.String(200, c.DefaultQuery("page", "1"));
    });
    auto request = MakeRequest("GET", "/query");
    Check(Dispatch(router, request).body == "1", "missing query default failed");
    request.query = "page=";
    Check(Dispatch(router, request).body.empty(), "empty query silently defaulted");
}

void CheckNoRoute()
{
    auto router = gin::Default();
    router.Use(disk::middleware::Errors());
    std::vector<int> order;
    router.Use([&](gin::Context& c) {
        order.push_back(1);
        c.Next();
        order.push_back(3);
    });
    auto request = MakeRequest("GET", "/missing");
    Check(Dispatch(router, request).status == 404, "default 404 failed");
    Check(order == std::vector<int>({1, 3}), "404 skipped global middleware");
    order.clear();
    router.NoRoute({[&](gin::Context& c) { order.push_back(2); c.String(418, "custom"); }});
    Check(Dispatch(router, request).body == "custom", "custom NoRoute failed");
    Check(order == std::vector<int>({1, 2, 3}), "NoRoute middleware order failed");
    router.NoRoute({[](gin::Context&) { throw disk::Error(400, "NoRoute failure"); }});
    Check(Dispatch(router, request).status == 400, "NoRoute bypassed Recovery");
    router.Use([](gin::Context& c) { c.AbortWithStatus(403); });
    Check(Dispatch(router, request).status == 403, "NoRoute ignored global Abort");
}

} // namespace

int main()
{
    try
    {
        CheckJWT();
        CheckIdentifiersAndQueries();
        CheckNoRoute();

        auto router = gin::Default();
    router.Use(disk::middleware::Errors());
        std::vector<int> order;

        router.Use([&](gin::Context& c) {
            order.push_back(1);
            c.Next();
            order.push_back(5);
        });

        router.Use([](gin::Context& context) {
            const auto user = context.GetHeader("x-test-user");
            if (!user.empty()) context.Set("user_id", disk::Id(user));
        });
        auto api = router.Group("/api");
        api.Use([&](gin::Context& c) {
            order.push_back(2);
            if (disk::handler::UserID(c) == 0)
            {
                c.AbortWithStatusJSON(401, gin::H{{"error", "Authentication required"}});
                return;
            }
            c.Set("authenticated", true);
            c.Next();
            order.push_back(4);
        });

        api.GET("/files/new", [](gin::Context& c) {
            c.String(200, "new");
        });

        api.GET("/files/:id", [&](gin::Context& c) {
            order.push_back(3);
            Check(c.FullPath() == "/api/files/:id", "full path was not retained");
            Check(c.Get("authenticated") != nullptr, "middleware value was not retained");
            c.String(200, c.Param("id"));
        });

        // 1. 验证动态参数路由和洋葱圈中间件执行顺序
        const auto dynamic = Dispatch(router, MakeRequest("GET", "/api/files/42", 7));
        Check(dynamic.body == "42", "parameter route failed");
        Check(order == std::vector<int>({1, 2, 3, 4, 5}), "middleware order failed");

        // 2. 验证静态路由优先于动态参数路由
        Check(Dispatch(router, MakeRequest("GET", "/api/files/new", 7)).body == "new",
              "static route did not take precedence");

        // 3. 验证 Abort 阻断后续 Handler
        bool downstream = false;
        api.Handle("GET", "/blocked",
                   {[](gin::Context& c) {
                        c.AbortWithStatusJSON(403, gin::H{{"error", "blocked"}});
                    },
                    [&](gin::Context&) { downstream = true; }});
        const auto blocked = Dispatch(router, MakeRequest("GET", "/api/blocked", 7));
        Check(blocked.status == 403 && !downstream,
              "Abort did not stop the remaining route handlers");

        // 4. 验证未鉴权拦截
        const auto unauthorized = Dispatch(router, MakeRequest("GET", "/api/files/42", 0));
        Check(unauthorized.status == 401, "group middleware did not reject an unauthenticated request");

        // 5. 验证 404 处理
        const auto missing = Dispatch(router, MakeRequest("POST", "/api/files/42", 7));
        Check(missing.status == 404, "method-specific routing failed");

        std::cout << "All Gin-style router, parameters and middleware checks passed!\n";
        return 0;
    }
    catch (const std::exception& error)
    {
        std::cerr << error.what() << '\n';
        return 1;
    }
}

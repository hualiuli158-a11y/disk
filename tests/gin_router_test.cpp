#include "gin/gin.h"
#include "common.h"
#include <iostream>
#include <stdexcept>
#include <vector>

namespace {

void Check(bool condition, const char* message)
{
    if (!condition)
        throw std::runtime_error(message);
}

disk::Request MakeRequest(std::string method, std::string path, std::uint64_t user = 0)
{
    disk::Request request;
    request.method = std::move(method);
    request.path = std::move(path);
    request.user = user;
    return request;
}

} // namespace

int main()
{
    try
    {
        auto router = gin::Default();
        std::vector<int> order;

        router.Use([&](gin::Context& c) {
            order.push_back(1);
            c.Next();
            order.push_back(5);
        });

        auto api = router.Group("/api");
        api.Use([&](gin::Context& c) {
            order.push_back(2);
            if (c.GetUserID() == 0)
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
        const auto dynamic = router.Dispatch(MakeRequest("GET", "/api/files/42", 7));
        Check(dynamic.body == "42", "parameter route failed");
        Check(order == std::vector<int>({1, 2, 3, 4, 5}), "middleware order failed");

        // 2. 验证静态路由优先于动态参数路由
        Check(router.Dispatch(MakeRequest("GET", "/api/files/new", 7)).body == "new",
              "static route did not take precedence");

        // 3. 验证 Abort 阻断后续 Handler
        bool downstream = false;
        api.Handle("GET", "/blocked",
                   {[](gin::Context& c) {
                        c.AbortWithStatusJSON(403, gin::H{{"error", "blocked"}});
                    },
                    [&](gin::Context&) { downstream = true; }});
        const auto blocked = router.Dispatch(MakeRequest("GET", "/api/blocked", 7));
        Check(blocked.status == 403 && !downstream,
              "Abort did not stop the remaining route handlers");

        // 4. 验证未鉴权拦截
        const auto unauthorized = router.Dispatch(MakeRequest("GET", "/api/files/42", 0));
        Check(unauthorized.status == 401, "group middleware did not reject an unauthenticated request");

        // 5. 验证 404 处理
        const auto missing = router.Dispatch(MakeRequest("POST", "/api/files/42", 7));
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

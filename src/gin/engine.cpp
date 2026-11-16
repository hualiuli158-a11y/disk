#include "engine.h"
#include "common.h"
#include <boost/mysql/common_server_errc.hpp>
#include <iostream>

namespace gin {

Engine::Engine(HandlersChain middleware) : RouterGroup(this, "/")
{
    Use(std::move(middleware));
}

Engine New() { return Engine{}; }

HandlerFunc Recovery()
{
    return [](Context& c) {
        try
        {
            c.Next();
        }
        catch (const disk::Error& err)
        {
            c.AbortWithStatusJSON(err.status, nlohmann::json{{"error", err.what()}});
        }
        catch (const boost::system::system_error& err)
        {
            if (err.code() == boost::mysql::common_server_errc::er_dup_entry)
            {
                c.AbortWithStatusJSON(409, nlohmann::json{{"error", "Name already exists"}});
            }
            else
            {
                std::cerr << "[GIN] Database or I/O failure: " << err.code() << '\n';
                c.AbortWithStatusJSON(503, nlohmann::json{{"error", "Dependency unavailable"}});
            }
        }
        catch (const std::exception& err)
        {
            std::cerr << "[GIN] Internal server error: " << err.what() << '\n';
            c.AbortWithStatusJSON(500, nlohmann::json{{"error", "Internal server error"}});
        }
        catch (...)
        {
            std::cerr << "[GIN] Unknown exception caught\n";
            c.AbortWithStatusJSON(500, nlohmann::json{{"error", "Internal server error"}});
        }
    };
}

Engine Default()
{
    return Engine{HandlersChain{Recovery()}};
}

void Engine::AddRoute(const std::string& method, const std::string& path, HandlersChain handlers)
{
    _router.Add(method, path, std::move(handlers));
}

void Engine::NoRoute(HandlersChain handlers)
{
    _no_route = std::move(handlers);
}

disk::Response Engine::Dispatch(const disk::Request& request,
                                std::shared_ptr<disk::BodyPipe> pipe,
                                std::shared_ptr<disk::Cancellation> cancel) const
{
    disk::Response response;
    auto match = _router.Find(request.method, request.path);
    if (match.handlers)
    {
        Context context(request, response, *match.handlers,
                        std::move(match.params), std::move(match.full_path),
                        std::move(pipe), std::move(cancel));
        context.Next();
        return response;
    }

    // 路由未命中处理
    response.status = 404;
    if (!_no_route.empty())
    {
        Context context(request, response, _no_route, {}, "", std::move(pipe), std::move(cancel));
        context.Next();
    }
    else
    {
        response.headers["Content-Type"] = "application/json";
        response.body = nlohmann::json{{"error", "Endpoint not implemented"}}.dump();
    }
    return response;
}

} // namespace gin

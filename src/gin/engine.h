#pragma once

#include "routergroup.h"
#include "tree.h"
#include "http_types.h"

#include <memory>
#include <string>

namespace disk {
class BodyPipe;
class Cancellation;
}

namespace gin {

class Engine : public RouterGroup
{
public:
    explicit Engine(HandlersChain middleware = {});
    Engine(const Engine&) = delete;
    Engine& operator=(const Engine&) = delete;
    Engine(Engine&& other) noexcept;
    Engine& operator=(Engine&& other) noexcept;

    disk::Response Dispatch(const disk::Request& request,
                            std::shared_ptr<disk::BodyPipe> pipe = nullptr,
                            std::shared_ptr<disk::Cancellation> cancel = nullptr) const;

    void NoRoute(HandlersChain handlers);

private:
    friend class RouterGroup;
    void AddRoute(const std::string& method, const std::string& path, HandlersChain handlers);

    detail::Router _router;
    HandlersChain _no_route;
};

Engine New();
Engine Default();

} // namespace gin

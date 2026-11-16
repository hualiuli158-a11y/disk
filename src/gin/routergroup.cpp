#include "routergroup.h"
#include "engine.h"

#include <stdexcept>

namespace gin {
namespace {

void ValidateHandlers(const HandlersChain& handlers)
{
    for (const auto& handler : handlers)
        if (!handler) throw std::invalid_argument("middleware must not be empty");
}

} // namespace

RouterGroup::RouterGroup(Engine* engine, std::string base_path, HandlersChain handlers)
    : _handlers(std::move(handlers)), _engine(engine), _base_path(std::move(base_path)) {}

std::string RouterGroup::AbsolutePath(const std::string& relative_path) const
{
    if (relative_path.empty()) return _base_path;
    if (relative_path.front() != '/') throw std::invalid_argument("relative route must start with '/'");
    if (_base_path == "/") return relative_path;
    return _base_path.back() == '/' ? _base_path.substr(0, _base_path.size() - 1) + relative_path
                                    : _base_path + relative_path;
}

RouterGroup& RouterGroup::Use(HandlersChain middleware)
{
    ValidateHandlers(middleware);
    _handlers.insert(_handlers.end(), middleware.begin(), middleware.end());
    return *this;
}

RouterGroup RouterGroup::Group(const std::string& relative_path, HandlersChain handlers)
{
    ValidateHandlers(handlers);
    HandlersChain combined = _handlers;
    combined.insert(combined.end(), handlers.begin(), handlers.end());
    return RouterGroup(_engine, AbsolutePath(relative_path), std::move(combined));
}

std::string RouterGroup::BasePath() const { return _base_path; }

RouterGroup& RouterGroup::Handle(const std::string& method, const std::string& relative_path,
                                HandlersChain handlers)
{
    if (handlers.empty()) throw std::invalid_argument("route requires a handler");
    HandlersChain combined = _handlers;
    combined.insert(combined.end(), handlers.begin(), handlers.end());
    _engine->AddRoute(method, AbsolutePath(relative_path), std::move(combined));
    return *this;
}

} // namespace gin

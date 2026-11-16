#pragma once

#include "context.h"
#include "tree.h"

#include <string>
#include <utility>

namespace gin {

class Engine;

class RouterGroup
{
public:
    RouterGroup& Use(HandlersChain middleware);
    RouterGroup Group(const std::string& relative_path, HandlersChain handlers = {});
    std::string BasePath() const;
    RouterGroup& Handle(const std::string& method, const std::string& relative_path, HandlersChain handlers);

    template <typename... Rest>
    RouterGroup& Use(HandlerFunc middleware, Rest... rest)
    {
        return Use(HandlersChain{std::move(middleware), HandlerFunc(std::move(rest))...});
    }

    template <typename... Rest>
    RouterGroup Group(const std::string& path, HandlerFunc middleware, Rest... rest)
    {
        return Group(path, HandlersChain{std::move(middleware), HandlerFunc(std::move(rest))...});
    }

    template <typename... Rest>
    RouterGroup& Handle(const std::string& method, const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle(method, path, HandlersChain{std::move(handler), HandlerFunc(std::move(rest))...});
    }

    template <typename... Rest>
    RouterGroup& GET(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("GET", path, std::move(handler), std::move(rest)...);
    }

    template <typename... Rest>
    RouterGroup& POST(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("POST", path, std::move(handler), std::move(rest)...);
    }

    template <typename... Rest>
    RouterGroup& PUT(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("PUT", path, std::move(handler), std::move(rest)...);
    }

    template <typename... Rest>
    RouterGroup& PATCH(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("PATCH", path, std::move(handler), std::move(rest)...);
    }

    template <typename... Rest>
    RouterGroup& Delete(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("DELETE", path, std::move(handler), std::move(rest)...);
    }

#ifdef DELETE
#pragma push_macro("DELETE")
#undef DELETE
    template <typename... Rest>
    RouterGroup& DELETE(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("DELETE", path, std::move(handler), std::move(rest)...);
    }
#pragma pop_macro("DELETE")
#else
    template <typename... Rest>
    RouterGroup& DELETE(const std::string& path, HandlerFunc handler, Rest... rest)
    {
        return Handle("DELETE", path, std::move(handler), std::move(rest)...);
    }
#endif

protected:
    RouterGroup(Engine* engine, std::string base_path, HandlersChain handlers = {});
    HandlersChain _handlers;

private:
    std::string AbsolutePath(const std::string& relative_path) const;
    Engine* _engine;
    std::string _base_path;
};

} // namespace gin

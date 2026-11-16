#pragma once

#include <map>
#include <memory>
#include <string>
#include <vector>
#include <functional>

namespace gin {

class Context;
using HandlerFunc = std::function<void(Context&)>;
using HandlersChain = std::vector<HandlerFunc>;

struct Param {
    std::string Key;
    std::string Value;
};
using Params = std::vector<Param>;

namespace detail {

struct RouteMatch {
    const HandlersChain* handlers = nullptr;
    Params params;
    std::string full_path;
};

class Router {
public:
    void Add(const std::string& method, const std::string& path, HandlersChain handlers);
    RouteMatch Find(const std::string& method, const std::string& path) const;
    std::vector<std::string> AllowedMethods(const std::string& path) const;

private:
    struct Node {
        std::map<std::string, std::unique_ptr<Node>> children;
        std::unique_ptr<Node> param;
        std::unique_ptr<Node> catch_all;
        std::string name;
        std::string full_path;
        HandlersChain handlers;
    };

    static bool Match(const Node& node, const std::vector<std::string>& segments,
                      std::size_t index, RouteMatch& result);
    std::map<std::string, Node> _trees;
};

} // namespace detail
} // namespace gin

#include "tree.h"

#include <algorithm>
#include <stdexcept>

namespace gin::detail {
namespace {

std::vector<std::string> SplitPath(const std::string& path)
{
    std::vector<std::string> result;
    std::size_t begin = 1;
    while (true)
    {
        const auto end = path.find('/', begin);
        result.push_back(path.substr(begin, end == std::string::npos ? end : end - begin));
        if (end == std::string::npos) return result;
        begin = end + 1;
    }
}

void Validate(const std::string& method, const std::string& path, const HandlersChain& handlers)
{
    if (method.empty() || !std::all_of(method.begin(), method.end(),
        [](char ch) { return ch >= 'A' && ch <= 'Z'; }))
        throw std::invalid_argument("HTTP method must contain uppercase ASCII letters");
    if (path.empty() || path.front() != '/' || path.find_first_of("?#") != std::string::npos)
        throw std::invalid_argument("route must be an absolute path without query or fragment");
    if (handlers.empty()) throw std::invalid_argument("route requires a handler");
    for (const auto& handler : handlers)
        if (!handler) throw std::invalid_argument("handler must not be empty");
}

} // namespace

void Router::Add(const std::string& method, const std::string& path, HandlersChain handlers)
{
    Validate(method, path, handlers);
    const auto segments = SplitPath(path);
    for (std::size_t i = 0; i < segments.size(); ++i)
    {
        const auto& segment = segments[i];
        const auto wildcard = segment.find_first_of(":*");
        if (wildcard == std::string::npos) continue;
        if (wildcard != 0 || segment.size() == 1 ||
            segment.find_first_of(":*", 1) != std::string::npos ||
            (segment.front() == '*' && i + 1 != segments.size()))
            throw std::invalid_argument("parameters occupy a full segment; catch-all must be last");
    }

    Node* node = &_trees[method];
    for (const auto& segment : segments)
    {
        if (!segment.empty() && (segment.front() == ':' || segment.front() == '*'))
        {
            auto& next = segment.front() == ':' ? node->param : node->catch_all;
            const auto name = segment.substr(1);
            if (next && next->name != name)
                throw std::logic_error("conflicting parameter names in route: " + path);
            if (!next)
            {
                next = std::make_unique<Node>();
                next->name = name;
            }
            node = next.get();
        }
        else
        {
            auto& next = node->children[segment];
            if (!next) next = std::make_unique<Node>();
            node = next.get();
        }
    }
    if (!node->handlers.empty()) throw std::logic_error("duplicate route: " + method + " " + path);
    node->full_path = path;
    node->handlers = std::move(handlers);
}

bool Router::Match(const Node& node, const std::vector<std::string>& segments,
                   std::size_t index, RouteMatch& result)
{
    if (index == segments.size())
    {
        if (node.handlers.empty()) return false;
        result.handlers = &node.handlers;
        result.full_path = node.full_path;
        return true;
    }
    const auto found = node.children.find(segments[index]);
    if (found != node.children.end() && Match(*found->second, segments, index + 1, result)) return true;
    if (node.param && !segments[index].empty())
    {
        result.params.push_back({node.param->name, segments[index]});
        if (Match(*node.param, segments, index + 1, result)) return true;
        result.params.pop_back();
    }
    if (node.catch_all)
    {
        std::string remaining;
        for (std::size_t i = index; i < segments.size(); ++i) remaining += "/" + segments[i];
        result.params.push_back({node.catch_all->name, std::move(remaining)});
        result.handlers = &node.catch_all->handlers;
        result.full_path = node.catch_all->full_path;
        return !node.catch_all->handlers.empty();
    }
    return false;
}

RouteMatch Router::Find(const std::string& method, const std::string& path) const
{
    RouteMatch result;
    const auto tree = _trees.find(method);
    if (tree != _trees.end() && !path.empty() && path.front() == '/')
        Match(tree->second, SplitPath(path), 0, result);
    return result;
}

std::vector<std::string> Router::AllowedMethods(const std::string& path) const
{
    std::vector<std::string> methods;
    for (const auto& entry : _trees)
        if (Find(entry.first, path).handlers) methods.push_back(entry.first);
    return methods;
}

} // namespace gin::detail

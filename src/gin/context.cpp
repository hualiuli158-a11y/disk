#include "context.h"
#include "tree.h"

#include <algorithm>
#include <cctype>
#include <stdexcept>

namespace gin {

Context::Context(const disk::Request& request, disk::Response& response,
                 const HandlersChain& handlers, gin::Params params,
                 std::string full_path,
                 std::shared_ptr<disk::BodyPipe> pipe,
                 std::shared_ptr<disk::Cancellation> cancel)
    : Request(request), Response(response), Params(std::move(params)),
      _handlers(handlers), _full_path(std::move(full_path)),
      _pipe(std::move(pipe)), _cancel(std::move(cancel))
{
    if (Request.user != 0)
    {
        Set("user_id", Request.user);
    }
}

void Context::Next()
{
    ++_index;
    while (!_aborted && _index < static_cast<std::ptrdiff_t>(_handlers.size()))
    {
        _handlers[static_cast<std::size_t>(_index)](*this);
        ++_index;
    }
}

void Context::Abort()
{
    _aborted = true;
}

bool Context::IsAborted() const
{
    return _aborted;
}

void Context::AbortWithStatus(int code)
{
    Status(code);
    Abort();
}

void Context::AbortWithStatusJSON(int code, const nlohmann::json& value)
{
    Abort();
    JSON(code, value);
}

std::string Context::Param(const std::string& key) const
{
    for (const auto& param : Params)
    {
        if (param.Key == key) return param.Value;
    }
    return {};
}

std::uint64_t Context::ParamUInt64(const std::string& key) const
{
    const auto val = Param(key);
    if (val.empty()) return 0;
    try
    {
        return std::stoull(val);
    }
    catch (...)
    {
        return 0;
    }
}

std::string Context::FullPath() const
{
    return _full_path;
}

std::string Context::Query(const std::string& key) const
{
    const auto found = Request.query.find(key);
    return found == Request.query.end() ? std::string{} : found->second;
}

std::string Context::DefaultQuery(const std::string& key, const std::string& default_value) const
{
    const auto found = Request.query.find(key);
    return (found == Request.query.end() || found->second.empty()) ? default_value : found->second;
}

std::string Context::GetHeader(const std::string& name) const
{
    for (const auto& [k, v] : Request.headers)
    {
        if (k.size() == name.size() &&
            std::equal(k.begin(), k.end(), name.begin(), [](char a, char b) {
                return std::tolower(static_cast<unsigned char>(a)) ==
                       std::tolower(static_cast<unsigned char>(b));
            }))
        {
            return v;
        }
    }
    return {};
}

void Context::Header(const std::string& name, const std::string& value)
{
    Response.headers[name] = value;
}

void Context::Set(std::string key, std::any value)
{
    _keys.insert_or_assign(std::move(key), std::move(value));
}

const std::any* Context::Get(const std::string& key) const
{
    const auto found = _keys.find(key);
    return found == _keys.end() ? nullptr : &found->second;
}

const std::any& Context::MustGet(const std::string& key) const
{
    const auto* value = Get(key);
    if (!value) throw std::out_of_range("context key does not exist: " + key);
    return *value;
}

std::uint64_t Context::GetUserID() const
{
    const auto* anyVal = Get("user_id");
    if (anyVal)
    {
        try { return std::any_cast<std::uint64_t>(*anyVal); } catch (...) {}
    }
    return Request.user;
}

void Context::Status(int code)
{
    Response.status = code;
}

void Context::Data(int code, const std::string& content_type, const std::string& data)
{
    Header("Content-Type", content_type);
    Status(code);
    Response.body = data;
}

void Context::String(int code, const std::string& value)
{
    Data(code, "text/plain; charset=utf-8", value);
}

void Context::JSON(int code, const nlohmann::json& value)
{
    Data(code, "application/json", value.dump());
}

void Context::SetReader(std::shared_ptr<disk::ObjectReader> reader, const std::string& content_type)
{
    Header("Content-Type", content_type);
    Status(200);
    Response.reader = std::move(reader);
}

} // namespace gin

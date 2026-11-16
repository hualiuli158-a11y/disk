#pragma once

#include "http_types.h"
#include <nlohmann/json.hpp>

#include <any>
#include <cstddef>
#include <cstdint>
#include <functional>
#include <memory>
#include <string>
#include <unordered_map>
#include <utility>
#include <vector>

namespace disk {
class BodyPipe;
class Cancellation;
}

namespace gin {

class Context;
using HandlerFunc = std::function<void(Context&)>;
using HandlersChain = std::vector<HandlerFunc>;
using H = nlohmann::json;

struct Param;
using Params = std::vector<Param>;

class Context
{
public:
    const disk::Request& Request;
    disk::Response& Response;
    gin::Params Params;

    Context(const disk::Request& request, disk::Response& response,
            const HandlersChain& handlers, gin::Params params = {},
            std::string full_path = {},
            std::shared_ptr<disk::BodyPipe> pipe = nullptr,
            std::shared_ptr<disk::Cancellation> cancel = nullptr);

    Context(const Context&) = delete;
    Context& operator=(const Context&) = delete;

    void Next();
    void Abort();
    bool IsAborted() const;
    void AbortWithStatus(int code);
    void AbortWithStatusJSON(int code, const nlohmann::json& value);

    std::string Param(const std::string& key) const;
    std::uint64_t ParamUInt64(const std::string& key) const;
    std::string FullPath() const;
    std::string Query(const std::string& key) const;
    std::string DefaultQuery(const std::string& key, const std::string& default_value) const;
    std::string GetHeader(const std::string& name) const;
    void Header(const std::string& name, const std::string& value);

    void Set(std::string key, std::any value);
    const std::any* Get(const std::string& key) const;
    const std::any& MustGet(const std::string& key) const;
    std::uint64_t GetUserID() const;

    void Status(int code);
    void String(int code, const std::string& value);
    void JSON(int code, const nlohmann::json& value);
    void Data(int code, const std::string& content_type, const std::string& data);
    void SetReader(std::shared_ptr<disk::ObjectReader> reader, const std::string& content_type = "application/octet-stream");

    const std::shared_ptr<disk::BodyPipe>& BodyPipe() const { return _pipe; }
    const std::shared_ptr<disk::Cancellation>& Cancellation() const { return _cancel; }

    template <typename T>
    bool ShouldBindJSON(T& value)
    {
        try
        {
            if (Request.body.empty()) return false;
            value = nlohmann::json::parse(Request.body).template get<T>();
            return true;
        }
        catch (...)
        {
            return false;
        }
    }

    template <typename T>
    bool BindJSON(T& value)
    {
        if (ShouldBindJSON(value)) return true;
        AbortWithStatusJSON(400, nlohmann::json{{"error", "Invalid JSON payload"}});
        return false;
    }

private:
    const HandlersChain& _handlers;
    std::ptrdiff_t _index = -1;
    bool _aborted = false;
    std::string _full_path;
    std::unordered_map<std::string, std::any> _keys;
    std::shared_ptr<disk::BodyPipe> _pipe;
    std::shared_ptr<disk::Cancellation> _cancel;
};

} // namespace gin

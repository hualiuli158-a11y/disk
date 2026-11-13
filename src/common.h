#pragma once
#include <cstddef>
#include <cstdint>
#include <json/json.h>
#include <stdexcept>
#include <string>
namespace disk
{
struct Error : std::runtime_error
{
    int status;
    Error(int statusCode, const std::string &message) : std::runtime_error(message), status(statusCode) {}
};
inline std::string JsonText(const Json::Value &value)
{
    Json::StreamWriterBuilder writer;
    writer["indentation"] = "";
    return Json::writeString(writer, value);
}
Json::Value ParseJson(const std::string &body);
std::uint64_t Id(const std::string &value);
std::uint64_t ParseUploadSize(const std::string &value);
std::string NormalizeName(const std::string &name);
std::string Trim(const std::string &text);
std::size_t Utf8Length(const std::string &text);
std::string Hex(const unsigned char *bytes, std::size_t size);
} // namespace disk

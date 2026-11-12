#include "common.h"
#include <charconv>
#include <memory>
#include <vector>
namespace disk
{
namespace
{
struct Character
{
    std::uint32_t value;
    std::size_t offset;
};
std::vector<Character> Characters(const std::string &text)
{
    std::vector<Character> result;
    for (std::size_t offset = 0; offset < text.size();)
    {
        const auto begin = offset;
        const auto first = static_cast<unsigned char>(text[offset++]);
        std::uint32_t value = first;
        unsigned continuation = 0;
        if (first >= 0xc2 && first <= 0xdf)
        {
            value &= 0x1f;
            continuation = 1;
        }
        else if (first >= 0xe0 && first <= 0xef)
        {
            value &= 0x0f;
            continuation = 2;
        }
        else if (first >= 0xf0 && first <= 0xf4)
        {
            value &= 7;
            continuation = 3;
        }
        else if (first >= 0x80)
            throw Error(400, "Invalid UTF-8 text");
        if (offset + continuation > text.size())
            throw Error(400, "Invalid UTF-8 text");
        for (unsigned i = 0; i < continuation; ++i)
        {
            const auto next = static_cast<unsigned char>(text[offset++]);
            if ((next & 0xc0) != 0x80)
                throw Error(400, "Invalid UTF-8 text");
            value = (value << 6) | (next & 0x3f);
        }
        if ((continuation == 2 && value < 0x800) || (continuation == 3 && value < 0x10000) ||
            (value >= 0xd800 && value <= 0xdfff) || value > 0x10ffff)
            throw Error(400, "Invalid UTF-8 text");
        result.push_back({value, begin});
    }
    return result;
}
bool Space(std::uint32_t c)
{
    return (c >= 9 && c <= 13) || c == 32 || c == 0x85 || c == 0xa0 || c == 0x1680 ||
           (c >= 0x2000 && c <= 0x200a) || c == 0x2028 || c == 0x2029 || c == 0x202f || c == 0x205f ||
           c == 0x3000;
}
} // namespace
std::string Trim(const std::string &text)
{
    const auto characters = Characters(text);
    std::size_t begin = 0, end = characters.size();
    while (begin < end && Space(characters[begin].value))
        ++begin;
    while (end > begin && Space(characters[end - 1].value))
        --end;
    if (begin == end)
        return {};
    const auto finish = end == characters.size() ? text.size() : characters[end].offset;
    return text.substr(characters[begin].offset, finish - characters[begin].offset);
}

std::size_t Utf8Length(const std::string &text)
{
    return Characters(text).size();
}
Json::Value ParseJson(const std::string &text)
{
    Json::CharReaderBuilder builder;
    builder["rejectDupKeys"] = true;
    builder["failIfExtra"] = true;
    Json::Value value;
    std::string errors;
    const std::unique_ptr<Json::CharReader> reader(builder.newCharReader());
    if (!reader->parse(text.data(), text.data() + text.size(), &value, &errors) || !value.isObject())
        throw Error(400, "Expected JSON object");
    return value;
}
std::uint64_t Id(const std::string &text)
{
    std::uint64_t value = 0;
    const auto parsed = std::from_chars(text.data(), text.data() + text.size(), value);
    if (parsed.ec != std::errc{} || parsed.ptr != text.data() + text.size() || !value)
        throw Error(400, "Invalid identifier");
    return value;
}
std::string NormalizeName(const std::string &input)
{
    const auto name = Trim(input);
    const auto characters = Characters(name);
    if (name.empty() || characters.size() > 255 || name == "." || name == ".." ||
        name.find_first_of("/\\") != std::string::npos)
        throw Error(400, "Invalid file or folder name");
    for (const auto &character : characters)
        if (character.value < 32 || character.value == 127)
            throw Error(400, "Invalid file or folder name");
    return name;
}
std::string Hex(const unsigned char *bytes, std::size_t size)
{
    constexpr char alphabet[] = "0123456789abcdef";
    std::string text;
    text.reserve(size * 2);
    for (std::size_t i = 0; i < size; ++i)
    {
        text += alphabet[bytes[i] >> 4];
        text += alphabet[bytes[i] & 15];
    }
    return text;
}
} // namespace disk

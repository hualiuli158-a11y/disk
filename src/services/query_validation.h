#pragma once
#include "common.h"
#include <limits>
#include <utility>
namespace disk
{
inline std::pair<std::uint64_t, std::uint64_t> ParsePagination(const std::string &page,
                                                               const std::string &pageSize)
{
    const auto number = Id(page);
    const auto size = Id(pageSize);
    if (size > 100 || number > static_cast<std::uint64_t>(std::numeric_limits<std::int64_t>::max()) / size)
        throw Error(400, "Invalid pagination");
    return {number, size};
}
} // namespace disk

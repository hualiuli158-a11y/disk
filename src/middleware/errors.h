#pragma once

#include "gin/gin.h"
#include "common.h"
#include <boost/system/system_error.hpp>
#include <boost/mysql/common_server_errc.hpp>
#include <iostream>

namespace disk::middleware {

inline gin::HandlerFunc Errors()
{
    return [](gin::Context& c) {
        try
        {
            c.Next();
        }
        catch (const disk::Error& err)
        {
            c.AbortWithStatusJSON(err.status, nlohmann::json{{"error", err.what()}});
        }
        catch (const boost::system::system_error& err)
        {
            if (err.code() == boost::mysql::common_server_errc::er_dup_entry)
            {
                c.AbortWithStatusJSON(409, nlohmann::json{{"error", "Name already exists"}});
            }
            else
            {
                std::cerr << "[GIN] Database or I/O failure: " << err.code() << '\n';
                c.AbortWithStatusJSON(503, nlohmann::json{{"error", "Dependency unavailable"}});
            }
        }
        catch (const std::exception& err)
        {
            std::cerr << "[GIN] Internal server error: " << err.what() << '\n';
            c.AbortWithStatusJSON(500, nlohmann::json{{"error", "Internal server error"}});
        }
        catch (...)
        {
            std::cerr << "[GIN] Unknown exception caught\n";
            c.AbortWithStatusJSON(500, nlohmann::json{{"error", "Internal server error"}});
        }
    };
}


} // namespace disk::middleware

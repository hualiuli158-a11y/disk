#pragma once

#include "gin/gin.h"
#include "auth.h"

namespace disk::middleware {

inline gin::HandlerFunc JWT(const Auth& auth)
{
    return [&auth](gin::Context& c) {
        std::string authorization = c.GetHeader("Authorization");
        if (authorization.empty())
        {
            authorization = c.GetHeader("token");
        }
        if (authorization.empty())
        {
            c.AbortWithStatusJSON(401, gin::H{{"error", "Authentication required"}});
            return;
        }

        try
        {
            std::uint64_t userId = auth.Verify(authorization);
            c.Set("user_id", userId);
            c.Next();
        }
        catch (const std::exception&)
        {
            c.AbortWithStatusJSON(401, gin::H{{"error", "Invalid or expired token"}});
        }
    };
}

} // namespace disk::middleware

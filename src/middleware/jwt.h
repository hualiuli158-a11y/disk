#pragma once

#include "gin/gin.h"
#include "auth.h"
#include "common.h"

namespace disk::middleware {

inline gin::HandlerFunc JWT(const Auth& auth)
{
    return [&auth](gin::Context& c) {
        if (c.Get("user_id"))
        {
            c.Next();
            return;
        }
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

        std::uint64_t userId;
        try
        {
            userId = auth.Verify(authorization);
        }
        catch (const Error& error)
        {
            if (error.status != 401)
                throw;
            c.AbortWithStatusJSON(401, gin::H{{"error", "Invalid or expired token"}});
            return;
        }
        c.Set("user_id", userId);
        c.Next();
    };
}

} // namespace disk::middleware

#include "user_handler.h"

namespace disk::handler {

void UserHandler::Profile(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto profile = accounts_.Profile(userId);

    c.JSON(200, gin::H{
        {"id", profile.id},
        {"username", profile.username},
        {"used_quota", profile.usedQuota},
        {"quota_limit", profile.quotaLimit},
        {"created_at", profile.createdAt},
        {"updated_at", profile.updatedAt}
    });
}

void UserHandler::Quota(gin::Context& c)
{
    const auto userId = c.GetUserID();
    const auto result = accounts_.GetQuota(userId);

    c.JSON(200, gin::H{
        {"used", result.used},
        {"limit", result.limit},
        {"remaining", result.remaining}
    });
}

} // namespace disk::handler

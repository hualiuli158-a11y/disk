#pragma once

#include "gin/gin.h"
#include "services/account_service.h"

namespace disk::handler {

class UserHandler
{
public:
    explicit UserHandler(AccountService& accounts) : accounts_(accounts) {}

    void Profile(gin::Context& c);
    void Quota(gin::Context& c);

private:
    AccountService& accounts_;
};

} // namespace disk::handler

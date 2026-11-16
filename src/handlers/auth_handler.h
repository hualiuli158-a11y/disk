#pragma once

#include "gin/gin.h"
#include "services/account_service.h"

namespace disk::handler {

class AuthHandler
{
public:
    explicit AuthHandler(AccountService& accounts) : accounts_(accounts) {}

    void Register(gin::Context& c);
    void Login(gin::Context& c);

private:
    AccountService& accounts_;
};

} // namespace disk::handler

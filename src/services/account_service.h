#pragma once
#include "auth.h"
#include "repositories/user_repository.h"

namespace disk
{
class AccountService
{
  public:
    AccountService(UserRepository &users, const Auth &auth) : users_(users), auth_(auth) {}
    std::string Register(const std::string &username, const std::string &password);
    std::string Login(const std::string &username, const std::string &password);
    UserProfile Profile(std::uint64_t user);
    Quota GetQuota(std::uint64_t user);

  private:
    UserRepository &users_;
    const Auth &auth_;
};
} // namespace disk

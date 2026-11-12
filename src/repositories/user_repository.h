#pragma once
#include "database.h"
#include "models.h"

namespace disk
{
class UserRepository
{
  public:
    explicit UserRepository(Database &database) : database_(database) {}
    std::uint64_t CreateWithRoot(const std::string &username, const std::string &passwordHash);
    std::optional<UserCredentials> FindByUsername(const std::string &username);
    std::optional<UserProfile> FindById(std::uint64_t user);

  private:
    Database &database_;
};
} // namespace disk

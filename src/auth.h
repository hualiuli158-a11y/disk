#pragma once
#include "config.h"
#include <cstdint>
#include <string>
namespace disk
{
class Auth
{
  public:
    explicit Auth(const Config &config) : secret_(config.jwtSecret), issuer_(config.jwtIssuer) {}
    std::string Token(std::uint64_t user) const;
    std::uint64_t Verify(const std::string &authorization) const;
    static std::string Hash(const std::string &password);
    static bool Check(const std::string &password, const std::string &hash);

  private:
    std::string secret_, issuer_;
};
} // namespace disk

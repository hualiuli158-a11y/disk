#include "account_service.h"
#include "common.h"

namespace disk
{
namespace
{
std::string ValidateCredentials(const std::string &username, const std::string &password)
{
    const auto name = Trim(username);
    if (name.empty() || name.size() > 100 || password.empty() || password.size() > 72)
        throw Error(400, "Invalid credentials");
    return name;
}
} // namespace

std::string AccountService::Register(const std::string &username, const std::string &password)
{
    const auto name = ValidateCredentials(username, password);
    if (Utf8Length(password) < 8 || Trim(password).empty())
        throw Error(400, "Password must have at least 8 characters");
    const auto hash = Auth::Hash(password);
    const auto user = users_.CreateWithRoot(name, hash);
    return auth_.Token(user);
}

std::string AccountService::Login(const std::string &username, const std::string &password)
{
    const auto name = ValidateCredentials(username, password);
    const auto credentials = users_.FindByUsername(name);
    if (!credentials || !Auth::Check(password, credentials->passwordHash))
        throw Error(401, "Invalid credentials");
    return auth_.Token(credentials->id);
}

UserProfile AccountService::Profile(std::uint64_t user)
{
    const auto profile = users_.FindById(user);
    if (!profile)
        throw Error(401, "User not found");
    return *profile;
}

Quota AccountService::GetQuota(std::uint64_t user)
{
    const auto profile = Profile(user);
    return {profile.usedQuota, profile.quotaLimit, profile.quotaLimit - profile.usedQuota};
}
} // namespace disk

#include "auth.h"
#include "botan_all.h"
#include "common.h"
#include <jwt-cpp/jwt.h>
namespace disk
{
std::string Auth::Token(std::uint64_t user) const
{
    const auto encode = [](const std::string &value)
    {
        return jwt::base::trim<jwt::alphabet::base64url>(jwt::base::encode<jwt::alphabet::base64url>(value));
    };
    Json::Value header;
    header["alg"] = "HS256";
    header["typ"] = "JWT";
    const auto now =
        std::chrono::duration_cast<std::chrono::seconds>(std::chrono::system_clock::now().time_since_epoch())
            .count();
    Json::Value claims;
    claims["iss"] = issuer_;
    claims["iat"] = Json::Int64(now);
    claims["exp"] = Json::Int64(now + 900);
    claims["user_id"] = Json::UInt64(user);
    const auto payload = encode(JsonText(header)) + "." + encode(JsonText(claims));
    std::error_code error;
    const auto signature = jwt::algorithm::hs256(secret_).sign(payload, error);
    if (error)
        throw std::system_error(error);
    return payload + "." + encode(signature);
}
std::uint64_t Auth::Verify(const std::string &authorization) const
{
    try
    {
        if (authorization.rfind("Bearer ", 0) != 0)
            throw Error(401, "Authentication required");
        const auto decoded = jwt::decode(authorization.substr(7));
        jwt::verify().allow_algorithm(jwt::algorithm::hs256(secret_)).with_issuer(issuer_).verify(decoded);
        if (!decoded.has_expires_at())
            throw Error(401, "Token expiration required");
        // JsonCpp retains the complete unsigned MySQL identifier range.
        const auto claims = ParseJson(decoded.get_payload());
        if (!claims["user_id"].isUInt64() || !claims["user_id"].asUInt64())
            throw Error(401, "Invalid user");
        return claims["user_id"].asUInt64();
    }
    catch (...)
    {
        throw Error(401, "Invalid or expired access token");
    }
}
std::string Auth::Hash(const std::string &password)
{
    Botan::AutoSeeded_RNG random;
    return Botan::generate_bcrypt(password, random, 10, 'a');
}
bool Auth::Check(const std::string &password, const std::string &hash)
{
    return Botan::check_bcrypt(password, hash);
}
} // namespace disk

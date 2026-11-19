#include "auth.h"
#include "botan_all.h"
#include "common.h"
#include <jwt-cpp/jwt.h>
namespace disk
{
namespace
{
constexpr auto InvalidToken = "Invalid or expired access token";

auto DecodeAccessToken(const std::string &token)
{
    const auto decodeSegment = [](const std::string &segment)
    {
        try
        {
            using Alphabet = jwt::alphabet::base64url;
            return jwt::base::decode<Alphabet>(jwt::base::pad<Alphabet>(segment));
        }
        catch (const std::runtime_error &)
        {
            throw Error(401, InvalidToken);
        }
    };
    try
    {
        return jwt::decode<jwt::traits::kazuho_picojson>(token, decodeSegment);
    }
    catch (const std::invalid_argument &) {} // Invalid compact token format.
    catch (const jwt::error::invalid_json_exception &) {}
    catch (const std::bad_cast &) {} // Header or payload is not a JSON object.
    throw Error(401, InvalidToken);
}
} // namespace

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
    if (authorization.rfind("Bearer ", 0) != 0)
        throw Error(401, "Authentication required");
    const auto decoded = DecodeAccessToken(authorization.substr(7));

    Json::Value parsedClaims;
    try
    {
        // JsonCpp retains the complete unsigned MySQL identifier range.
        parsedClaims = ParseJson(decoded.get_payload());
    }
    catch (const Error &error)
    {
        if (error.status != 400)
            throw;
        throw Error(401, InvalidToken);
    }
    const auto &claims = parsedClaims;
    if (!decoded.has_algorithm() || !decoded.get_header_claim("alg").to_json().is<std::string>() ||
        !claims["iss"].isString() || !claims["exp"].isNumeric() ||
        (claims.isMember("iat") && !claims["iat"].isNumeric()) ||
        (claims.isMember("nbf") && !claims["nbf"].isNumeric()) ||
        !claims["user_id"].isUInt64() || !claims["user_id"].asUInt64())
        throw Error(401, InvalidToken);

    std::error_code error;
    jwt::verify().allow_algorithm(jwt::algorithm::hs256(secret_)).with_issuer(issuer_).verify(decoded, error);
    if (error.category() == jwt::error::token_verification_error_category() ||
        error == jwt::error::signature_verification_error::invalid_signature)
        throw Error(401, InvalidToken);
    if (error)
        throw std::system_error(error);
    return claims["user_id"].asUInt64();
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

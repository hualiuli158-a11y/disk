#include "config.h"
#include <charconv>
#include <cstdlib>
#include <stdexcept>
namespace disk
{
namespace
{
std::string Env(const char *key, std::string fallback = {})
{
    const char *value = std::getenv(key);
    return value ? value : fallback;
}
std::uint64_t Number(const char *key, std::uint64_t fallback, std::uint64_t maximum)
{
    const auto value = Env(key);
    if (value.empty())
        return fallback;
    std::uint64_t result = 0;
    const auto parsed = std::from_chars(value.data(), value.data() + value.size(), result);
    if (parsed.ec != std::errc{} || parsed.ptr != value.data() + value.size() || !result || result > maximum)
        throw std::runtime_error(std::string("Invalid environment setting ") + key);
    return result;
}
} // namespace
Config Config::Environment()
{
    Config c;
    c.listen = Env("LISTEN", c.listen);
    c.port = static_cast<std::uint16_t>(Number("PORT", c.port, 65535));
    c.ioThreads = static_cast<int>(Number("IO_THREADS", 2, 64));
    c.databaseHost = Env("DB_HOST", c.databaseHost);
    c.databasePort = static_cast<std::uint16_t>(Number("DB_PORT", 3306, 65535));
    c.databaseName = Env("DB_NAME", c.databaseName);
    c.databaseUser = Env("DB_USER");
    c.databasePassword = Env("DB_PASSWORD");
    c.jwtSecret = Env("JWT_SECRET");
    c.jwtIssuer = Env("JWT_ISSUER", c.jwtIssuer);
    c.storageAddress = Env("STORAGE_ADDRESS", c.storageAddress);
    c.storageCA = Env("STORAGE_TLS_CA");
    c.storageCertificate = Env("STORAGE_TLS_CLIENT_CERT");
    c.storageKey = Env("STORAGE_TLS_CLIENT_KEY");
    c.storageServerName = Env("STORAGE_TLS_SERVER_NAME");
    c.corsOrigin = Env("CORS_ALLOWED_ORIGIN");
    c.maxUploadBytes = Number("UPLOAD_MAX_BYTES", c.maxUploadBytes, 1ULL << 30);
    c.requestTimeout = std::chrono::seconds(Number("REQUEST_TIMEOUT_SECONDS", 1800, 86400));
    if (c.databaseUser.empty() || c.jwtSecret.size() < 32 || c.jwtIssuer.empty())
        throw std::runtime_error("Set DB_USER, JWT_ISSUER and JWT_SECRET (at least 32 bytes)");
    return c;
}
} // namespace disk

#pragma once
#include <chrono>
#include <cstddef>
#include <cstdint>
#include <string>
namespace disk
{
struct Config
{
    std::string listen = "127.0.0.1";
    std::uint16_t port = 8080;
    int ioThreads = 2;
    std::string databaseHost = "127.0.0.1";
    std::uint16_t databasePort = 3306;
    std::string databaseName = "disk";
    std::string databaseUser;
    std::string databasePassword;
    std::string jwtSecret;
    std::string jwtIssuer = "disk";
    std::string storageAddress = "127.0.0.1:50051";
    std::string storageCA, storageCertificate, storageKey, storageServerName;
    std::string corsOrigin;
    bool swaggerEnabled = false;
    std::uint64_t maxUploadBytes = 1ULL << 30;
    std::size_t maxConcurrentUploads = 4;
    std::chrono::seconds uploadIdleTimeout{30};
    std::chrono::seconds requestTimeout{1800};
    static Config Environment();
};
} // namespace disk

#include "gin/gin.h"
#include "auth.h"
#include "middleware/jwt.h"
#include "middleware/errors.h"
#include "middleware/upload_limits.h"
#include <csignal>
#include <iostream>
namespace
{
volatile std::sig_atomic_t stopped = 0;
void Signal(int)
{
    stopped = 1;
}
class GeneratedReader final : public gin::ContentReader
{
  public:
    std::uint64_t Size() const override
    {
        return 32 * 1024 * 1024;
    }
    std::string Read() override
    {
        if (offset_ == Size())
            return {};
        const auto length = std::min<std::uint64_t>(65536, Size() - offset_);
        offset_ += length;
        return std::string(length, 'd');
    }

  private:
    std::uint64_t offset_ = 0;
};
} // namespace
int main()
{
    disk::Config config;
    config.port = 0;
    config.jwtSecret = std::string(48, 't');
    config.corsOrigin = "http://localhost:5173";
    config.uploadIdleTimeout = std::chrono::seconds(2);
    std::atomic<unsigned> activeUploads{0};
    std::atomic<unsigned> activeBusiness{0};
    disk::Auth auth(config);
    if (auth.Verify("Bearer " + auth.Token(UINT64_MAX)) != UINT64_MAX)
        return 1;
    auto router = gin::New();
    router.Use(disk::middleware::Errors());
    router.SetThreadCount(2);
    gin::Limits limits;
    limits.max_body_bytes = config.maxUploadBytes + 65536;
    router.SetLimits(limits);
    gin::ServerOptions options;
    options.max_buffered_body_bytes = 65536;
    options.body_idle_timeout = config.uploadIdleTimeout;
    options.cors_origin = config.corsOrigin;
    options.cors_allowed_headers = "Authorization,Content-Type,If-Match,X-Upload-Size";
    options.cors_exposed_headers = "ETag,X-File-SHA256,Last-Modified,Content-Disposition";
    options.worker_pools["uploads"] = {4, 64};
    options.worker_pools["health"] = {1, 4};
    router.SetServerOptions(options);

    auto api = router.Group("/api", disk::middleware::JWT(auth));
    api.POST("/files",
        [&](gin::Context& context) {
            struct UploadActivity
            {
                std::atomic<unsigned>& count;
                ~UploadActivity() { --count; }
            };
            ++activeUploads;
            const UploadActivity activity{activeUploads};
            const auto user = std::any_cast<std::uint64_t>(context.MustGet("user_id"));
            if (user == 600) std::this_thread::sleep_for(std::chrono::seconds(4));
            std::uint64_t size = 0;
            std::string block;
            while (context.BodyStream()->Pop(block))
            {
                size += block.size();
                std::this_thread::sleep_for(std::chrono::milliseconds(1));
            }
            context.JSON(200, gin::H{{"size", size}});
        });
    router.SetRouteOptions("POST", "/api/files", {
        true, "uploads", config.maxConcurrentUploads,
        {disk::middleware::Errors(), disk::middleware::JWT(auth),
         disk::middleware::UploadLimits(config.maxUploadBytes)}
    });
    router.POST("/echo", [](gin::Context& context) {
        context.JSON(200, gin::H{{"body", context.Request.body}, {"path", context.Request.path}});
    });
    router.GET("/upload-status", [&](gin::Context& context) {
        context.JSON(200, gin::H{{"active", activeUploads.load()}});
    });
    router.GET("/busy", [&](gin::Context& context) {
        ++activeBusiness;
        std::this_thread::sleep_for(std::chrono::seconds(2));
        --activeBusiness;
        context.JSON(200, gin::H{});
    });
    router.GET("/livez", [&](gin::Context& context) {
        context.JSON(200, gin::H{{"active_business", activeBusiness.load()}});
    });
    router.SetRouteOptions("GET", "/livez", {false, "health"});
    router.GET("/download", [](gin::Context& context) {
        context.DataFromReader(200, "application/octet-stream", std::make_shared<GeneratedReader>());
    });
    std::signal(SIGINT, Signal);
    std::signal(SIGTERM, Signal);
    std::atomic<bool> finished{false};
    std::thread monitor(
        [&]
        {
            while (!finished && !router.Port())
                std::this_thread::sleep_for(std::chrono::milliseconds(1));
            if (!finished)
                std::cout << "READY " << router.Port() << ' ' << auth.Token(1) << std::endl;
            while (!finished && !stopped)
                std::this_thread::sleep_for(std::chrono::milliseconds(20));
            if (stopped)
                router.Stop();
        });
    try { router.Run("127.0.0.1:0"); }
    catch (...)
    {
        finished = true;
        monitor.join();
        throw;
    }
    finished = true;
    monitor.join();
}

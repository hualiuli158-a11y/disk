#include "http_server.h"
#include <csignal>
#include <iostream>
namespace
{
volatile std::sig_atomic_t stopped = 0;
void Signal(int)
{
    stopped = 1;
}
class GeneratedReader final : public disk::ObjectReader
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
    disk::Auth auth(config);
    if (auth.Verify("Bearer " + auth.Token(UINT64_MAX)) != UINT64_MAX)
        return 1;
    disk::HttpServer server(config, auth,
                            [](const auto &request, const auto &pipe, const auto &cancel)
                            {
                                if (pipe)
                                {
                                    std::uint64_t count = 0;
                                    std::string block;
                                    while (pipe->Pop(block))
                                    {
                                        count += block.size();
                                        std::this_thread::sleep_for(std::chrono::milliseconds(1));
                                    }
                                    Json::Value result;
                                    result["size"] = Json::UInt64(count);
                                    return disk::JsonResponse(result);
                                }
                                if (request.path == "/download")
                                {
                                    disk::Response response;
                                    response.reader = std::make_shared<GeneratedReader>();
                                    return response;
                                }
                                Json::Value result;
                                result["body"] = request.body;
                                result["path"] = request.path;
                                return disk::JsonResponse(result);
                            });
    std::signal(SIGINT, Signal);
    std::signal(SIGTERM, Signal);
    std::atomic<bool> finished{false};
    std::thread monitor(
        [&]
        {
            while (!finished && !stopped)
                std::this_thread::sleep_for(std::chrono::milliseconds(20));
            if (stopped)
                server.Stop();
        });
    std::cout << "READY " << server.Port() << ' ' << auth.Token(1) << std::endl;
    server.Start();
    finished = true;
    monitor.join();
}

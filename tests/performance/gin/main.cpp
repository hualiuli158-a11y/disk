#include "gin/gin.h"
#include <csignal>
#include <iostream>
#include <thread>

namespace {
volatile std::sig_atomic_t interrupted = 0;
void Interrupt(int) { interrupted = 1; }
}

int main(int argc, char** argv)
{
    try
    {
        unsigned port = 0, threads = 2, duration = 3600;
        for (int i = 1; i < argc; i += 2)
        {
            if (i + 1 == argc) throw std::runtime_error("missing option value");
            const std::string key = argv[i], text = argv[i + 1];
            std::size_t end = 0;
            const auto value = std::stoul(text, &end);
            if (text.front() == '-' || end != text.size()) throw std::runtime_error("invalid option value");
            if (key == "--port" && value <= 65535) port = value;
            else if (key == "--threads" && value > 0 && value <= 64) threads = value;
            else if (key == "--duration" && value > 0 && value <= 86400) duration = value;
            else throw std::runtime_error("unknown option or value out of range");
        }
        // No per-request Logger: the benchmark measures the framework execution path.
        auto router = gin::New();
        router.SetThreadCount(threads);
        router.GET("/benchmark/plaintext", [](gin::Context& c) { c.String(200, "Hello, World!"); });
        router.GET("/benchmark/json", [](gin::Context& c) { c.JSON(200, gin::H{{"message", "Hello, World!"}}); });
        router.POST("/benchmark/echo", [](gin::Context& c) { c.Data(200, "application/octet-stream", c.Request.body); });
        std::signal(SIGINT, Interrupt);
        std::signal(SIGTERM, Interrupt);
        std::jthread monitor([&](std::stop_token stop) {
            const auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(duration);
            while (!stop.stop_requested() && !interrupted && std::chrono::steady_clock::now() < deadline)
                std::this_thread::sleep_for(std::chrono::milliseconds(20));
            if (!stop.stop_requested()) router.Stop();
        });
        router.Run("127.0.0.1:" + std::to_string(port));
    }
    catch (const std::exception& error)
    {
        std::cerr << error.what() << '\n';
        return 1;
    }
}

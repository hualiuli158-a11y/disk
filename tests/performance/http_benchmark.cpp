#include "http_server.h"
#include <csignal>
#include <iostream>
#include <thread>

namespace
{
volatile std::sig_atomic_t interrupted = 0;
void Interrupt(int) { interrupted = 1; }
} // namespace

int main(int argc, char **argv)
{
    try
    {
        disk::Config config;
        config.jwtSecret = std::string(48, 'b');
        config.port = 0;
        unsigned duration = 3600;
        for (int i = 1; i < argc; i += 2)
        {
            if (i + 1 == argc)
                throw std::runtime_error("missing option value");
            std::size_t end = 0;
            const std::string text = argv[i + 1], key = argv[i];
            const auto value = std::stoul(text, &end);
            if (text.front() == '-' || end != text.size())
                throw std::runtime_error("invalid option value");
            if (key == "--port" && value <= 65535)
                config.port = value;
            else if (key == "--threads" && value > 0 && value <= 64)
                config.ioThreads = value;
            else if (key == "--duration" && value > 0 && value <= 86400)
                duration = value;
            else
                throw std::runtime_error("unknown option or value out of range");
        }
        disk::Auth auth(config);
        disk::HttpServer server(config, auth,
                                [](const auto &request, const auto &, const auto &)
                                {
                                    if (request.method == "GET" && request.path == "/benchmark/plaintext")
                                    {
                                        disk::Response response;
                                        response.headers["Content-Type"] = "text/plain";
                                        response.body = "Hello, World!";
                                        return response;
                                    }
                                    if (request.method == "GET" && request.path == "/benchmark/json")
                                    {
                                        Json::Value value;
                                        value["message"] = "Hello, World!";
                                        return disk::JsonResponse(value);
                                    }
                                    if (request.method == "POST" && request.path == "/benchmark/echo")
                                    {
                                        disk::Response response;
                                        response.headers["Content-Type"] = "application/octet-stream";
                                        response.body = request.body;
                                        return response;
                                    }
                                    return disk::ErrorResponse(404, "Benchmark endpoint not found");
                                });
        std::signal(SIGINT, Interrupt);
        std::signal(SIGTERM, Interrupt);
        std::jthread monitor(
            [&](std::stop_token stop)
            {
                const auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(duration);
                while (!stop.stop_requested() && !interrupted && std::chrono::steady_clock::now() < deadline)
                    std::this_thread::sleep_for(std::chrono::milliseconds(20));
                if (!stop.stop_requested())
                    server.Stop();
            });
        std::cout << "READY " << server.Port() << std::endl;
        server.Start();
        return 0;
    }
    catch (const std::exception &error)
    {
        std::cerr << error.what() << '\n';
        return 1;
    }
}

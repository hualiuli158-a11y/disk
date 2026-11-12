#include "application.h"
#include "http_server.h"
#include <csignal>
#include <iostream>
namespace
{
volatile std::sig_atomic_t interrupted = 0;
void Signal(int)
{
    interrupted = 1;
}
} // namespace
int main()
{
    try
    {
        auto config = disk::Config::Environment();
        disk::Database database(config);
        database.Check();
        disk::GrpcStorage storage(config);
        storage.Check();
        disk::Application application(config, database, storage);
        disk::HttpServer server(config, application.Authentication(),
                                [&](const auto &request, const auto &pipe, const auto &cancel)
                                {
                                    return pipe ? application.Upload(request, pipe, cancel)
                                                : application.Handle(request, cancel);
                                });
        std::signal(SIGINT, Signal);
        std::signal(SIGTERM, Signal);
        std::atomic<bool> finished{false};
        std::thread signals(
            [&]
            {
                while (!finished && !interrupted)
                    std::this_thread::sleep_for(std::chrono::milliseconds(100));
                if (interrupted)
                    server.Stop();
            });
        std::cout << "Disk C++ API listening on " << config.listen << ':' << server.Port() << std::endl;
        try
        {
            server.Start();
        }
        catch (...)
        {
            finished = true;
            signals.join();
            throw;
        }
        finished = true;
        signals.join();
    }
    catch (const std::exception &error)
    {
        std::cerr << error.what() << '\n';
        return 1;
    }
}

#include "config.h"
#include "database.h"
#include "storage.h"
#include "auth.h"
#include "router/router.h"
#include "http_server.h"

#include <csignal>
#include <iostream>
#include <thread>
#include <atomic>

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

        disk::Auth auth(config);

        disk::UserRepository usersRepository(database);
        disk::FolderRepository foldersRepository(database);
        disk::FileRepository filesRepository(database);
        disk::SearchRepository searchRepository(database);
        disk::TrashRepository trashRepository(database);

        disk::AccountService accounts(usersRepository, auth);
        disk::FolderService folders(foldersRepository);
        disk::FileService files(filesRepository, storage);
        disk::SearchService search(searchRepository);
        disk::TrashService trash(trashRepository, storage);

        disk::AppContext appContext{
            database,
            storage,
            auth,
            accounts,
            folders,
            files,
            search,
            trash
        };

        // 使用 Gin 风格初始化路由器与路由树
        auto router = disk::SetupRouter(appContext);

        disk::HttpServer server(config, auth,
                                [&](const auto &request, const auto &pipe, const auto &cancel)
                                {
                                    return router->Dispatch(request, pipe, cancel);
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

        std::cout << "Disk Gin C++ API listening on " << config.listen << ':' << server.Port() << std::endl;

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

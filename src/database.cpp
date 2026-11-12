#include "database.h"
#include <boost/asio/detached.hpp>
namespace disk
{
namespace
{
boost::mysql::pool_params Settings(const Config &config)
{
    boost::mysql::pool_params p;
    p.server_address.emplace_host_and_port(config.databaseHost, config.databasePort);
    p.username = config.databaseUser;
    p.password = config.databasePassword;
    p.database = config.databaseName;
    p.thread_safe = true;
    p.initial_size = 1;
    p.max_size = 8;
    p.connect_timeout = std::chrono::seconds(3);
    return p;
}
} // namespace
Database::Database(const Config &config) : pool_(databaseIo_, Settings(config))
{
    pool_.async_run(boost::asio::detached);
    worker_ = std::thread(
        [this]
        {
            databaseIo_.run();
        });
}
Database::~Database()
{
    pool_.cancel();
    worker_.join();
}
boost::mysql::pooled_connection Database::Get()
{
    auto promise = std::make_shared<std::promise<boost::mysql::pooled_connection>>();
    auto future = promise->get_future();
    boost::asio::post(
        databaseIo_,
        [this, promise]
        {
            pool_.async_get_connection(boost::asio::cancel_after(
                std::chrono::seconds(5),
                [promise](boost::system::error_code error, boost::mysql::pooled_connection connection)
                {
                    if (error)
                        promise->set_exception(std::make_exception_ptr(boost::system::system_error(error)));
                    else
                        promise->set_value(std::move(connection));
                }));
        });
    return future.get();
}
void Database::Check()
{
    auto connection = Get();
    Execute(connection.get(), "SELECT 1");
}
Transaction::Transaction(Database &database) : database_(database), connection_(database.Get())
{
    database_.Execute(connection_.get(), "START TRANSACTION");
}
Transaction::~Transaction()
{
    if (!committed_)
    {
        try
        {
            database_.Execute(connection_.get(), "ROLLBACK");
        }
        catch (...)
        {
        }
    }
}
void Transaction::Commit()
{
    database_.Execute(connection_.get(), "COMMIT");
    committed_ = true;
}
} // namespace disk

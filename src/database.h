#pragma once
#include "config.h"
#include <boost/asio/cancel_after.hpp>
#include <boost/asio/io_context.hpp>
#include <boost/asio/post.hpp>
#include <boost/asio/use_future.hpp>
#include <boost/mysql/connection_pool.hpp>
#include <boost/mysql/results.hpp>
#include <boost/mysql/with_params.hpp>
#include <future>
#include <thread>
namespace disk
{
class Database
{
  public:
    explicit Database(const Config &config);
    ~Database();
    boost::mysql::pooled_connection Get();
    template <class Query>
    boost::mysql::results Execute(boost::mysql::any_connection &connection, Query query)
    {
        auto promise = std::make_shared<std::promise<boost::mysql::results>>();
        auto future = promise->get_future();
        // Execute is synchronous to business workers. All initiation happens on databaseIo_.
        boost::asio::post(
            databaseIo_,
            [promise, &connection, &query]
            {
                auto result = std::make_shared<boost::mysql::results>();
                try
                {
                    connection.async_execute(
                        query, *result,
                        boost::asio::cancel_after(std::chrono::seconds(5),
                                                  [promise, result](boost::system::error_code error)
                                                  {
                                                      if (error)
                                                          promise->set_exception(std::make_exception_ptr(
                                                              boost::system::system_error(error)));
                                                      else
                                                          promise->set_value(std::move(*result));
                                                  }));
                }
                catch (...)
                {
                    promise->set_exception(std::current_exception());
                }
            });
        return future.get();
    }
    void Check();

  private:
    boost::asio::io_context databaseIo_;
    boost::mysql::connection_pool pool_;
    std::thread worker_;
};
class Transaction
{
  public:
    explicit Transaction(Database &database);
    ~Transaction();
    void Commit();
    boost::mysql::any_connection &Connection()
    {
        return connection_.get();
    }

  private:
    Database &database_;
    boost::mysql::pooled_connection connection_;
    bool committed_ = false;
};
} // namespace disk

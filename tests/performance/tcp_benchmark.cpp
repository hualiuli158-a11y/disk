#include "net/tcpserver.h"
#include <algorithm>
#include <array>
#include <atomic>
#include <boost/asio.hpp>
#include <chrono>
#include <cmath>
#include <csignal>
#include <cstdint>
#include <iostream>
#include <memory>
#include <string>
#include <thread>
#include <vector>

namespace
{
using Clock = std::chrono::steady_clock;
using Tcp = boost::asio::ip::tcp;
volatile std::sig_atomic_t interrupted = 0;

void Interrupt(int) { interrupted = 1; }

struct Options
{
    std::string mode, host = "127.0.0.1";
    unsigned port = 0, threads = 2, connections = 10, bytes = 1024;
    unsigned warmup = 2, duration = 10, readDelay = 0, reconnect = 0, reconnectDelay = 0;
};

Options Parse(int argc, char **argv)
{
    if (argc < 2)
        throw std::runtime_error("usage: disk_tcp_benchmark server|client [--host IP] [--port N] "
                                 "[--threads N] [--connections N] [--bytes N] [--warmup S] "
                                 "[--duration S] [--read-delay-ms N] [--reconnect 0|1]");
    Options options;
    options.mode = argv[1];
    for (int i = 2; i < argc; i += 2)
    {
        if (i + 1 == argc)
            throw std::runtime_error("missing option value");
        const std::string key = argv[i], text = argv[i + 1];
        if (key == "--host")
        {
            options.host = text;
            continue;
        }
        std::size_t end = 0;
        const auto value = std::stoul(text, &end);
        if (end != text.size() || text.front() == '-' || value > 100000000)
            throw std::runtime_error("invalid numeric option");
        if (key == "--port")
            options.port = value;
        else if (key == "--threads")
            options.threads = value;
        else if (key == "--connections")
            options.connections = value;
        else if (key == "--bytes")
            options.bytes = value;
        else if (key == "--warmup")
            options.warmup = value;
        else if (key == "--duration")
            options.duration = value;
        else if (key == "--read-delay-ms")
            options.readDelay = value;
        else if (key == "--reconnect")
            options.reconnect = value;
        else if (key == "--reconnect-delay-ms")
            options.reconnectDelay = value;
        else
            throw std::runtime_error("unknown option: " + key);
    }
    if ((options.mode != "server" && options.mode != "client") || options.port > 65535 || !options.threads ||
        options.threads > 64 || !options.connections || options.connections > 10000 || !options.bytes ||
        options.bytes > 1048576 || !options.duration || options.duration > 86400 || options.warmup > 3600 ||
        options.readDelay > 60000 || options.reconnectDelay > 60000 || options.reconnect > 1 ||
        (options.mode == "client" && !options.port))
        throw std::runtime_error("option out of range");
    boost::asio::ip::make_address(options.host);
    return options;
}

struct Histogram
{
    // 100 microsecond buckets, with the final bucket retaining overflow samples.
    std::array<std::uint64_t, 100001> latency{};
    std::uint64_t completed = 0;
    double maximumMs = 0;

    void Record(Clock::duration elapsed)
    {
        const auto microseconds = std::chrono::duration_cast<std::chrono::microseconds>(elapsed).count();
        ++latency[std::min<std::uint64_t>((microseconds + 99) / 100, latency.size() - 1)];
        maximumMs = std::max(maximumMs, std::chrono::duration<double, std::milli>(elapsed).count());
        ++completed;
    }

    double Percentile(double fraction) const
    {
        const auto target = static_cast<std::uint64_t>(std::ceil(completed * fraction));
        std::uint64_t seen = 0;
        for (std::size_t i = 0; i < latency.size(); ++i)
        {
            seen += latency[i];
            if (target && seen >= target)
                return i == latency.size() - 1 ? maximumMs : i / 10.0;
        }
        return 0;
    }

    void Merge(const Histogram &other)
    {
        completed += other.completed;
        maximumMs = std::max(maximumMs, other.maximumMs);
        for (std::size_t i = 0; i < latency.size(); ++i)
            latency[i] += other.latency[i];
    }
};

struct Statistics
{
    Histogram messages, connections;
    std::uint64_t started = 0, errors = 0, connected = 0, invalid = 0;
    int lastErrorCode = 0;
};

class Client
{
  public:
    Client(boost::asio::io_context &io, const Options &options, Statistics &statistics,
           Clock::time_point begin, Clock::time_point end, unsigned identity)
        : socket_(io), delay_(io), options_(options), statistics_(statistics), begin_(begin), end_(end),
          payload_(options.bytes, static_cast<char>('a' + identity % 26)), received_(options.bytes, '\0')
    {
    }

    void Connect()
    {
        connectStarted_ = Clock::now();
        socket_.async_connect(
            {boost::asio::ip::make_address(options_.host), static_cast<unsigned short>(options_.port)},
            [this](const auto &error)
            {
                if (Failed(error))
                    return;
                const auto connected = Clock::now();
                if (connectStarted_ >= begin_ && connected <= end_)
                    statistics_.connections.Record(connected - connectStarted_);
                boost::system::error_code optionError;
                socket_.set_option(Tcp::no_delay(true), optionError);
                if (Failed(optionError))
                    return;
                ++statistics_.connected;
                Send();
            });
    }

    void Stop()
    {
        stopped_ = true;
        boost::system::error_code ignored;
        delay_.cancel();
        socket_.close(ignored);
    }

  private:
    bool Failed(const boost::system::error_code &error)
    {
        if (!error)
            return false;
        if (!stopped_)
        {
            ++statistics_.errors;
            statistics_.lastErrorCode = error.value();
        }
        Stop();
        return true;
    }

    void Send()
    {
        started_ = Clock::now();
        measured_ = started_ >= begin_ && started_ < end_;
        if (started_ >= end_)
            return;
        if (measured_)
            ++statistics_.started;
        // Vary the complete payload between round trips to detect stale or corrupted echoes.
        std::fill(payload_.begin(), payload_.end(), static_cast<char>(++sequence_ % 251));
        boost::asio::async_write(socket_, boost::asio::buffer(payload_),
                                 [this](const auto &error, std::size_t)
                                 {
                                     if (Failed(error))
                                         return;
                                     if (!options_.readDelay)
                                     {
                                         Read();
                                         return;
                                     }
                                     delay_.expires_after(std::chrono::milliseconds(options_.readDelay));
                                     delay_.async_wait(
                                         [this](const auto &error)
                                         {
                                             if (!Failed(error))
                                                 Read();
                                         });
                                 });
    }

    void Read()
    {
        boost::asio::async_read(socket_, boost::asio::buffer(received_),
                                [this](const auto &error, std::size_t)
                                {
                                    if (Failed(error))
                                        return;
                                    const auto finished = Clock::now();
                                    if (received_ != payload_)
                                    {
                                        ++statistics_.invalid;
                                        Stop();
                                        return;
                                    }
                                    if (measured_ && finished <= end_)
                                        statistics_.messages.Record(finished - started_);
                                    if (options_.reconnect)
                                    {
                                        boost::system::error_code ignored;
                                        socket_.close(ignored);
                                        delay_.expires_after(
                                            std::chrono::milliseconds(options_.reconnectDelay));
                                        delay_.async_wait(
                                            [this](const auto &error)
                                            {
                                                if (!Failed(error))
                                                    Connect();
                                            });
                                    }
                                    else
                                        Send();
                                });
    }

    Tcp::socket socket_;
    boost::asio::steady_timer delay_;
    const Options &options_;
    Statistics &statistics_;
    Clock::time_point begin_, end_, started_, connectStarted_;
    std::string payload_, received_;
    unsigned sequence_ = 0;
    bool stopped_ = false, measured_ = false;
};

int RunClients(const Options &options)
{
    const auto begin = Clock::now() + std::chrono::seconds(options.warmup);
    const auto end = begin + std::chrono::seconds(options.duration);
    std::vector<std::unique_ptr<Statistics>> statistics;
    std::vector<std::thread> workers;
    const auto threads = std::min(options.threads, options.connections);
    for (unsigned thread = 0; thread < threads; ++thread)
    {
        statistics.push_back(std::make_unique<Statistics>());
        auto *result = statistics.back().get();
        workers.emplace_back(
            [&, thread, result]
            {
                boost::asio::io_context io;
                std::vector<std::unique_ptr<Client>> clients;
                for (unsigned i = thread; i < options.connections; i += threads)
                {
                    clients.push_back(std::make_unique<Client>(io, options, *result, begin, end, i));
                    clients.back()->Connect();
                }
                boost::asio::steady_timer stop(io, end);
                stop.async_wait(
                    [&](const auto &)
                    {
                        for (auto &client : clients)
                            client->Stop();
                    });
                io.run();
            });
    }
    for (auto &worker : workers)
        worker.join();
    auto total = std::make_unique<Statistics>();
    for (const auto &result : statistics)
    {
        total->started += result->started;
        total->errors += result->errors;
        total->invalid += result->invalid;
        total->connected += result->connected;
        total->messages.Merge(result->messages);
        total->connections.Merge(result->connections);
        if (result->lastErrorCode)
            total->lastErrorCode = result->lastErrorCode;
    }
    const double throughput = total->messages.completed / static_cast<double>(options.duration);
    std::cout << "{\"completed\":" << total->messages.completed << ",\"started\":" << total->started
              << ",\"unfinished\":" << total->started - total->messages.completed
              << ",\"errors\":" << total->errors << ",\"invalid\":" << total->invalid
              << ",\"connections_established\":" << total->connected
              << ",\"duration_seconds\":" << options.duration << ",\"warmup_seconds\":" << options.warmup
              << ",\"connections\":" << options.connections << ",\"client_threads\":" << threads
              << ",\"message_bytes\":" << options.bytes << ",\"reconnect\":" << options.reconnect
              << ",\"reconnect_delay_ms\":" << options.reconnectDelay
              << ",\"last_error_code\":" << total->lastErrorCode
              << ",\"connections_measured\":" << total->connections.completed << ",\"connects_per_second\":"
              << total->connections.completed / static_cast<double>(options.duration)
              << ",\"connect_p50_ms\":" << total->connections.Percentile(.5)
              << ",\"connect_p95_ms\":" << total->connections.Percentile(.95)
              << ",\"connect_p99_ms\":" << total->connections.Percentile(.99)
              << ",\"read_delay_ms\":" << options.readDelay << ",\"messages_per_second\":" << throughput
              << ",\"receive_mib_per_second\":" << throughput * options.bytes / 1048576.0
              << ",\"p50_ms\":" << total->messages.Percentile(0.50)
              << ",\"p95_ms\":" << total->messages.Percentile(0.95)
              << ",\"p99_ms\":" << total->messages.Percentile(0.99)
              << ",\"max_ms\":" << total->messages.maximumMs << ",\"histogram_bucket_ms\":0.1}" << std::endl;
    return total->errors || total->invalid || !total->messages.completed ? 1 : 0;
}
} // namespace

int main(int argc, char **argv)
{
    try
    {
        const auto options = Parse(argc, argv);
        if (options.mode == "client")
            return RunClients(options);
        TcpServer server(static_cast<std::uint16_t>(options.port), options.host);
        server.SetThreadCount(options.threads);
        server.SetIdleTimeout(std::chrono::seconds(30));
        server.SetMessageCallback(
            [](const PtrConnection &connection, Buffer *input)
            {
                connection->Send(input->ReadPositon(), input->ReadAbleSize());
                input->Clear();
                if (connection->PendingOutputBytes() > 2 * 1024 * 1024)
                    connection->Release();
            });
        std::signal(SIGINT, Interrupt);
        std::signal(SIGTERM, Interrupt);
        std::jthread monitor(
            [&](std::stop_token stop)
            {
                const auto deadline = Clock::now() + std::chrono::seconds(options.duration);
                while (!stop.stop_requested() && !interrupted && Clock::now() < deadline)
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

#include "http_server.h"
#include "common.h"
#include <array>
#include <boost/beast/core/buffers_range.hpp>
#include <boost/beast/http.hpp>
#include <boost/mysql/common_server_errc.hpp>
#include <charconv>
#include <iostream>
#include <sstream>
namespace disk
{
namespace http = boost::beast::http;
namespace
{
std::string Decode(const std::string &input, bool query)
{
    std::string output;
    for (std::size_t i = 0; i < input.size(); ++i)
    {
        char c = input[i];
        if (c == '%')
        {
            if (i + 2 >= input.size())
                throw Error(400, "Invalid URL escape");
            unsigned value = 0;
            const auto parsed = std::from_chars(input.data() + i + 1, input.data() + i + 3, value, 16);
            if (parsed.ec != std::errc{} || parsed.ptr != input.data() + i + 3)
                throw Error(400, "Invalid URL escape");
            c = static_cast<char>(value);
            i += 2;
        }
        else if (query && c == '+')
            c = ' ';
        if (!c || c == '\r' || c == '\n')
            throw Error(400, "Invalid URL character");
        output += c;
    }
    return output;
}
Response ExceptionResponse()
{
    try
    {
        throw;
    }
    catch (const Error &error)
    {
        return ErrorResponse(error.status, error.what());
    }
    catch (const boost::system::system_error &error)
    {
        if (error.code() == boost::mysql::common_server_errc::er_dup_entry)
            return ErrorResponse(409, "Name already exists");
        std::cerr << "Database or I/O failure: " << error.code() << '\n';
        return ErrorResponse(503, "Dependency unavailable");
    }
    catch (const std::exception &error)
    {
        std::cerr << "Request failed: " << error.what() << '\n';
        return ErrorResponse(500, "Internal server error");
    }
}
} // namespace
class HttpSession : public std::enable_shared_from_this<HttpSession>
{
  public:
    HttpSession(HttpServer &server, const PtrConnection &connection)
        : server_(server), connection_(connection)
    {
        Reset();
    }
    void Start()
    {
        Arm(std::chrono::seconds(10));
    }
    void Cancel()
    {
        std::lock_guard lock(cancelMutex_);
        cancellation_->Cancel();
        if (pipe_)
            pipe_->Abort();
    }
    void Closed()
    {
        closed_ = true;
        Cancel();
        if (const auto connection = connection_.lock())
        {
            connection->Loop()->CancelTimer(timer_);
            connection->Loop()->CancelTimer(uploadIdleTimer_);
        }
        reader_.reset();
    }
    void Read(Buffer *input)
    {
        if (server_.stopping_)
        {
            Cancel();
            connection_.lock()->Release();
            return;
        }
        if (closed_ || waiting_)
            return;
        try
        {
            while (input->ReadAbleSize() && !waiting_)
            {
                if (pipe_ && pipe_->Full())
                {
                    connection_.lock()->PauseRead();
                    return;
                }
                parser_->get().body().data = block_.data();
                parser_->get().body().size = block_.size();
                boost::system::error_code error;
                const auto consumed =
                    parser_->put(boost::asio::buffer(input->ReadPositon(), input->ReadAbleSize()), error);
                input->MoveReadOffset(consumed);
                if (error && error != http::error::need_more && error != http::error::need_buffer)
                    throw Error(error == http::error::body_limit ? 413 : 400, "Invalid HTTP request");
                if (parser_->is_header_done() && !headerDone_)
                    Prepare();
                if (responding_)
                    return;
                const auto produced = block_.size() - parser_->get().body().size;
                if (produced)
                {
                    if (pipe_)
                    {
                        lastBodyProgress_ = std::chrono::steady_clock::now();
                        if (!pipe_->Push(std::string(block_.data(), produced)))
                            throw Error(400, "Upload stream closed");
                    }
                    else
                    {
                        if (request_.body.size() + produced > 65536)
                            throw Error(413, "JSON request too large");
                        request_.body.append(block_.data(), produced);
                    }
                }
                if (parser_->is_done())
                {
                    waiting_ = true;
                    connection_.lock()->PauseRead();
                    if (pipe_)
                    {
                        connection_.lock()->Loop()->CancelTimer(uploadIdleTimer_);
                        pipe_->Finish();
                    }
                    else
                        Dispatch(false);
                    return;
                }
                if (!consumed)
                    return;
            }
        }
        catch (...)
        {
            Respond(ExceptionResponse(), true);
        }
    }
    void Written()
    {
        if (closed_)
            return;
        if (reader_)
            NextDownload();
        else if (responding_)
            Complete();
    }

  private:
    void Reset()
    {
        parser_ = std::make_unique<http::request_parser<http::buffer_body>>();
        parser_->header_limit(32768);
        parser_->body_limit(server_.config_.maxUploadBytes + 65536);
        request_ = {};
        headerDone_ = waiting_ = responding_ = false;
        std::lock_guard lock(cancelMutex_);
        cancellation_ = std::make_shared<Cancellation>();
        pipe_.reset();
    }
    void Arm(std::chrono::seconds duration)
    {
        const auto connection = connection_.lock();
        if (!connection)
            return;
        connection->Loop()->CancelTimer(timer_);
        std::weak_ptr<HttpSession> weak = shared_from_this();
        timer_ = connection->Loop()->RunAfter(std::chrono::duration_cast<std::chrono::milliseconds>(duration),
                                              [weak]
                                              {
                                                  if (const auto self = weak.lock())
                                                  {
                                                      self->Cancel();
                                                      if (self->responding_)
                                                          self->connection_.lock()->Release();
                                                      else
                                                          self->Respond(ErrorResponse(408, "Request timeout"),
                                                                        true);
                                                  }
                                              });
    }
    void Prepare()
    {
        headerDone_ = true;
        const auto &message = parser_->get();
        if (message.version() != 11 && message.version() != 10)
            throw Error(400, "Unsupported HTTP version");
        request_.method = std::string(message.method_string());
        const std::string target(message.target());
        if (target.empty() || target.front() != '/' || target.size() > 8192)
            throw Error(400, "Invalid request target");
        const auto question = target.find('?');
        request_.path = Decode(target.substr(0, question), false);
        if (question != std::string::npos)
        {
            std::istringstream query(target.substr(question + 1));
            std::string pair;
            while (std::getline(query, pair, '&'))
            {
                if (request_.query.size() >= 32)
                    throw Error(400, "Too many query parameters");
                const auto equal = pair.find('=');
                const auto key = Decode(pair.substr(0, equal), true);
                const auto value = Decode(equal == std::string::npos ? "" : pair.substr(equal + 1), true);
                if (!request_.query.emplace(key, value).second)
                    throw Error(400, "Duplicate query parameter");
            }
        }
        for (const auto &field : message)
        {
            std::string name(field.name_string());
            std::transform(name.begin(), name.end(), name.begin(),
                           [](unsigned char c)
                           {
                               return static_cast<char>(std::tolower(c));
                           });
            if (!request_.headers.emplace(name, std::string(field.value())).second && name == "authorization")
                throw Error(400, "Duplicate authorization");
        }
        keepAlive_ = message.keep_alive();
        Arm(server_.config_.requestTimeout);
        if (request_.method == "OPTIONS")
        {
            waiting_ = true;
            Response response;
            response.status = 204;
            Respond(std::move(response), true);
            return;
        }
        if (request_.path.rfind("/api/", 0) == 0)
            request_.user = server_.auth_.Verify(request_.headers["authorization"]);
        if (request_.headers.contains("expect"))
            throw Error(417, "Expect is unsupported");
        if (request_.method == "POST" && request_.path == "/api/files")
        {
            const auto declared = request_.headers.find("x-upload-size");
            if (declared != request_.headers.end() &&
                ParseUploadSize(declared->second) > server_.config_.maxUploadBytes)
                throw Error(413, "File exceeds upload limit");
            {
                const std::lock_guard lock(cancelMutex_);
                if (cancellation_->Cancelled())
                    throw Error(503, "Server stopping");
                pipe_ = std::make_shared<BodyPipe>();
            }
            if (!server_.AcquireUpload(request_.user))
                throw Error(429, "Concurrent upload limit reached");
            try
            {
                const std::weak_ptr<HttpSession> weak = shared_from_this();
                pipe_->OnConsumed(
                    [weak]
                    {
                        if (auto self = weak.lock())
                            if (auto connection = self->connection_.lock())
                                connection->Loop()->QueueInLoop(
                                    [self, connection]
                                    {
                                        if (!self->closed_ && !self->waiting_)
                                        {
                                            self->lastBodyProgress_ = std::chrono::steady_clock::now();
                                            connection->ResumeRead();
                                        }
                                    });
                    });
                lastBodyProgress_ = std::chrono::steady_clock::now();
                uploadIdleTimer_ = connection_.lock()->Loop()->RunEvery(std::chrono::seconds(1),
                                                                        [weak]
                                                                        {
                                                                            if (auto self = weak.lock())
                                                                                self->CheckUploadIdle();
                                                                        });
                Dispatch(true);
            }
            catch (...)
            {
                server_.ReleaseUpload(request_.user);
                throw;
            }
        }
        else if (parser_->content_length() && *parser_->content_length() > 65536)
            throw Error(413, "Request body too large");
    }
    void Dispatch(bool upload)
    {
        auto self = shared_from_this();
        auto request = request_;
        const auto pipe = pipe_;
        const auto cancel = cancellation_;
        auto *pool = &server_.business_;
        if (upload)
            pool = &server_.uploadWorkers_;
        else if (request.method == "GET" && (request.path == "/livez" || request.path == "/readyz"))
            pool = &server_.healthWorkers_;
        const bool posted = pool->Post(
            [self, request = std::move(request), pipe, cancel, upload]
            {
                Response response;
                try
                {
                    response = self->server_.handler_(request, pipe, cancel);
                }
                catch (...)
                {
                    response = ExceptionResponse();
                }
                if (upload)
                    self->server_.ReleaseUpload(request.user);
                if (const auto connection = self->connection_.lock())
                {
                    connection->Loop()->QueueInLoop(
                        [self, response = std::move(response), cancel]() mutable
                        {
                            if (!self->closed_ && !cancel->Cancelled())
                                self->Respond(std::move(response), false);
                        });
                }
            });
        if (!posted)
        {
            throw Error(503, "Server work queue full");
        }
    }
    void CheckUploadIdle()
    {
        if (closed_ || waiting_ || responding_ || !pipe_ || pipe_->Full())
            return;
        if (std::chrono::steady_clock::now() - lastBodyProgress_ >= server_.config_.uploadIdleTimeout)
        {
            connection_.lock()->Loop()->CancelTimer(uploadIdleTimer_);
            Respond(ErrorResponse(408, "Upload body idle timeout"), true);
        }
    }
    void Respond(Response response, bool close)
    {
        if (closed_ || responding_)
            return;
        waiting_ = responding_ = true;
        const auto connection = connection_.lock();
        connection->PauseRead();
        keepAlive_ = keepAlive_ && !close && response.status < 400 && parser_->is_done();
        if (response.status >= 400)
            Cancel();
        if (!server_.config_.corsOrigin.empty())
        {
            const auto origin = request_.headers.find("origin");
            if (origin != request_.headers.end() && origin->second == server_.config_.corsOrigin)
            {
                response.headers["Access-Control-Allow-Origin"] = origin->second;
                response.headers["Vary"] = "Origin";
                response.headers["Access-Control-Allow-Methods"] = "GET,POST,PUT,PATCH,DELETE,OPTIONS";
                response.headers["Access-Control-Allow-Headers"] =
                    "Authorization,Content-Type,If-Match,X-Upload-Size";
                response.headers["Access-Control-Expose-Headers"] =
                    "ETag,X-File-SHA256,Last-Modified,Content-Disposition";
            }
        }
        response.headers["X-Content-Type-Options"] = "nosniff";
        response.headers["Cache-Control"] = "no-store";
        reader_ = std::move(response.reader);
        if (reader_)
        {
            http::response<http::empty_body> header{static_cast<http::status>(response.status), 11};
            header.keep_alive(keepAlive_);
            header.content_length(reader_->Size());
            for (const auto &[key, value] : response.headers)
                header.set(key, value);
            http::response_serializer<http::empty_body> serializer(header);
            serializer.split(true);
            std::string wire;
            boost::system::error_code error;
            while (!serializer.is_header_done())
            {
                std::size_t bytes = 0;
                serializer.next(error,
                                [&](boost::system::error_code &, const auto &buffers)
                                {
                                    for (const auto &buffer : boost::beast::buffers_range_ref(buffers))
                                    {
                                        wire.append(static_cast<const char *>(buffer.data()), buffer.size());
                                        bytes += buffer.size();
                                    }
                                });
                if (error)
                {
                    connection->Release();
                    return;
                }
                serializer.consume(bytes);
            }
            remaining_ = reader_->Size();
            connection->Send(wire.data(), wire.size());
        }
        else
        {
            http::response<http::string_body> message{static_cast<http::status>(response.status), 11};
            message.keep_alive(keepAlive_);
            for (const auto &[key, value] : response.headers)
                message.set(key, value);
            message.body() = std::move(response.body);
            message.prepare_payload();
            std::ostringstream stream;
            stream << message;
            const auto wire = stream.str();
            connection->Send(wire.data(), wire.size());
        }
    }
    void NextDownload()
    {
        const auto self = shared_from_this();
        const auto reader = reader_;
        const auto cancel = cancellation_;
        if (!server_.downloadWorkers_.Post(
                [self, reader, cancel]
                {
                    std::string block;
                    bool failed = false;
                    try
                    {
                        block = reader->Read();
                    }
                    catch (...)
                    {
                        failed = true;
                    }
                    if (const auto connection = self->connection_.lock())
                        connection->Loop()->QueueInLoop(
                            [self, connection, cancel, failed, block = std::move(block)]
                            {
                                if (self->closed_ || cancel->Cancelled())
                                    return;
                                if (failed || block.size() > self->remaining_)
                                {
                                    connection->Release();
                                    return;
                                }
                                if (block.empty())
                                {
                                    if (self->remaining_)
                                    {
                                        connection->Release();
                                        return;
                                    }
                                    self->reader_.reset();
                                    self->Complete();
                                }
                                else
                                {
                                    self->remaining_ -= block.size();
                                    connection->Send(block.data(), block.size());
                                }
                            });
                }))
            connection_.lock()->Release();
    }
    void Complete()
    {
        const auto connection = connection_.lock();
        if (!keepAlive_)
        {
            connection->Shutdown();
            return;
        }
        Reset();
        Arm(std::chrono::seconds(10));
        connection->ResumeRead();
    }
    HttpServer &server_;
    std::weak_ptr<Connection> connection_;
    std::unique_ptr<http::request_parser<http::buffer_body>> parser_;
    std::array<char, 65536> block_{};
    Request request_;
    std::mutex cancelMutex_;
    std::shared_ptr<Cancellation> cancellation_;
    std::shared_ptr<BodyPipe> pipe_;
    std::shared_ptr<ObjectReader> reader_;
    TimerHandle timer_;
    TimerHandle uploadIdleTimer_;
    std::chrono::steady_clock::time_point lastBodyProgress_;
    bool closed_ = false, headerDone_ = false, waiting_ = false, responding_ = false, keepAlive_ = false;
    std::uint64_t remaining_ = 0;
};
HttpServer::HttpServer(Config config, const Auth &auth, Handler handler)
    : config_(std::move(config)), auth_(auth), handler_(std::move(handler)),
      tcp_(config_.port, config_.listen)
{
    tcp_.SetThreadCount(config_.ioThreads);
    tcp_.SetConnectedCallback(
        [this](const PtrConnection &connection)
        {
            if (stopping_)
            {
                connection->Release();
                return;
            }
            auto session = std::make_shared<HttpSession>(*this, connection);
            connection->SetContext(session);
            {
                const std::lock_guard lock(mutex_);
                if (stopping_)
                {
                    connection->Release();
                    return;
                }
                sessions_[connection->Id()] = session;
            }
            session->Start();
        });
    tcp_.SetMessageCallback(
        [](const PtrConnection &connection, Buffer *input)
        {
            auto *session = std::any_cast<std::shared_ptr<HttpSession>>(&connection->Context());
            if (session)
                (*session)->Read(input);
        });
    tcp_.SetWriteCompleteCallback(
        [](const PtrConnection &connection)
        {
            auto *session = std::any_cast<std::shared_ptr<HttpSession>>(&connection->Context());
            if (session)
                (*session)->Written();
        });
    tcp_.SetClosedCallback(
        [this](const PtrConnection &connection)
        {
            auto *session = std::any_cast<std::shared_ptr<HttpSession>>(&connection->Context());
            if (session)
                (*session)->Closed();
            connection->SetContext({});
            const std::lock_guard lock(mutex_);
            sessions_.erase(connection->Id());
        });
}
HttpServer::~HttpServer()
{
    Stop();
    if (shutdown_.joinable())
        shutdown_.join();
}
void HttpServer::Start()
{
    tcp_.Start();
}
void HttpServer::Stop()
{
    if (stopping_.exchange(true))
        return;
    {
        const std::lock_guard lock(mutex_);
        for (const auto &[id, session] : sessions_)
            session->Cancel();
    }
    shutdown_ = std::thread(
        [this]
        {
            business_.Join();
            uploadWorkers_.Join();
            downloadWorkers_.Join();
            healthWorkers_.Join();
            tcp_.Stop();
        });
}
bool HttpServer::AcquireUpload(std::uint64_t user)
{
    const std::lock_guard lock(mutex_);
    if (activeUploads_ >= config_.maxConcurrentUploads)
        return false;
    auto &count = uploads_[user];
    if (count >= 2)
        return false;
    ++count;
    ++activeUploads_;
    return true;
}
void HttpServer::ReleaseUpload(std::uint64_t user)
{
    const std::lock_guard lock(mutex_);
    const auto found = uploads_.find(user);
    if (found != uploads_.end())
    {
        --activeUploads_;
        if (--found->second == 0)
            uploads_.erase(found);
    }
}
} // namespace disk

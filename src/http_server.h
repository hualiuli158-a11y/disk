#pragma once
#include "auth.h"
#include "common/BusinessPool.h"
#include "http_types.h"
#include "net/tcpserver.h"
#include <atomic>
#include <map>
namespace disk
{
class HttpSession;
class HttpServer
{
  public:
    using Handler = std::function<Response(const Request &, const std::shared_ptr<BodyPipe> &,
                                           const std::shared_ptr<Cancellation> &)>;
    HttpServer(Config config, const Auth &auth, Handler handler);
    ~HttpServer();
    void Start();
    void Stop();
    std::uint16_t Port() const
    {
        return tcp_.Port();
    }

  private:
    friend class HttpSession;
    bool AcquireUpload(std::uint64_t user);
    void ReleaseUpload(std::uint64_t user);
    Config config_;
    const Auth &auth_;
    Handler handler_;
    TcpServer tcp_;
    BusinessPool business_{4, 64};
    BusinessPool healthWorkers_{1, 4};
    BusinessPool uploadWorkers_{4, 64};
    BusinessPool downloadWorkers_{4, 16};
    std::mutex mutex_;
    std::map<std::uint64_t, std::shared_ptr<HttpSession>> sessions_;
    std::map<std::uint64_t, unsigned> uploads_;
    std::size_t activeUploads_ = 0;
    std::atomic<bool> stopping_{false};
    std::thread shutdown_;
};
} // namespace disk

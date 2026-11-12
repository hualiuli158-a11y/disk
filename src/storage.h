#pragma once
#include "config.h"
#include "object_storage.grpc.pb.h"
#include <condition_variable>
#include <deque>
#include <functional>
#include <grpcpp/grpcpp.h>
#include <memory>
#include <mutex>
namespace disk
{
class Cancellation
{
  public:
    void Cancel();
    void Attach(const std::shared_ptr<grpc::ClientContext> &context);
    bool Cancelled() const;

  private:
    mutable std::mutex mutex_;
    bool cancelled_ = false;
    std::weak_ptr<grpc::ClientContext> context_;
};
// Single producer and consumer. The HTTP loop pauses at Capacity blocks.
class BodyPipe
{
  public:
    static constexpr std::size_t Capacity = 4;
    bool Push(std::string block);
    bool Pop(std::string &block);
    bool Full() const;
    void Finish();
    void Abort();
    void OnConsumed(std::function<void()> callback);

  private:
    mutable std::mutex mutex_;
    std::condition_variable ready_;
    std::deque<std::string> blocks_;
    bool finished_ = false, aborted_ = false;
    std::function<void()> consumed_;
};
class ObjectReader
{
  public:
    virtual ~ObjectReader() = default;
    virtual std::uint64_t Size() const = 0;
    virtual std::string Read() = 0;
};
class ObjectStorage
{
  public:
    using Source = std::function<bool(std::string &)>;
    virtual ~ObjectStorage() = default;
    virtual void Check() = 0;
    virtual void Put(const std::string &key, Source source, const std::shared_ptr<Cancellation> &cancel) = 0;
    virtual std::shared_ptr<ObjectReader> Open(const std::string &key,
                                               const std::shared_ptr<Cancellation> &cancel) = 0;
    virtual void Delete(const std::string &key) = 0;
};
class GrpcStorage final : public ObjectStorage
{
  public:
    explicit GrpcStorage(const Config &config);
    void Check() override;
    void Put(const std::string &key, Source source, const std::shared_ptr<Cancellation> &cancel) override;
    std::shared_ptr<ObjectReader> Open(const std::string &key,
                                       const std::shared_ptr<Cancellation> &cancel) override;
    void Delete(const std::string &key) override;

  private:
    Config config_;
    std::unique_ptr<objectstorage::v1::ObjectStorage::Stub> stub_;
};
} // namespace disk

#pragma once
#include "config.h"
#include "gin/gin.h"
#include "object_storage.grpc.pb.h"
#include <condition_variable>
#include <deque>
#include <functional>
#include <grpcpp/grpcpp.h>
#include <memory>
#include <mutex>
namespace disk
{
using Cancellation = gin::Cancellation;
using BodyPipe = gin::BodyPipe;
using ObjectReader = gin::ContentReader;
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

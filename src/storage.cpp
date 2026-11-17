#include "storage.h"
#include "common.h"
#include <fstream>
#include <iostream>
#include <sstream>
namespace disk
{
namespace
{
std::string ReadCertificate(const std::string &path)
{
    std::ifstream file(path, std::ios::binary);
    if (!file)
        throw std::runtime_error("Cannot read storage TLS certificate");
    return std::string(std::istreambuf_iterator<char>(file), {});
}
void CheckStatus(const grpc::Status &status)
{
    if (status.ok())
        return;
    std::cerr << "Storage RPC failed (" << status.error_code() << "): " << status.error_message() << '\n';
    if (status.error_code() == grpc::StatusCode::NOT_FOUND)
        throw Error(404, "Object not found");
    if (status.error_code() == grpc::StatusCode::OUT_OF_RANGE)
        throw Error(413, "Object too large");
    if (status.error_code() == grpc::StatusCode::DEADLINE_EXCEEDED)
        throw Error(504, "Storage timeout");
    throw Error(503, "Object storage unavailable");
}
std::shared_ptr<grpc::ClientContext> Context(const Config &config,
                                             const std::shared_ptr<Cancellation> &cancel)
{
    auto context = std::make_shared<grpc::ClientContext>();
    context->set_deadline(std::chrono::system_clock::now() + config.requestTimeout);
    const std::weak_ptr<grpc::ClientContext> weak = context;
    cancel->OnCancel([weak] {
        if (const auto active = weak.lock()) active->TryCancel();
    });
    return context;
}
class GrpcReader final : public ObjectReader
{
  public:
    GrpcReader(objectstorage::v1::ObjectStorage::Stub &stub, const Config &config, const std::string &key,
               const std::shared_ptr<Cancellation> &cancel)
        : context_(Context(config, cancel))
    {
        objectstorage::v1::GetRequest request;
        request.set_key(key);
        stream_ = stub.Get(context_.get(), request);
        objectstorage::v1::GetReply reply;
        if (!stream_->Read(&reply))
        {
            CheckStatus(stream_->Finish());
            throw Error(502, "Missing object header");
        }
        if (!reply.has_size() || reply.size() > (1ULL << 30))
            throw Error(502, "Invalid object header");
        size_ = remaining_ = reply.size();
    }
    ~GrpcReader() override
    {
        context_->TryCancel();
    }
    std::uint64_t Size() const override
    {
        return size_;
    }
    std::string Read() override
    {
        objectstorage::v1::GetReply reply;
        if (!stream_->Read(&reply))
        {
            CheckStatus(stream_->Finish());
            if (remaining_)
                throw Error(502, "Truncated object");
            return {};
        }
        if (!reply.has_data() || reply.data().empty() || reply.data().size() > 65536 ||
            reply.data().size() > remaining_)
            throw Error(502, "Invalid object chunk");
        remaining_ -= reply.data().size();
        return std::move(*reply.mutable_data());
    }

  private:
    std::shared_ptr<grpc::ClientContext> context_;
    std::unique_ptr<grpc::ClientReader<objectstorage::v1::GetReply>> stream_;
    std::uint64_t size_ = 0, remaining_ = 0;
};
} // namespace
GrpcStorage::GrpcStorage(const Config &config) : config_(config)
{
    std::shared_ptr<grpc::ChannelCredentials> credentials;
    if (config.storageCA.empty())
    {
        const auto &address = config.storageAddress;
        if (address.rfind("127.0.0.1:", 0) != 0 && address.rfind("localhost:", 0) != 0 &&
            address.rfind("[::1]:", 0) != 0)
            throw std::runtime_error("Configure TLS for non-loopback storage");
        credentials = grpc::InsecureChannelCredentials();
    }
    else
    {
        grpc::SslCredentialsOptions options;
        options.pem_root_certs = ReadCertificate(config.storageCA);
        if (config.storageCertificate.empty() != config.storageKey.empty())
            throw std::runtime_error("Set both storage client TLS files");
        if (!config.storageCertificate.empty())
        {
            options.pem_cert_chain = ReadCertificate(config.storageCertificate);
            options.pem_private_key = ReadCertificate(config.storageKey);
        }
        credentials = grpc::SslCredentials(options);
    }
    grpc::ChannelArguments arguments;
    if (!config.storageServerName.empty())
        arguments.SetSslTargetNameOverride(config.storageServerName);
    stub_ = objectstorage::v1::ObjectStorage::NewStub(
        grpc::CreateCustomChannel(config.storageAddress, credentials, arguments));
}
void GrpcStorage::Check()
{
    grpc::ClientContext context;
    context.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(3));
    objectstorage::v1::ListRequest request;
    request.set_prefix("objects");
    request.set_limit(1);
    auto stream = stub_->List(&context, request);
    objectstorage::v1::ListReply reply;
    while (stream->Read(&reply))
    {
    }
    CheckStatus(stream->Finish());
}
void GrpcStorage::Put(const std::string &key, Source source, const std::shared_ptr<Cancellation> &cancel)
{
    auto context = Context(config_, cancel);
    objectstorage::v1::PutReply reply;
    auto stream = stub_->Put(context.get(), &reply);
    struct CancelContext
    {
        grpc::ClientContext &context;
        ~CancelContext()
        {
            context.TryCancel();
        }
    } cancelOnExit{*context};
    objectstorage::v1::PutRequest request;
    request.set_key(key);
    if (!stream->Write(request))
    {
        CheckStatus(stream->Finish());
        throw Error(503, "Storage stream closed");
    }
    std::string block;
    std::uint64_t total = 0;
    while (source(block))
    {
        if (block.empty())
            continue;
        if (block.size() > 65536 || block.size() > config_.maxUploadBytes - total)
            throw Error(413, "File exceeds upload limit");
        total += block.size();
        request.set_data(std::move(block));
        if (!stream->Write(request))
        {
            CheckStatus(stream->Finish());
            throw Error(503, "Storage stream closed");
        }
    }
    stream->WritesDone();
    CheckStatus(stream->Finish());
    if (reply.size() != total)
        throw Error(502, "Stored size mismatch");
}
std::shared_ptr<ObjectReader> GrpcStorage::Open(const std::string &key,
                                                const std::shared_ptr<Cancellation> &cancel)
{
    return std::make_shared<GrpcReader>(*stub_, config_, key, cancel);
}
void GrpcStorage::Delete(const std::string &key)
{
    grpc::ClientContext context;
    context.set_deadline(std::chrono::system_clock::now() + std::chrono::seconds(5));
    objectstorage::v1::DeleteRequest request;
    request.set_key(key);
    objectstorage::v1::DeleteReply reply;
    const auto status = stub_->Delete(&context, request, &reply);
    if (status.error_code() != grpc::StatusCode::NOT_FOUND)
        CheckStatus(status);
}
} // namespace disk

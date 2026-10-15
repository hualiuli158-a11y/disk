#include "object_storage_service.h"

#include <algorithm>
#include <array>
#include <cstdint>
#include <exception>
#include <limits>
#include <memory>
#include <string>
#include <utility>

namespace
{
grpc::Status ToGrpcStatus(const storage::StoreError& error)
{
    using storage::StoreStatus;
    grpc::StatusCode code = grpc::StatusCode::INTERNAL;
    switch (error.status())
    {
    case StoreStatus::InvalidRequest: code = grpc::StatusCode::INVALID_ARGUMENT; break;
    case StoreStatus::NotFound: code = grpc::StatusCode::NOT_FOUND; break;
    case StoreStatus::TooLarge: code = grpc::StatusCode::OUT_OF_RANGE; break;
    case StoreStatus::NoSpace: code = grpc::StatusCode::RESOURCE_EXHAUSTED; break;
    case StoreStatus::IoError: code = grpc::StatusCode::INTERNAL; break;
    }
    return {code, error.what()};
}

grpc::Status InternalError(const std::exception& error)
{
    return {grpc::StatusCode::INTERNAL, error.what()};
}

grpc::Status Cancelled()
{
    return {grpc::StatusCode::CANCELLED, "request cancelled"};
}
} // namespace

grpc::Status ObjectStorageService::Put(
    grpc::ServerContext* context,
    grpc::ServerReader<objectstorage::v1::PutRequest>* reader,
    objectstorage::v1::PutReply* reply)
{
    try
    {
        objectstorage::v1::PutRequest message;
        if (!reader->Read(&message) ||
            message.payload_case() != objectstorage::v1::PutRequest::kKey)
            return {grpc::StatusCode::INVALID_ARGUMENT, "first message must contain key"};
        auto upload = store_.BeginPut(message.key());
        while (reader->Read(&message))
        {
            if (context->IsCancelled()) return Cancelled();
            if (message.payload_case() != objectstorage::v1::PutRequest::kData)
                return {grpc::StatusCode::INVALID_ARGUMENT, "only data may follow key"};
            if (message.data().size() > 64 * 1024)
                return {grpc::StatusCode::OUT_OF_RANGE, "chunk exceeds 64 KiB"};
            upload.Write(message.data());
        }
        if (context->IsCancelled()) return Cancelled();
        const auto size = upload.size();
        upload.Commit(size);
        reply->set_size(size);
        return grpc::Status::OK;
    }
    catch (const storage::StoreError& error) { return ToGrpcStatus(error); }
    catch (const std::exception& error) { return InternalError(error); }
}

grpc::Status ObjectStorageService::Get(
    grpc::ServerContext* context,
    const objectstorage::v1::GetRequest* request,
    grpc::ServerWriter<objectstorage::v1::GetReply>* writer)
{
    try
    {
        auto object = store_.Open(request->key());
        objectstorage::v1::GetReply header;
        header.set_size(object.size());
        if (!writer->Write(header)) return Cancelled();

        std::array<char, 64 * 1024> buffer{};
        std::uint64_t sent = 0;
        while (sent < object.size())
        {
            if (context->IsCancelled()) return Cancelled();
            const auto capacity = static_cast<std::size_t>(
                std::min<std::uint64_t>(buffer.size(), object.size() - sent));
            const auto count = object.Read(buffer.data(), capacity);
            if (count == 0)
                return {grpc::StatusCode::DATA_LOSS, "object ended before declared size"};
            objectstorage::v1::GetReply chunk;
            chunk.set_data(buffer.data(), count);
            if (!writer->Write(chunk)) return Cancelled();
            sent += count;
        }
        return grpc::Status::OK;
    }
    catch (const storage::StoreError& error) { return ToGrpcStatus(error); }
    catch (const std::exception& error) { return InternalError(error); }
}

grpc::Status ObjectStorageService::Delete(
    grpc::ServerContext* context,
    const objectstorage::v1::DeleteRequest* request,
    objectstorage::v1::DeleteReply*)
{
    if (context->IsCancelled()) return Cancelled();
    try
    {
        store_.Delete(request->key());
        return grpc::Status::OK;
    }
    catch (const storage::StoreError& error) { return ToGrpcStatus(error); }
    catch (const std::exception& error) { return InternalError(error); }
}

grpc::Status ObjectStorageService::List(
    grpc::ServerContext* context,
    const objectstorage::v1::ListRequest* request,
    grpc::ServerWriter<objectstorage::v1::ListReply>* writer)
{
    if (context->IsCancelled()) return Cancelled();
    try
    {
        if (request->limit() == 0 && !request->after_key().empty())
            return {grpc::StatusCode::INVALID_ARGUMENT, "list cursor requires a page limit"};
        const auto objects = request->limit() == 0 ?
            store_.List(request->prefix()) :
            store_.ListPage(request->prefix(), request->after_key(), request->limit());
        for (const auto& object : objects)
        {
            if (context->IsCancelled()) return Cancelled();
            objectstorage::v1::ListReply item;
            item.set_key(object.key);
            item.set_size(object.size);
            item.set_modified_at_unix_ms(object.modified_at_unix_ms);
            if (!writer->Write(item)) return Cancelled();
        }
        return grpc::Status::OK;
    }
    catch (const storage::StoreError& error) { return ToGrpcStatus(error); }
    catch (const std::exception& error) { return InternalError(error); }
}

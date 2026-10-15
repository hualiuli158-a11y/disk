#ifndef GRPC_OBJECT_STORAGE_SERVICE_H
#define GRPC_OBJECT_STORAGE_SERVICE_H

#include "object_storage.grpc.pb.h"
#include "storage/local_store.h"

class ObjectStorageService final : public objectstorage::v1::ObjectStorage::Service
{
public:
    explicit ObjectStorageService(const std::string& root) : store_(root) {}

    grpc::Status Put(grpc::ServerContext* context,
                     grpc::ServerReader<objectstorage::v1::PutRequest>* reader,
                     objectstorage::v1::PutReply* reply) override;
    grpc::Status Get(grpc::ServerContext* context,
                     const objectstorage::v1::GetRequest* request,
                     grpc::ServerWriter<objectstorage::v1::GetReply>* writer) override;
    grpc::Status Delete(grpc::ServerContext* context,
                        const objectstorage::v1::DeleteRequest* request,
                        objectstorage::v1::DeleteReply* reply) override;
    grpc::Status List(grpc::ServerContext* context,
                      const objectstorage::v1::ListRequest* request,
                      grpc::ServerWriter<objectstorage::v1::ListReply>* writer) override;

private:
    storage::LocalStore store_;
};

#endif

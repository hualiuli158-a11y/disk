#pragma once
#include "database.h"
#include "models.h"
#include <functional>

namespace disk
{
class FileRepository
{
  public:
    explicit FileRepository(Database &database) : database_(database) {}
    void CheckUpload(std::uint64_t user, std::uint64_t folder, std::optional<std::uint64_t> size);
    UploadCommit CreateUploadedFile(const UploadedObject &object);
    std::optional<StoredFile> FindForDownload(std::uint64_t user, std::uint64_t file);
    void Rename(std::uint64_t user, std::uint64_t file, const std::string &name);
    void Move(std::uint64_t user, std::uint64_t file, std::uint64_t folder);
    // The callback runs while the unique storage key remains locked by the transaction.
    void WithUnreferencedCandidate(const std::string &key, const std::function<void()> &cleanup);

  private:
    UploadCommit CreateUploadedFileAttempt(const UploadedObject &object);
    Database &database_;
};
} // namespace disk

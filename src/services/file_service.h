#pragma once
#include "repositories/file_repository.h"
#include "storage.h"

namespace disk
{
struct UploadInput
{
    std::uint64_t user;
    std::uint64_t folder;
    std::string name;
    std::optional<std::string> declaredSize;
    ObjectStorage::Source content;
};

struct FileDownload
{
    StoredFile file;
    std::shared_ptr<ObjectReader> reader;
};

class FileService
{
  public:
    FileService(FileRepository &files, ObjectStorage &storage) : files_(files), storage_(storage) {}
    UploadedFile Upload(const UploadInput &input, const std::shared_ptr<Cancellation> &cancel);
    FileDownload Download(std::uint64_t user, std::uint64_t file,
                          const std::shared_ptr<Cancellation> &cancel);
    std::string Rename(std::uint64_t user, std::uint64_t file, const std::string &name);
    void Move(std::uint64_t user, std::uint64_t file, std::uint64_t folder);

  private:
    void CleanupCandidate(const std::string &key) noexcept;
    FileRepository &files_;
    ObjectStorage &storage_;
};
} // namespace disk

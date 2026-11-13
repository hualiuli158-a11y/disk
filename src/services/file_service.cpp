#include "file_service.h"
#include "common.h"
#include <iostream>
#include <openssl/evp.h>
#include <openssl/rand.h>

namespace disk
{
namespace
{
std::string NewKey()
{
    unsigned char bytes[24];
    if (RAND_bytes(bytes, sizeof(bytes)) != 1)
        throw std::runtime_error("Random key generation failed");
    const auto value = Hex(bytes, sizeof(bytes));
    return "objects/" + value.substr(0, 2) + "/" + value;
}
} // namespace

UploadedFile FileService::Upload(const UploadInput &input, const std::shared_ptr<Cancellation> &cancel)
{
    const auto name = NormalizeName(input.name);
    std::optional<std::uint64_t> declaredSize;
    if (input.declaredSize)
        declaredSize = ParseUploadSize(*input.declaredSize);
    // The transaction at commit time rechecks ownership and quota after transfer.
    files_.CheckUpload(input.user, input.folder, declaredSize);
    const std::unique_ptr<EVP_MD_CTX, decltype(&EVP_MD_CTX_free)> digest(EVP_MD_CTX_new(), EVP_MD_CTX_free);
    if (!digest || EVP_DigestInit_ex(digest.get(), EVP_sha256(), nullptr) != 1)
        throw std::runtime_error("SHA256 initialization failed");
    const auto key = NewKey();
    bool committed = false;
    std::uint64_t size = 0;
    try
    {
        storage_.Put(
            key,
            [&](std::string &block)
            {
                if (!input.content(block))
                    return false;
                size += block.size();
                if (EVP_DigestUpdate(digest.get(), block.data(), block.size()) != 1)
                    throw std::runtime_error("SHA256 update failed");
                return true;
            },
            cancel);
        if (cancel->Cancelled())
            throw Error(499, "Upload cancelled");
        if (declaredSize && *declaredSize != size)
            throw Error(400, "Upload size mismatch");
        unsigned char bytes[EVP_MAX_MD_SIZE];
        unsigned int length = 0;
        if (EVP_DigestFinal_ex(digest.get(), bytes, &length) != 1)
            throw std::runtime_error("SHA256 finalization failed");
        const UploadedObject object{input.user, input.folder, name, Hex(bytes, length), size, key};
        auto result = files_.CreateUploadedFile(object);
        committed = true;
        if (!result.adopted)
            CleanupCandidate(key);
        return std::move(result.file);
    }
    catch (...)
    {
        if (!committed)
            CleanupCandidate(key);
        throw;
    }
}

FileDownload FileService::Download(std::uint64_t user, std::uint64_t file,
                                   const std::shared_ptr<Cancellation> &cancel)
{
    auto stored = files_.FindForDownload(user, file);
    if (!stored)
        throw Error(404, "File not found");
    auto reader = storage_.Open(stored->storageKey, cancel);
    if (reader->Size() != stored->size)
        throw Error(502, "Stored size mismatch");
    return {std::move(*stored), std::move(reader)};
}

void FileService::CleanupCandidate(const std::string &key) noexcept
{
    try
    {
        files_.WithUnreferencedCandidate(key,
                                         [&]
                                         {
                                             storage_.Delete(key);
                                         });
    }
    catch (...)
    {
        std::cerr << "Candidate object cleanup deferred\n";
    }
}

std::string FileService::Rename(std::uint64_t user, std::uint64_t file, const std::string &name)
{
    const auto normalized = NormalizeName(name);
    files_.Rename(user, file, normalized);
    return normalized;
}
void FileService::Move(std::uint64_t user, std::uint64_t file, std::uint64_t folder)
{
    files_.Move(user, file, folder);
}
} // namespace disk

#include "file_handler.h"
#include "multipart_upload.h"
#include "handler_utils.h"

#include <cctype>

namespace disk::handler {

void FileHandler::Upload(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto contentType = c.GetHeader("content-type");
    if (contentType.rfind("multipart/form-data", 0) != 0)
    {
        throw Error(400, "Expected multipart/form-data");
    }

    const auto& pipe = c.BodyStream();
    if (!pipe)
    {
        throw Error(400, "Missing upload pipe");
    }

    MultipartUpload multipart(pipe, Parameter(contentType, "boundary"));
    UploadInput input;
    input.user = userId;
    input.folder = multipart.folder;
    input.name = multipart.name;

    const auto declared = c.GetHeader("x-upload-size");
    if (!declared.empty())
    {
        input.declaredSize = declared;
    }

    input.content = [&](std::string& block) {
        return multipart.Read(block);
    };

    const auto file = files_.Upload(input, c.Cancellation());

    c.JSON(200, gin::H{
        {"id", file.id},
        {"folder_id", file.folderId},
        {"name", file.name},
        {"size", file.size},
        {"sha256", file.sha256},
        {"created_at", file.createdAt}
    });
}

void FileHandler::Download(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto fileId = Id(c.Param("id"));

    auto download = files_.Download(userId, fileId, c.Cancellation());

    c.Header("Content-Type", "application/octet-stream");
    c.Header("ETag", "\"" + download.file.sha256 + "\"");
    c.Header("X-File-SHA256", download.file.sha256);

    std::string escaped;
    for (unsigned char ch : download.file.name)
    {
        if (std::isalnum(ch) && ch < 128)
            escaped += static_cast<char>(ch);
        else
            escaped += "%" + Hex(&ch, 1);
    }
    c.Header("Content-Disposition", "attachment; filename*=UTF-8''" + escaped);

    c.Status(200);
    c.DataFromReader(200, "application/octet-stream", std::move(download.reader));
}

void FileHandler::Rename(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto fileId = Id(c.Param("id"));

    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("name") || !body["name"].is_string())
    {
        throw Error(400, "Name required");
    }

    const auto name = body["name"].get<std::string>();
    const auto normalized = files_.Rename(userId, fileId, name);

    c.JSON(200, gin::H{
        {"id", fileId},
        {"name", normalized}
    });
}

void FileHandler::Move(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto fileId = Id(c.Param("id"));

    nlohmann::json body;
    if (!c.ShouldBindJSON(body) || !body.contains("folder_id") || !body["folder_id"].is_number_unsigned())
    {
        throw Error(400, "Target folder required");
    }

    const auto target = body["folder_id"].get<std::uint64_t>();
    files_.Move(userId, fileId, target);

    c.JSON(200, gin::H{
        {"id", fileId},
        {"folder_id", target}
    });
}

void FileHandler::Delete(gin::Context& c)
{
    const auto userId = UserID(c);
    const auto fileId = Id(c.Param("id"));

    trash_.DeleteFile(userId, fileId);
    c.Status(204);
}

} // namespace disk::handler

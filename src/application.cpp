#include "application.h"
#include <cctype>
#include <multipart_parser.h>
#include <regex>
namespace disk
{
namespace
{
std::string Parameter(const std::string &header, const std::string &key)
{
    std::size_t position = header.find(';');
    while (position != std::string::npos && ++position < header.size())
    {
        while (position < header.size() && header[position] == ' ')
            ++position;
        const auto equal = header.find('=', position);
        if (equal == std::string::npos)
            return {};
        auto name = header.substr(position, equal - position);
        while (!name.empty() && name.back() == ' ')
            name.pop_back();
        position = equal + 1;
        std::string value;
        if (position < header.size() && header[position] == '"')
        {
            ++position;
            bool closed = false;
            while (position < header.size())
            {
                char c = header[position++];
                if (c == '"')
                {
                    closed = true;
                    break;
                }
                if (c == '\\' && position < header.size())
                    c = header[position++];
                value += c;
            }
            if (!closed)
                throw Error(400, "Malformed quoted parameter");
        }
        else
        {
            const auto end = header.find(';', position);
            value = header.substr(position, end - position);
            position = end;
        }
        if (name == key)
            return value;
        position = header.find(';', position);
    }
    return {};
}
Json::Value FolderJson(const Folder &folder)
{
    Json::Value value;
    value["id"] = Json::UInt64(folder.id);
    value["parent_id"] =
        folder.parentId ? Json::Value(Json::UInt64(*folder.parentId)) : Json::Value(Json::nullValue);
    value["name"] = folder.name;
    value["created_at"] = folder.createdAt;
    return value;
}
Json::Value FolderListJson(const FolderListing &listing, const std::string &parentKey)
{
    Json::Value value;
    value[parentKey] = FolderJson(listing.folder);
    value["children"] = Json::arrayValue;
    for (const auto &folder : listing.children)
        value["children"].append(FolderJson(folder));
    return value;
}
Json::Value ContentsJson(const ContentsPage &contents)
{
    Json::Value value;
    value["items"] = Json::arrayValue;
    for (const auto &item : contents.items)
    {
        Json::Value entry;
        entry["id"] = Json::UInt64(item.id);
        entry["type"] = item.type;
        entry["parent_id"] = Json::UInt64(item.parentId);
        entry["name"] = item.name;
        if (item.size)
            entry["size"] = Json::Int64(*item.size);
        if (item.sha256)
            entry["sha256"] = *item.sha256;
        entry["created_at"] = item.createdAt;
        entry["updated_at"] = item.updatedAt;
        value["items"].append(entry);
    }
    value["page"] = Json::UInt64(contents.page);
    value["page_size"] = Json::UInt64(contents.pageSize);
    value["total"] = Json::Int64(contents.total);
    value["total_pages"] = Json::UInt64((contents.total + contents.pageSize - 1) / contents.pageSize);
    return value;
}
std::string QueryValue(const Request &request, const std::string &key, std::string fallback)
{
    const auto found = request.query.find(key);
    return found == request.query.end() ? fallback : found->second;
}
// Multipart parsing runs on a transfer worker. Only one 64 KiB input block is decoded at a time.
class MultipartUpload
{
  public:
    MultipartUpload(std::shared_ptr<BodyPipe> pipe, const std::string &boundary) : pipe_(std::move(pipe))
    {
        if (boundary.empty() || boundary.size() > 200 || boundary.find_first_of("\r\n") != std::string::npos)
            throw Error(400, "Invalid multipart boundary");
        callbacks_.on_part_data_begin = Begin;
        callbacks_.on_header_field = HeaderField;
        callbacks_.on_header_value = HeaderValue;
        callbacks_.on_headers_complete = HeadersComplete;
        callbacks_.on_part_data = Data;
        callbacks_.on_part_data_end = End;
        callbacks_.on_body_end = Done;
        const auto delimiter = "--" + boundary;
        parser_.reset(multipart_parser_init(delimiter.c_str(), &callbacks_));
        if (!parser_)
            throw std::bad_alloc();
        multipart_parser_set_data(parser_.get(), this);
        while (!fileStarted_)
        {
            if (!ParseNext())
                throw Error(400, "Missing upload file");
        }
    }
    std::uint64_t folder = 0;
    std::string name;
    bool Read(std::string &block)
    {
        while (pending_.empty())
        {
            if (!ParseNext())
            {
                if (!done_ || !fileDone_)
                    throw Error(400, "Incomplete multipart upload");
                return false;
            }
        }
        block = std::move(pending_.front());
        pending_.pop_front();
        return true;
    }

  private:
    static MultipartUpload &Self(multipart_parser *parser)
    {
        return *static_cast<MultipartUpload *>(multipart_parser_get_data(parser));
    }
    template <class Operation> static int Invoke(multipart_parser *parser, Operation operation) noexcept
    {
        auto &self = Self(parser);
        try
        {
            operation(self);
            return 0;
        }
        catch (...)
        {
            self.error_ = std::current_exception();
            return 1;
        }
    }
    static int Begin(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          if (++s.parts_ > 2)
                              throw Error(400, "Unexpected multipart part");
                          s.field_.clear();
                          s.value_.clear();
                          s.disposition_.clear();
                          s.fieldData_.clear();
                          s.inValue_ = false;
                      });
    }
    void SaveHeader()
    {
        if (field_ == "Content-Disposition" || field_ == "content-disposition")
            disposition_ = value_;
        field_.clear();
        value_.clear();
    }
    static int HeaderField(multipart_parser *p, const char *at, size_t length)
    {
        return Invoke(p,
                      [&](auto &s)
                      {
                          if (s.inValue_)
                          {
                              s.SaveHeader();
                              s.inValue_ = false;
                          }
                          s.field_.append(at, length);
                          if ((s.headerBytes_ += length) > 8192)
                              throw Error(400, "Multipart headers too large");
                      });
    }
    static int HeaderValue(multipart_parser *p, const char *at, size_t length)
    {
        return Invoke(p,
                      [&](auto &s)
                      {
                          s.inValue_ = true;
                          s.value_.append(at, length);
                          if ((s.headerBytes_ += length) > 8192)
                              throw Error(400, "Multipart headers too large");
                      });
    }
    static int HeadersComplete(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          s.SaveHeader();
                          s.partName_ = Parameter(s.disposition_, "name");
                          if (s.partName_ == "folder_id" && s.parts_ == 1)
                              return;
                          if (s.partName_ != "file" || s.parts_ != 2 || !s.folder)
                              throw Error(400, "Expected folder_id followed by file");
                          s.name = Parameter(s.disposition_, "filename");
                          s.fileStarted_ = true;
                      });
    }
    static int Data(multipart_parser *p, const char *at, size_t length)
    {
        return Invoke(p,
                      [&](auto &s)
                      {
                          if (s.partName_ == "folder_id")
                          {
                              if (s.fieldData_.size() + length > 20)
                                  throw Error(400, "Invalid folder identifier");
                              s.fieldData_.append(at, length);
                          }
                          else
                          {
                              // MIME callbacks may split at every CR byte. Coalesce fragments before RPC
                              // writes.
                              while (length)
                              {
                                  if (s.pending_.empty() || s.pending_.back().size() == 65536)
                                      s.pending_.emplace_back();
                                  const auto take = std::min(length, 65536 - s.pending_.back().size());
                                  s.pending_.back().append(at, take);
                                  at += take;
                                  length -= take;
                              }
                          }
                      });
    }
    static int End(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          if (s.partName_ == "folder_id")
                              s.folder = Id(s.fieldData_);
                          else
                              s.fileDone_ = true;
                      });
    }
    static int Done(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          s.done_ = true;
                      });
    }
    bool ParseNext()
    {
        std::string input;
        if (!pipe_->Pop(input))
            return false;
        const auto parsed = multipart_parser_execute(parser_.get(), input.data(), input.size());
        if (error_)
            std::rethrow_exception(error_);
        if (parsed != input.size())
            throw Error(400, "Malformed multipart upload");
        return true;
    }
    std::shared_ptr<BodyPipe> pipe_;
    multipart_parser_settings callbacks_{};
    std::unique_ptr<multipart_parser, decltype(&multipart_parser_free)> parser_{nullptr,
                                                                                multipart_parser_free};
    std::deque<std::string> pending_;
    std::exception_ptr error_;
    std::string field_, value_, disposition_, fieldData_, partName_;
    std::size_t headerBytes_ = 0;
    int parts_ = 0;
    bool inValue_ = false, fileStarted_ = false, fileDone_ = false, done_ = false;
};
} // namespace
Response Application::Account(const Request &request)
{
    const auto body = ParseJson(request.body);
    if (!body["username"].isString() || !body["password"].isString())
        throw Error(400, "Username and password required");
    const auto username = body["username"].asString();
    const auto password = body["password"].asString();
    Json::Value result;
    result["access_token"] = request.path == "/register" ? accounts_.Register(username, password)
                                                         : accounts_.Login(username, password);
    return JsonResponse(result);
}
Response Application::Profile(std::uint64_t user, bool quota)
{
    Json::Value value;
    if (quota)
    {
        const auto result = accounts_.GetQuota(user);
        value["used"] = Json::Int64(result.used);
        value["limit"] = Json::Int64(result.limit);
        value["remaining"] = Json::Int64(result.remaining);
    }
    else
    {
        const auto profile = accounts_.Profile(user);
        value["id"] = Json::UInt64(profile.id);
        value["username"] = profile.username;
        value["used_quota"] = Json::Int64(profile.usedQuota);
        value["quota_limit"] = Json::Int64(profile.quotaLimit);
    }
    return JsonResponse(value);
}
Response Application::Root(std::uint64_t user)
{
    return JsonResponse(FolderListJson(folders_.Root(user), "root"));
}
Response Application::CreateFolder(const Request &request)
{
    const auto body = ParseJson(request.body);
    if (!body["parent_id"].isUInt64() || !body["parent_id"].asUInt64() || !body["name"].isString())
        throw Error(400, "Parent and name required");
    return JsonResponse(
        FolderJson(folders_.Create(request.user, body["parent_id"].asUInt64(), body["name"].asString())));
}
Response Application::Contents(const Request &request, std::uint64_t folderId, bool childrenOnly)
{
    if (childrenOnly)
        return JsonResponse(FolderListJson(folders_.Children(request.user, folderId), "folder"));
    const ContentsQuery query{QueryValue(request, "page", "1"), QueryValue(request, "page_size", "100"),
                              QueryValue(request, "sort", "name"), QueryValue(request, "order", "asc")};
    const auto contents = folders_.Contents(request.user, folderId, query);
    return JsonResponse(ContentsJson(contents));
}
Response Application::Upload(const Request &request, const std::shared_ptr<BodyPipe> &pipe,
                             const std::shared_ptr<Cancellation> &cancel)
{
    const auto found = request.headers.find("content-type");
    if (found == request.headers.end() || found->second.rfind("multipart/form-data", 0) != 0)
        throw Error(400, "Expected multipart/form-data");
    MultipartUpload multipart(pipe, Parameter(found->second, "boundary"));
    UploadInput input;
    input.user = request.user;
    input.folder = multipart.folder;
    input.name = multipart.name;
    const auto declared = request.headers.find("x-upload-size");
    if (declared != request.headers.end())
        input.declaredSize = declared->second;
    input.content = [&](std::string &block)
    {
        return multipart.Read(block);
    };
    const auto file = files_.Upload(input, cancel);
    Json::Value value;
    value["id"] = Json::UInt64(file.id);
    value["folder_id"] = Json::UInt64(file.folderId);
    value["name"] = file.name;
    value["size"] = Json::UInt64(file.size);
    value["sha256"] = file.sha256;
    value["created_at"] = file.createdAt;
    return JsonResponse(value);
}
Response Application::Download(std::uint64_t user, std::uint64_t file,
                               const std::shared_ptr<Cancellation> &cancel)
{
    auto download = files_.Download(user, file, cancel);
    Response response;
    response.reader = std::move(download.reader);
    response.headers["Content-Type"] = "application/octet-stream";
    response.headers["ETag"] = "\"" + download.file.sha256 + "\"";
    response.headers["X-File-SHA256"] = download.file.sha256;
    std::string escaped;
    for (unsigned char c : download.file.name)
    {
        if (std::isalnum(c) && c < 128)
            escaped += static_cast<char>(c);
        else
            escaped += "%" + Hex(&c, 1);
    }
    response.headers["Content-Disposition"] = "attachment; filename*=UTF-8''" + escaped;
    return response;
}
Response Application::Rename(const Request &request, std::uint64_t id, bool folder)
{
    const auto body = ParseJson(request.body);
    if (!body["name"].isString())
        throw Error(400, "Name required");
    const auto name = body["name"].asString();
    const auto normalized =
        folder ? folders_.Rename(request.user, id, name) : files_.Rename(request.user, id, name);
    Json::Value result;
    result["id"] = Json::UInt64(id);
    result["name"] = normalized;
    return JsonResponse(result);
}
Response Application::Move(const Request &request, std::uint64_t id, bool folder)
{
    const auto body = ParseJson(request.body);
    const std::string key = folder ? "parent_id" : "folder_id";
    if (!body[key].isUInt64() || !body[key].asUInt64())
        throw Error(400, "Target folder required");
    const auto target = body[key].asUInt64();
    if (folder)
        folders_.Move(request.user, id, target);
    else
        files_.Move(request.user, id, target);
    Json::Value result;
    result["id"] = Json::UInt64(id);
    result[key] = Json::UInt64(target);
    return JsonResponse(result);
}
Response Application::Search(const Request &request)
{
    SearchQuery query;
    query.text = QueryValue(request, "q", "");
    query.type = QueryValue(request, "type", "all");
    query.contents = {QueryValue(request, "page", "1"), QueryValue(request, "page_size", "50"),
                      QueryValue(request, "sort", "name"), QueryValue(request, "order", "asc")};
    const auto result = search_.Search(request.user, std::move(query));
    auto value = ContentsJson(result.contents);
    value["query"] = result.query.text;
    value["type"] = result.query.type;
    value["sort"] = result.query.contents.sort;
    value["order"] = result.query.contents.order;
    return JsonResponse(value);
}
Response Application::Trash(const Request &request)
{
    const auto result =
        trash_.List(request.user, QueryValue(request, "page", "1"), QueryValue(request, "page_size", "50"));
    Json::Value value;
    value["items"] = Json::arrayValue;
    for (const auto &entry : result.items)
    {
        Json::Value item;
        item["id"] = Json::UInt64(entry.id);
        item["type"] = entry.type;
        item["resource_id"] = Json::UInt64(entry.resourceId);
        item["original_parent_id"] = entry.originalParentId
                                         ? Json::Value(Json::UInt64(*entry.originalParentId))
                                         : Json::Value(Json::nullValue);
        item["name"] = entry.name;
        item["trashed_at"] = entry.trashedAt;
        item["expires_at"] = entry.expiresAt;
        value["items"].append(item);
    }
    value["page"] = Json::UInt64(result.page);
    value["page_size"] = Json::UInt64(result.pageSize);
    value["total"] = Json::Int64(result.total);
    value["total_pages"] = Json::UInt64((result.total + result.pageSize - 1) / result.pageSize);
    return JsonResponse(value);
}
Response Application::Handle(const Request &request, const std::shared_ptr<Cancellation> &cancel)
{
    if (request.method == "GET" && request.path == "/livez")
    {
        Json::Value value;
        value["status"] = "alive";
        return JsonResponse(value);
    }
    if (request.method == "GET" && request.path == "/readyz")
    {
        database_.Check();
        storage_.Check();
        Json::Value value;
        value["status"] = "ready";
        return JsonResponse(value);
    }
    if (request.method == "POST" && (request.path == "/login" || request.path == "/register"))
        return Account(request);
    if (request.method == "GET" && request.path == "/api/profile")
        return Profile(request.user, false);
    if (request.method == "GET" && request.path == "/api/quota")
        return Profile(request.user, true);
    if (request.method == "GET" && request.path == "/api/folders/root/children")
        return Root(request.user);
    if (request.method == "POST" && request.path == "/api/folders")
        return CreateFolder(request);
    std::smatch match;
    if (request.method == "GET" &&
        std::regex_match(request.path, match, std::regex("/api/folders/([0-9]+)/(contents|children)")))
        return Contents(request, Id(match[1]), match[2] == "children");
    if (request.method == "GET" &&
        std::regex_match(request.path, match, std::regex("/api/files/([0-9]+)/download")))
        return Download(request.user, Id(match[1]), cancel);
    if (request.method == "GET" && request.path == "/api/search")
        return Search(request);
    if (request.method == "GET" && request.path == "/api/trash")
        return Trash(request);
    if (request.method == "PATCH" &&
        std::regex_match(request.path, match, std::regex("/api/(files|folders)/([0-9]+)")))
        return Rename(request, Id(match[2]), match[1] == "folders");
    if (request.method == "PATCH" &&
        std::regex_match(request.path, match, std::regex("/api/(files|folders)/([0-9]+)/move")))
        return Move(request, Id(match[2]), match[1] == "folders");
    if (request.method == "DELETE" &&
        std::regex_match(request.path, match, std::regex("/api/(files|folders)/([0-9]+)")))
    {
        if (match[1] == "folders")
            trash_.DeleteFolder(request.user, Id(match[2]));
        else
            trash_.DeleteFile(request.user, Id(match[2]));
        return Response{204};
    }
    if (request.method == "POST" &&
        std::regex_match(request.path, match, std::regex("/api/trash/([0-9]+)/restore")))
    {
        trash_.Restore(request.user, Id(match[1]));
        return Response{204};
    }
    if (request.method == "DELETE" &&
        std::regex_match(request.path, match, std::regex("/api/trash/([0-9]+)")))
    {
        trash_.PermanentlyDelete(request.user, Id(match[1]));
        return Response{204};
    }
    throw Error(404, "Endpoint not implemented");
}
} // namespace disk

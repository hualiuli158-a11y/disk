#pragma once
#include "common.h"
#include "http_types.h"
#include "services/account_service.h"
#include "services/file_service.h"
#include "services/folder_service.h"
#include "services/search_service.h"
#include "services/trash_service.h"
namespace disk
{
class Application
{
  public:
    Application(const Config &config, Database &database, ObjectStorage &storage)
        : database_(database), storage_(storage), auth_(config), usersRepository_(database),
          foldersRepository_(database), filesRepository_(database), searchRepository_(database),
          trashRepository_(database), accounts_(usersRepository_, auth_), folders_(foldersRepository_),
          files_(filesRepository_, storage), search_(searchRepository_), trash_(trashRepository_, storage)
    {
    }
    Application(const Application &) = delete;
    Application &operator=(const Application &) = delete;
    const Auth &Authentication() const
    {
        return auth_;
    }
    Response Handle(const Request &request, const std::shared_ptr<Cancellation> &cancel);
    Response Upload(const Request &request, const std::shared_ptr<BodyPipe> &body,
                    const std::shared_ptr<Cancellation> &cancel);

  private:
    Response Account(const Request &request);
    Response Profile(std::uint64_t user, bool quota);
    Response Root(std::uint64_t user);
    Response CreateFolder(const Request &request);
    Response Contents(const Request &request, std::uint64_t folder, bool childrenOnly);
    Response Download(std::uint64_t user, std::uint64_t file, const std::shared_ptr<Cancellation> &cancel);
    Response Rename(const Request &request, std::uint64_t id, bool folder);
    Response Move(const Request &request, std::uint64_t id, bool folder);
    Response Search(const Request &request);
    Response Trash(const Request &request);
    Database &database_;
    ObjectStorage &storage_;
    Auth auth_;
    UserRepository usersRepository_;
    FolderRepository foldersRepository_;
    FileRepository filesRepository_;
    SearchRepository searchRepository_;
    TrashRepository trashRepository_;
    AccountService accounts_;
    FolderService folders_;
    FileService files_;
    SearchService search_;
    TrashService trash_;
};
} // namespace disk

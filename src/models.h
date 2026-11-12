#pragma once
#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace disk
{
struct UserCredentials
{
    std::uint64_t id;
    std::string passwordHash;
};

struct UserProfile
{
    std::uint64_t id;
    std::string username;
    std::int64_t usedQuota;
    std::int64_t quotaLimit;
};

struct Quota
{
    std::int64_t used;
    std::int64_t limit;
    std::int64_t remaining;
};

struct Folder
{
    std::uint64_t id;
    std::optional<std::uint64_t> parentId;
    std::string name;
    std::string createdAt;
};

struct FolderListing
{
    Folder folder;
    std::vector<Folder> children;
};

struct ContentsQuery
{
    std::string page = "1";
    std::string pageSize = "100";
    std::string sort = "name";
    std::string order = "asc";
};

struct ContentItem
{
    std::uint64_t id;
    std::string type;
    std::uint64_t parentId;
    std::string name;
    std::optional<std::int64_t> size;
    std::optional<std::string> sha256;
    std::string createdAt;
    std::string updatedAt;
};

struct ContentsPage
{
    std::vector<ContentItem> items;
    std::uint64_t page;
    std::uint64_t pageSize;
    std::int64_t total;
};

struct UploadedFile
{
    std::uint64_t id;
    std::uint64_t folderId;
    std::string name;
    std::uint64_t size;
    std::string sha256;
    std::string createdAt;
};

struct UploadedObject
{
    std::uint64_t user;
    std::uint64_t folder;
    std::string name;
    std::string sha256;
    std::uint64_t size;
    std::string storageKey;
};

struct UploadCommit
{
    UploadedFile file;
    bool adopted;
};

struct StoredFile
{
    std::string name;
    std::string storageKey;
    std::string sha256;
    std::uint64_t size;
};
struct SearchQuery
{
    std::string text;
    std::string type = "all";
    ContentsQuery contents;
};
struct SearchResult
{
    ContentsPage contents;
    SearchQuery query;
};
struct TrashEntry
{
    std::uint64_t id;
    std::string type;
    std::uint64_t resourceId;
    std::optional<std::uint64_t> originalParentId;
    std::string name;
    std::string trashedAt;
    std::string expiresAt;
};
struct TrashPage
{
    std::vector<TrashEntry> items;
    std::uint64_t page;
    std::uint64_t pageSize;
    std::int64_t total;
};
} // namespace disk

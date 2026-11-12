#include "storage/local_store.h"

#include <algorithm>
#include <cerrno>
#include <cstring>
#include <dirent.h>
#include <filesystem>
#include <fcntl.h>
#include <functional>
#include <limits>
#include <sys/stat.h>
#include <unistd.h>
#include <utility>

namespace storage
{
namespace
{
constexpr std::size_t kMaxKeySize = 500;
constexpr std::uint64_t kMaxObjectSize = 1ULL << 30;

bool ValidUtf8(std::string_view value) noexcept
{
    for (std::size_t i = 0; i < value.size();)
    {
        const auto first = static_cast<unsigned char>(value[i]);
        if (first < 0x80)
        {
            ++i;
            continue;
        }

        std::size_t continuation_count = 0;
        std::uint32_t codepoint = 0;
        std::uint32_t minimum = 0;
        if (first >= 0xC2 && first <= 0xDF)
        {
            continuation_count = 1;
            codepoint = first & 0x1F;
            minimum = 0x80;
        }
        else if (first >= 0xE0 && first <= 0xEF)
        {
            continuation_count = 2;
            codepoint = first & 0x0F;
            minimum = 0x800;
        }
        else if (first >= 0xF0 && first <= 0xF4)
        {
            continuation_count = 3;
            codepoint = first & 0x07;
            minimum = 0x10000;
        }
        else
        {
            return false;
        }

        if (value.size() - i <= continuation_count) return false;
        for (std::size_t n = 1; n <= continuation_count; ++n)
        {
            const auto next = static_cast<unsigned char>(value[i + n]);
            if ((next & 0xC0) != 0x80) return false;
            codepoint = (codepoint << 6) | (next & 0x3F);
        }
        if (codepoint < minimum || (codepoint >= 0xD800 && codepoint <= 0xDFFF) ||
            codepoint > 0x10FFFF)
            return false;
        i += continuation_count + 1;
    }
    return true;
}

class UniqueFd
{
public:
    explicit UniqueFd(int fd = -1) noexcept : _fd(fd) {}
    ~UniqueFd() { if (_fd >= 0) ::close(_fd); }
    UniqueFd(const UniqueFd&) = delete;
    UniqueFd& operator=(const UniqueFd&) = delete;
    UniqueFd(UniqueFd&& other) noexcept : _fd(std::exchange(other._fd, -1)) {}

    [[nodiscard]] int Get() const noexcept { return _fd; }
    int Release() noexcept { return std::exchange(_fd, -1); }
    void Reset(int fd = -1) noexcept
    {
        if (_fd >= 0) ::close(_fd);
        _fd = fd;
    }

private:
    int _fd;
};

struct DirectoryCloser
{
    void operator()(DIR* directory) const noexcept { ::closedir(directory); }
};

StoreError SystemError(const std::string& operation, int error)
{
    StoreStatus status = StoreStatus::IoError;
    if (error == ENOENT) status = StoreStatus::NotFound;
    else if (error == ENOSPC || error == EDQUOT) status = StoreStatus::NoSpace;
    else if (error == ELOOP || error == ENOTDIR)
        status = StoreStatus::InvalidRequest;
    return StoreError(status, operation + ": " + std::strerror(error));
}

void SyncDirectory(int fd)
{
    if (::fsync(fd) < 0) throw SystemError("sync object directory", errno);
}

std::int64_t ModifiedAtMilliseconds(const struct stat& info)
{
    return static_cast<std::int64_t>(info.st_mtim.tv_sec) * 1000 +
           info.st_mtim.tv_nsec / 1000000;
}
} // 匿名命名空间结束

void LocalStore::ValidatePath(std::string_view path)
{
    if (path.empty() || path.size() > kMaxKeySize)
        throw StoreError(StoreStatus::InvalidRequest, "key length must be between 1 and 500 bytes");
    if (!ValidUtf8(path))
        throw StoreError(StoreStatus::InvalidRequest, "key must be valid UTF-8");
    if (path.find('\\') != std::string_view::npos ||
        path.find('\0') != std::string_view::npos)
        throw StoreError(StoreStatus::InvalidRequest, "key contains a forbidden byte");
    if (path.size() >= 2 && path[1] == ':' &&
        ((path[0] >= 'A' && path[0] <= 'Z') ||
         (path[0] >= 'a' && path[0] <= 'z')))
        throw StoreError(StoreStatus::InvalidRequest, "key must not contain a drive prefix");

    std::size_t begin = 0;
    while (begin < path.size())
    {
        const std::size_t end = path.find('/', begin);
        const auto segment = path.substr(begin, end == std::string_view::npos ? end : end - begin);
        if (segment.empty() || segment == "." || segment == "..")
            throw StoreError(StoreStatus::InvalidRequest, "key contains an invalid path segment");
        if (end == std::string_view::npos) return;
        begin = end + 1;
    }
    throw StoreError(StoreStatus::InvalidRequest, "key ends with a path separator");
}

LocalStore::LocalStore(const std::string& root)
{
    if (root.empty())
        throw StoreError(StoreStatus::InvalidRequest, "storage root is empty");
    std::error_code error;
    std::filesystem::create_directories(root, error);
    if (error) throw StoreError(StoreStatus::IoError,
                                "create storage root: " + error.message());
    _root_fd = ::open(root.c_str(), O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
    if (_root_fd < 0) throw SystemError("open storage root", errno);
}

LocalStore::~LocalStore()
{
    if (_root_fd >= 0) ::close(_root_fd);
}

int LocalStore::OpenDirectory(std::string_view path, bool create) const
{
    // 从已打开的根目录 fd 逐级进入，禁止路径中的符号链接跳出存储根目录。
    UniqueFd current(::dup(_root_fd));
    if (current.Get() < 0) throw SystemError("duplicate storage root", errno);
    if (path.empty()) return current.Release();

    std::size_t begin = 0;
    while (begin < path.size())
    {
        const auto separator = path.find('/', begin);
        const auto segment = std::string(path.substr(
            begin, separator == std::string_view::npos ? separator : separator - begin));
        int next = ::openat(current.Get(), segment.c_str(),
                            O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
        if (next < 0 && errno == ENOENT && create)
        {
            if (::mkdirat(current.Get(), segment.c_str(), 0700) < 0 && errno != EEXIST)
                throw SystemError("create object directory", errno);
            SyncDirectory(current.Get());
            next = ::openat(current.Get(), segment.c_str(),
                            O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW);
        }
        if (next < 0) throw SystemError("open object directory", errno);
        current.Reset(next);
        if (separator == std::string_view::npos) break;
        begin = separator + 1;
    }
    return current.Release();
}

int LocalStore::OpenParent(std::string_view key, bool create, std::string& name) const
{
    ValidatePath(key);
    const auto separator = key.rfind('/');
    name = std::string(key.substr(separator == std::string_view::npos ? 0 : separator + 1));
    if (separator == std::string_view::npos) return OpenDirectory({}, create);
    return OpenDirectory(key.substr(0, separator), create);
}

LocalStore::Upload::Upload(int directory_fd, int temporary_fd,
                           std::string temporary_name, std::string target_name,
                           std::shared_ptr<std::mutex> commit_mutex)
    : _directory_fd(directory_fd), _temporary_fd(temporary_fd),
      _temporary_name(std::move(temporary_name)), _target_name(std::move(target_name)),
      _commit_mutex(std::move(commit_mutex)) {}

LocalStore::Upload::Upload(Upload&& other) noexcept
    : _directory_fd(std::exchange(other._directory_fd, -1)),
      _temporary_fd(std::exchange(other._temporary_fd, -1)),
      _temporary_name(std::move(other._temporary_name)),
      _target_name(std::move(other._target_name)),
      _commit_mutex(std::move(other._commit_mutex)),
      _size(other._size), _committed(other._committed) {}

LocalStore::Upload& LocalStore::Upload::operator=(Upload&& other) noexcept
{
    if (this != &other)
    {
        Cleanup();
        _directory_fd = std::exchange(other._directory_fd, -1);
        _temporary_fd = std::exchange(other._temporary_fd, -1);
        _temporary_name = std::move(other._temporary_name);
        _target_name = std::move(other._target_name);
        _commit_mutex = std::move(other._commit_mutex);
        _size = other._size;
        _committed = other._committed;
    }
    return *this;
}

void LocalStore::Upload::Cleanup() noexcept
{
    // 正常完成的对象已被重命名；失败或连接中断时清除临时文件。
    if (_temporary_fd >= 0) ::close(std::exchange(_temporary_fd, -1));
    if (_directory_fd >= 0)
    {
        if (!_committed && !_temporary_name.empty())
            ::unlinkat(_directory_fd, _temporary_name.c_str(), 0);
        ::close(std::exchange(_directory_fd, -1));
    }
}

LocalStore::Upload::~Upload() { Cleanup(); }

void LocalStore::Upload::Write(std::string_view bytes)
{
    if (_temporary_fd < 0)
        throw StoreError(StoreStatus::InvalidRequest, "upload is closed");
    if (bytes.size() > kMaxObjectSize - _size)
        throw StoreError(StoreStatus::TooLarge, "object exceeds 1 GiB");

    std::size_t offset = 0;
    while (offset < bytes.size())
    {
        const auto count = std::min(bytes.size() - offset,
                                    static_cast<std::size_t>(std::numeric_limits<ssize_t>::max()));
        const ssize_t written = ::write(_temporary_fd, bytes.data() + offset, count);
        if (written < 0 && errno == EINTR) continue;
        if (written <= 0) throw SystemError("write temporary object", written < 0 ? errno : EIO);
        offset += static_cast<std::size_t>(written);
    }
    _size += bytes.size();
}

void LocalStore::Upload::Commit(std::uint64_t declared_size)
{
    if (_temporary_fd < 0 || _committed)
        throw StoreError(StoreStatus::InvalidRequest, "upload is closed");
    if (_size != declared_size)
        throw StoreError(StoreStatus::InvalidRequest, "object size does not match");
    // 先同步文件内容，再原子替换目标文件，最后同步目录项。
    if (::fsync(_temporary_fd) < 0) throw SystemError("sync temporary object", errno);
    const int fd = std::exchange(_temporary_fd, -1);
    if (::close(fd) < 0) throw SystemError("close temporary object", errno);

    std::lock_guard<std::mutex> lock(*_commit_mutex);
    if (::renameat(_directory_fd, _temporary_name.c_str(),
                   _directory_fd, _target_name.c_str()) < 0)
        throw SystemError("commit object", errno);
    _committed = true;
    SyncDirectory(_directory_fd);
}

LocalStore::Upload LocalStore::BeginPut(std::string_view key)
{
    std::string name;
    UniqueFd directory(OpenParent(key, true, name));
    for (int attempt = 0; attempt < 100; ++attempt)
    {
        const auto sequence = _temporary_sequence.fetch_add(1);
        const std::string temporary_name = ".upload-" + std::to_string(::getpid()) +
                                           "-" + std::to_string(sequence);
        UniqueFd temporary(::openat(directory.Get(), temporary_name.c_str(),
                                    O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC | O_NOFOLLOW,
                                    0600));
        if (temporary.Get() >= 0)
        {
            Upload upload(directory.Get(), temporary.Get(), temporary_name, name,
                          _commit_mutex);
            directory.Release();
            temporary.Release();
            return upload;
        }
        if (errno != EEXIST) throw SystemError("create temporary object", errno);
    }
    throw StoreError(StoreStatus::IoError,
                     "could not create a unique temporary object");
}

LocalStore::Reader::Reader(Reader&& other) noexcept
    : _fd(std::exchange(other._fd, -1)), _size(other._size) {}

LocalStore::Reader& LocalStore::Reader::operator=(Reader&& other) noexcept
{
    if (this != &other)
    {
        if (_fd >= 0) ::close(_fd);
        _fd = std::exchange(other._fd, -1);
        _size = other._size;
    }
    return *this;
}

LocalStore::Reader::~Reader()
{
    if (_fd >= 0) ::close(_fd);
}

std::size_t LocalStore::Reader::Read(char* destination, std::size_t capacity)
{
    if (_fd < 0)
        throw StoreError(StoreStatus::InvalidRequest, "reader is closed");
    if (capacity == 0) return 0;
    if (!destination)
        throw StoreError(StoreStatus::InvalidRequest, "reader destination is null");
    const auto length = std::min(capacity,
                                 static_cast<std::size_t>(std::numeric_limits<ssize_t>::max()));
    while (true)
    {
        const ssize_t count = ::read(_fd, destination, length);
        if (count < 0 && errno == EINTR) continue;
        if (count < 0) throw SystemError("read object", errno);
        return static_cast<std::size_t>(count);
    }
}

LocalStore::Reader LocalStore::Open(std::string_view key) const
{
    std::string name;
    UniqueFd directory(OpenParent(key, false, name));
    UniqueFd file(::openat(directory.Get(), name.c_str(), O_RDONLY | O_CLOEXEC | O_NOFOLLOW));
    if (file.Get() < 0) throw SystemError("open object", errno);
    struct stat info{};
    if (::fstat(file.Get(), &info) < 0) throw SystemError("stat object", errno);
    if (!S_ISREG(info.st_mode) || info.st_size < 0)
        throw StoreError(StoreStatus::InvalidRequest, "object is not a regular file");
    if (static_cast<std::uint64_t>(info.st_size) > kMaxObjectSize)
        throw StoreError(StoreStatus::TooLarge, "object exceeds 1 GiB");
    return Reader(file.Release(), static_cast<std::uint64_t>(info.st_size));
}

void LocalStore::Delete(std::string_view key)
{
    std::string name;
    UniqueFd directory;
    try
    {
        directory.Reset(OpenParent(key, false, name));
    }
    catch (const StoreError& error)
    {
        if (error.status() == StoreStatus::NotFound) return;
        throw;
    }

    std::lock_guard<std::mutex> lock(*_commit_mutex);
    struct stat info{};
    if (::fstatat(directory.Get(), name.c_str(), &info, AT_SYMLINK_NOFOLLOW) < 0)
    {
        if (errno == ENOENT) return;
        throw SystemError("stat object for deletion", errno);
    }
    if (!S_ISREG(info.st_mode))
        throw StoreError(StoreStatus::InvalidRequest,
                         "object to delete is not a regular file");
    if (::unlinkat(directory.Get(), name.c_str(), 0) < 0)
    {
        if (errno == ENOENT) return;
        throw SystemError("delete object", errno);
    }
    SyncDirectory(directory.Get());
}

std::vector<ObjectInfo> LocalStore::List(std::string_view prefix) const
{
    ValidatePath(prefix);
    UniqueFd base;
    try
    {
        base.Reset(OpenDirectory(prefix, false));
    }
    catch (const StoreError& error)
    {
        if (error.status() == StoreStatus::NotFound) return {};
        throw;
    }

    std::vector<ObjectInfo> objects;
    std::function<void(int, const std::string&)> visit;
    visit = [&](int fd, const std::string& path) {
        const int duplicate = ::dup(fd);
        if (duplicate < 0) throw SystemError("duplicate listing directory", errno);
        DIR* raw = ::fdopendir(duplicate);
        if (!raw)
        {
            const int saved = errno;
            ::close(duplicate);
            throw SystemError("open listing directory", saved);
        }
        std::unique_ptr<DIR, DirectoryCloser> listing(raw);
        while (true)
        {
            errno = 0;
            dirent* entry = ::readdir(listing.get());
            if (!entry)
            {
                if (errno != 0) throw SystemError("list objects", errno);
                break;
            }
            const std::string name(entry->d_name);
            // LIST 只返回正式对象，跳过尚未提交的上传临时文件。
            if (name == "." || name == ".." || name.rfind(".upload-", 0) == 0)
                continue;
            const std::string key = path + "/" + name;
            if (key.size() > kMaxKeySize) continue;
            try
            {
                ValidatePath(key);
            }
            catch (const StoreError& error)
            {
                if (error.status() == StoreStatus::InvalidRequest) continue;
                throw;
            }
            struct stat info{};
            if (::fstatat(fd, name.c_str(), &info, AT_SYMLINK_NOFOLLOW) < 0)
                throw SystemError("stat listed object", errno);
            if (S_ISDIR(info.st_mode))
            {
                UniqueFd child(::openat(fd, name.c_str(),
                                        O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW));
                if (child.Get() < 0) throw SystemError("open listed directory", errno);
                visit(child.Get(), key);
            }
            else if (S_ISREG(info.st_mode))
            {
                if (objects.size() >= 100000)
                    throw StoreError(StoreStatus::TooLarge,
                                     "object listing exceeds 100000 entries");
                objects.push_back(ObjectInfo{key, static_cast<std::uint64_t>(info.st_size),
                                             ModifiedAtMilliseconds(info)});
            }
        }
    };
    visit(base.Get(), std::string(prefix));
    std::sort(objects.begin(), objects.end(),
              [](const ObjectInfo& lhs, const ObjectInfo& rhs) { return lhs.key < rhs.key; });
    return objects;
}

std::vector<ObjectInfo> LocalStore::ListPage(std::string_view prefix,
                                             std::string_view after_key,
                                             std::size_t limit) const
{
    ValidatePath(prefix);
    if (limit == 0 || limit > 1000)
        throw StoreError(StoreStatus::InvalidRequest,
                         "list page limit must be between 1 and 1000");
    if (!after_key.empty())
    {
        ValidatePath(after_key);
        if (after_key.size() <= prefix.size() ||
            after_key.substr(0, prefix.size()) != prefix ||
            after_key[prefix.size()] != '/')
            throw StoreError(StoreStatus::InvalidRequest,
                             "list cursor is outside prefix");
    }

    UniqueFd base;
    try { base.Reset(OpenDirectory(prefix, false)); }
    catch (const StoreError& error)
    {
        if (error.status() == StoreStatus::NotFound) return {};
        throw;
    }

    std::vector<ObjectInfo> page;
    std::function<bool(int, const std::string&)> visit;
    visit = [&](int fd, const std::string& path) -> bool {
        const int duplicate = ::dup(fd);
        if (duplicate < 0) throw SystemError("duplicate listing directory", errno);
        DIR* raw = ::fdopendir(duplicate);
        if (!raw)
        {
            const int saved = errno;
            ::close(duplicate);
            throw SystemError("open listing directory", saved);
        }
        std::unique_ptr<DIR, DirectoryCloser> listing(raw);
        struct PageEntry
        {
            std::string name;
            std::string key;
            std::string order_key;
            struct stat info;
            bool directory;
        };
        std::vector<PageEntry> entries;
        while (true)
        {
            errno = 0;
            dirent* entry = ::readdir(listing.get());
            if (!entry)
            {
                if (errno != 0) throw SystemError("list objects", errno);
                break;
            }
            const std::string name(entry->d_name);
            if (name == "." || name == ".." || name.rfind(".upload-", 0) == 0)
                continue;
            const std::string key = path + "/" + name;
            if (key.size() > kMaxKeySize) continue;
            try { ValidatePath(key); }
            catch (const StoreError& error)
            {
                if (error.status() == StoreStatus::InvalidRequest) continue;
                throw;
            }
            struct stat info{};
            if (::fstatat(fd, name.c_str(), &info, AT_SYMLINK_NOFOLLOW) < 0)
                throw SystemError("stat listed object", errno);
            const bool directory = S_ISDIR(info.st_mode);
            if (!directory && !S_ISREG(info.st_mode)) continue;
            entries.push_back(PageEntry{name, key, key + (directory ? "/" : ""), info,
                                        directory});
        }
        std::sort(entries.begin(), entries.end(),
                  [](const PageEntry& lhs, const PageEntry& rhs) {
                      return lhs.order_key < rhs.order_key;
                  });

        for (const auto& entry : entries)
        {
            if (entry.directory)
            {
                const std::string subtree = entry.key + "/";
                if (!after_key.empty() && std::string_view(subtree) <= after_key &&
                    after_key.substr(0, subtree.size()) != std::string_view(subtree))
                    continue;
                UniqueFd child(::openat(fd, entry.name.c_str(),
                                        O_RDONLY | O_DIRECTORY | O_CLOEXEC | O_NOFOLLOW));
                if (child.Get() < 0) throw SystemError("open listed directory", errno);
                if (visit(child.Get(), entry.key)) return true;
            }
            else if (std::string_view(entry.key) > after_key)
            {
                if (entry.info.st_size < 0)
                    throw StoreError(StoreStatus::IoError,
                                     "listed object has negative size");
                page.push_back(ObjectInfo{entry.key,
                                          static_cast<std::uint64_t>(entry.info.st_size),
                                          ModifiedAtMilliseconds(entry.info)});
                if (page.size() == limit) return true;
            }
        }
        return false;
    };
    visit(base.Get(), std::string(prefix));
    return page;
}
} // storage 命名空间结束

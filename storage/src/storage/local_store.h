#ifndef GRPC_OBJECT_STORAGE_LOCAL_STORE_H
#define GRPC_OBJECT_STORAGE_LOCAL_STORE_H

#include <atomic>
#include <cstddef>
#include <cstdint>
#include <memory>
#include <mutex>
#include <stdexcept>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

namespace storage
{
enum class StoreStatus
{
    InvalidRequest,
    NotFound,
    TooLarge,
    NoSpace,
    IoError,
};

class StoreError : public std::runtime_error
{
public:
    StoreError(StoreStatus status, std::string message): std::runtime_error(std::move(message)), _status(status) {}
    [[nodiscard]] StoreStatus status() const noexcept { return _status; }

private:
    StoreStatus _status;
};

struct ObjectInfo
{
    std::string key;
    std::uint64_t size;
    std::int64_t modified_at_unix_ms;
};

// 所有 key 都相对根目录解析。
class LocalStore
{
public:
    explicit LocalStore(const std::string& root);
    ~LocalStore();
    LocalStore(const LocalStore&) = delete;
    LocalStore& operator=(const LocalStore&) = delete;

    // 未提交时只写同目录临时文件；析构时删除仍未提交的临时文件。
    class Upload
    {
    public:
        Upload(const Upload&) = delete;
        Upload& operator=(const Upload&) = delete;
        Upload(Upload&& other) noexcept;
        Upload& operator=(Upload&& other) noexcept;
        ~Upload();

        void Write(std::string_view bytes);
        void Commit(std::uint64_t declared_size);
        [[nodiscard]] std::uint64_t size() const noexcept { return _size; }

    private:
        friend class LocalStore;
        Upload(int directory_fd, int temporary_fd, std::string temporary_name,
               std::string target_name, std::shared_ptr<std::mutex> commit_mutex);
        void Cleanup() noexcept;

        int _directory_fd = -1;
        int _temporary_fd = -1;
        std::string _temporary_name;
        std::string _target_name;
        std::shared_ptr<std::mutex> _commit_mutex;
        std::uint64_t _size = 0;
        bool _committed = false;
    };

    // 持有已打开对象的 fd；对象被替换或删除后仍可继续读取这个版本。
    class Reader
    {
    public:
        Reader(const Reader&) = delete;
        Reader& operator=(const Reader&) = delete;
        Reader(Reader&& other) noexcept;
        Reader& operator=(Reader&& other) noexcept;
        ~Reader();

        std::size_t Read(char* destination, std::size_t capacity);
        [[nodiscard]] std::uint64_t size() const noexcept { return _size; }

    private:
        friend class LocalStore;
        Reader(int fd, std::uint64_t size) : _fd(fd), _size(size) {}
        int _fd = -1;
        std::uint64_t _size = 0;
    };

    Upload BeginPut(std::string_view key);
    Reader Open(std::string_view key) const;
    void Delete(std::string_view key);
    std::vector<ObjectInfo> List(std::string_view prefix) const;
    // Returns at most limit objects in key order, strictly after after_key.
    // An empty after_key starts at the beginning; the caller uses the last
    // returned key as the cursor for the next page.
    std::vector<ObjectInfo> ListPage(std::string_view prefix,
                                     std::string_view after_key,
                                     std::size_t limit) const;

private:
    int OpenDirectory(std::string_view path, bool create) const;
    int OpenParent(std::string_view key, bool create, std::string& name) const;
    static void ValidatePath(std::string_view path);

    int _root_fd = -1;
    std::atomic<std::uint64_t> _temporary_sequence{0};
    std::shared_ptr<std::mutex> _commit_mutex = std::make_shared<std::mutex>();
};
} // storage 命名空间结束

#endif // 头文件保护结束

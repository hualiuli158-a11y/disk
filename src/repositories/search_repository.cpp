#include "search_repository.h"
#include "repository_support.h"
namespace disk
{
namespace mysql = boost::mysql;
ContentsPage SearchRepository::Search(std::uint64_t user, const SearchQuery &query, std::uint64_t page,
                                      std::uint64_t pageSize)
{
    const std::string folders =
        "SELECT id,'folder' AS type,parent_id,name,CAST(NULL AS SIGNED) AS size,NULL AS "
        "sha256,created_at,updated_at "
        "FROM folders WHERE user_id={} AND parent_id IS NOT NULL AND deleted_at IS NULL AND name LIKE {}";
    const std::string files = "SELECT f.id,'file' AS type,f.folder_id AS "
                              "parent_id,f.name,p.size,p.sha256,f.created_at,f.updated_at "
                              "FROM files f JOIN physical_files p ON p.id=f.physical_file_id WHERE "
                              "f.user_id={} AND f.deleted_at IS NULL AND f.name LIKE {}";
    const auto base = query.type == "folder" ? folders
                      : query.type == "file" ? files
                                             : folders + " UNION ALL " + files;
    const auto pattern = "%" + query.text + "%";
    auto connection = database_.Get();
    auto execute = [&](const std::string &sql)
    {
        if (query.type == "all")
            return database_.Execute(connection.get(),
                                     mysql::with_params(mysql::runtime(sql), user, pattern, user, pattern));
        return database_.Execute(connection.get(), mysql::with_params(mysql::runtime(sql), user, pattern));
    };
    // Pagination values have already been validated as bounded unsigned integers.
    const auto sql =
        "SELECT id,type,parent_id,name,size,sha256,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%fZ'),"
        "DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%s.%fZ') FROM (" +
        base + ") items ORDER BY type DESC," + ContentSort(query.contents.sort, query.contents.order) +
        ",name ASC,id ASC LIMIT " + std::to_string(pageSize) + " OFFSET " +
        std::to_string((page - 1) * pageSize);
    const auto count = execute("SELECT COUNT(*) FROM (" + base + ") items");
    const auto rows = execute(sql);
    ContentsPage result;
    result.page = page;
    result.pageSize = pageSize;
    result.total = count.rows()[0][0].as_int64();
    for (auto row : rows.rows())
        result.items.push_back(ReadContentItem(row));
    return result;
}
} // namespace disk

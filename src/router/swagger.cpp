#include "swagger.h"
#include "disk_swagger_assets.h"
#include "gin/gin.h"

#include <memory>
#include <string>
#include <string_view>

namespace disk
{
namespace
{
class AssetReader final : public gin::ContentReader
{
  public:
    explicit AssetReader(std::string_view asset) : size_(asset.size()), remaining_(asset) {}

    std::uint64_t Size() const override { return size_; }

    std::string Read() override
    {
        const auto chunk = remaining_.substr(0, 65536);
        remaining_.remove_prefix(chunk.size());
        return std::string(chunk);
    }

  private:
    std::uint64_t size_;
    std::string_view remaining_;
};

template <std::size_t Size>
void RegisterAsset(gin::Engine &engine, const std::string &path, const std::string &mediaType,
                   const unsigned char (&asset)[Size], const std::string &contentSecurityPolicy = {})
{
    const std::string_view bytes(reinterpret_cast<const char *>(asset), Size);
    engine.GET(path, [mediaType, bytes, contentSecurityPolicy](gin::Context &context)
    {
        if (!contentSecurityPolicy.empty())
            context.Header("Content-Security-Policy", contentSecurityPolicy);
        context.DataFromReader(200, mediaType, std::make_shared<AssetReader>(bytes));
    });
}
} // namespace

void RegisterSwagger(gin::Engine &engine)
{
    RegisterAsset(engine, "/docs/", "text/html; charset=utf-8", swagger_assets::index,
                  "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; "
                  "object-src 'none'; base-uri 'none'; frame-ancestors 'none'");
    RegisterAsset(engine, "/docs/init.js", "application/javascript; charset=utf-8", swagger_assets::init);
    RegisterAsset(engine, "/docs/swagger-ui.css", "text/css; charset=utf-8", swagger_assets::swagger_ui);
    RegisterAsset(engine, "/docs/swagger-ui-bundle.js", "application/javascript; charset=utf-8",
                  swagger_assets::swagger_ui_bundle);
    RegisterAsset(engine, "/openapi.json", "application/json; charset=utf-8", swagger_assets::openapi);
}
} // namespace disk

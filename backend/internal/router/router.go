package router

import (
	"disk/internal/handler"
	"disk/internal/middleware"
	"disk/pkg/app"
	"disk/pkg/logger"
	"disk/pkg/setting"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Handlers 集中保存 HTTP 路由需要的业务处理器。
type Handlers struct {
	Login             *handler.LoginHandler
	Register          *handler.RegisterHandler
	Profile           *handler.ProfileHandler
	Quota             *handler.QuotaHandler
	Folders           *handler.ChildFoldersCreateHandler
	ListFolders       *handler.ListFoldersHandler
	Rename            *handler.RenameFolderHandler
	Children          *handler.ListFolderChildrenHandler
	Delete            *handler.DeleteFolderHandler
	Move              *handler.MoveFolderHandler
	Contents          *handler.FolderContentsHandler
	UploadFile        *handler.UploadFileHandler
	ListFiles         *handler.ListFilesHandler
	DownloadFile      *handler.DownloadFileHandler
	UpdateFileContent *handler.UpdateFileContentHandler
	FileVersions      *handler.FileVersionHandler
	DeleteFile        *handler.DeleteFileHandler
	RenameFile        *handler.RenameFileHandler
	MoveFile          *handler.MoveFileHandler
	Search            *handler.SearchHandler
	Trash             *handler.TrashHandler
}

func NewRouter(baseLogger *logger.Logger, jwtService *app.JWTService, handlers Handlers, abuse setting.AbuseProtectionConfig, upload setting.UploadConfig, security setting.SecurityConfig) (*gin.Engine, error) {
	r := gin.New()
	if err := r.SetTrustedProxies(abuse.TrustedProxies); err != nil {
		return nil, err
	}
	r.Use(gin.Recovery(), gin.Logger(), middleware.RequireHTTPS(security.RequireHTTPS), middleware.CORS(security.CORSAllowedOrigins))
	authLimiter := middleware.NewRateLimiter(abuse.AuthPerMinute, abuse.AuthBurst)
	uploadIPLimiter := middleware.NewRateLimiter(abuse.UploadPerMinute, abuse.UploadBurst)
	uploadUserLimiter := middleware.NewRateLimiter(abuse.UploadPerMinute, abuse.UploadBurst)
	searchIPLimiter := middleware.NewRateLimiter(abuse.SearchPerMinute, abuse.SearchBurst)
	searchUserLimiter := middleware.NewRateLimiter(abuse.SearchPerMinute, abuse.SearchBurst)

	if security.SwaggerEnabled {
		swaggerHandler := ginSwagger.WrapHandler(swaggerFiles.Handler)
		if security.SwaggerUsername != "" && security.SwaggerPassword != "" {
			swagger := r.Group("/swagger", gin.BasicAuth(gin.Accounts{
				security.SwaggerUsername: security.SwaggerPassword,
			}))
			swagger.GET("/*any", swaggerHandler)
		} else {
			r.GET("/swagger/*any", swaggerHandler)
		}
	}
	baseLogger.WithFields(logger.Fields{
		"swagger_enabled": security.SwaggerEnabled,
		"cors_origins":    len(security.CORSAllowedOrigins),
	}).Info("router security configured")
	r.POST("/login", middleware.RateLimit(authLimiter, middleware.ByClientIP("login")), handlers.Login.Login)
	r.POST("/register", middleware.RateLimit(authLimiter, middleware.ByClientIP("register")), handlers.Register.Register)

	authorized := r.Group("/api")
	authorized.Use(middleware.JWT(jwtService))
	{
		authorized.GET("/profile", handlers.Profile.Profile)
		authorized.GET("/quota", handlers.Quota.GetQuota)
		authorized.POST("/folders", handlers.Folders.CreateChildFolder)
		authorized.GET("/folders/root/children", handlers.ListFolders.ListFolders)
		authorized.PATCH("/folders/:id", handlers.Rename.RenameFolder)
		authorized.GET("/folders/:id/children", handlers.Children.ListChildren)
		authorized.DELETE("/folders/:id", handlers.Delete.DeleteFolder)
		authorized.PATCH("/folders/:id/move", handlers.Move.MoveFolder)
		authorized.GET("/folders/:id/contents", handlers.Contents.ListContents)
		authorized.POST("/files",
			middleware.RateLimit(uploadIPLimiter, middleware.ByClientIP("upload")),
			middleware.RateLimit(uploadUserLimiter, middleware.ByUserID("upload")),
			middleware.ConcurrentUploadsPerUser(upload.ConcurrentPerUser),
			middleware.MaxBodyBytes(upload.MaxFileBytes, upload.MaxRequestBytes()),
			handlers.UploadFile.UploadFile,
		)
		authorized.GET("/folders/:id/files", handlers.ListFiles.ListFiles)
		authorized.GET("/files/:id/download", handlers.DownloadFile.DownloadFile)
		authorized.PUT("/files/:id/content", handlers.UpdateFileContent.UpdateFileContent)
		authorized.GET("/files/:id/versions", handlers.FileVersions.ListFileVersions)
		authorized.GET("/files/:id/versions/:version_id/download", handlers.FileVersions.DownloadFileVersion)
		authorized.POST("/files/:id/versions/:version_id/restore", handlers.FileVersions.RestoreFileVersion)
		authorized.DELETE("/files/:id", handlers.DeleteFile.DeleteFile)
		authorized.PATCH("/files/:id", handlers.RenameFile.RenameFile)
		authorized.PATCH("/files/:id/move", handlers.MoveFile.MoveFile)
		authorized.GET("/search",
			middleware.RateLimit(searchIPLimiter, middleware.ByClientIP("search")),
			middleware.RateLimit(searchUserLimiter, middleware.ByUserID("search")),
			handlers.Search.Search,
		)
		authorized.GET("/trash", handlers.Trash.ListTrash)
		authorized.POST("/trash/:id/restore", handlers.Trash.RestoreTrash)
		authorized.DELETE("/trash/:id", handlers.Trash.PermanentlyDeleteTrash)
	}
	return r, nil
}

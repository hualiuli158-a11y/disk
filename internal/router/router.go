package router

import (
	"disk/internal/handler"
	"disk/internal/middleware"
	"disk/pkg/app"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Handlers 集中保存 HTTP 路由需要的业务处理器。
type Handlers struct {
	Login       *handler.LoginHandler
	Register    *handler.RegisterHandler
	Profile     *handler.ProfileHandler
	Folders     *handler.ChildFoldersCreateHandler
	ListFolders *handler.ListFoldersHandler
	Rename      *handler.RenameFolderHandler
	Children    *handler.ListFolderChildrenHandler
	Delete      *handler.DeleteFolderHandler
	Move        *handler.MoveFolderHandler
	UploadFile  *handler.UploadFileHandler
	ListFiles   *handler.ListFilesHandler
}

func NewRouter(baseLogger *logger.Logger, jwtService *app.JWTService, handlers Handlers) (*gin.Engine, error) {
	routerLogger := baseLogger.WithFields(logger.Fields{
		"module": "router",
	})
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/", func(c *gin.Context) {
		routerLogger.Info("handling index request")
		c.JSON(200, gin.H{
			"hello": "world",
		})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.POST("/login", handlers.Login.Login)
	r.POST("/register", handlers.Register.Register)

	authorized := r.Group("/api")
	authorized.Use(middleware.JWT(jwtService))
	{
		authorized.GET("/profile", handlers.Profile.Profile)
		authorized.POST("/folders", handlers.Folders.CreateChildFolder)
		authorized.GET("/folders/root/children", handlers.ListFolders.ListFolders)
		authorized.PATCH("/folders/:id", handlers.Rename.RenameFolder)
		authorized.GET("/folders/:id/children", handlers.Children.ListChildren)
		authorized.DELETE("/folders/:id", handlers.Delete.DeleteFolder)
		authorized.PATCH("/folders/:id/move", handlers.Move.MoveFolder)
		authorized.POST("/files", handlers.UploadFile.UploadFile)
		authorized.GET("/folders/:id/files", handlers.ListFiles.ListFiles)
	}
	return r, nil
}

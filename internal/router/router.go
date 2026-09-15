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
		//GET /api/folders/root/children：让客户端获得根目录和第一层目录
		//GET /api/folders/:id/children：浏览任意目录
		//PATCH /api/folders/:id：重命名目录
		//DELETE /api/folders/:id：删除空目录
	}
	return r, nil
}

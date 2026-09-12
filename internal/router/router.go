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

func NewRouter(
	baseLogger *logger.Logger,
	jwtService *app.JWTService,
	loginHandler *handler.LoginHandler,
) (*gin.Engine, error) {
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
	r.POST("/login", loginHandler.Login)

	authorized := r.Group("/api")
	authorized.Use(middleware.JWT(jwtService))
	{
		authorized.GET("/profile", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "登录成功"})
		})
	}
	return r, nil
}

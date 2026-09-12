package router

import (
	"disk/internal/middleware"
	"disk/pkg/app"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

func NewRouter(baseLogger *logger.Logger, jwtservice *app.JWTService) (*gin.Engine, error) {
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

	authorized := r.Group("/api")
	authorized.Use(middleware.JWT(jwtservice))
	{
		authorized.GET("/profile", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "登录成功"})
		})
	}
	return r, nil
}

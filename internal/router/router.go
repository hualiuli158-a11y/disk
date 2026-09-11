package router

import (
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

func NewRouter(baseLogger *logger.Logger) (*gin.Engine, error) {
	routerLogger := baseLogger.WithFields(logger.Fields{
		"module": "router",
	})
	r := gin.New()
	gin.Default()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/", func(c *gin.Context) {
		routerLogger.Info("handling index request")
		c.JSON(200, gin.H{
			"hello": "world",
		})
	})
	return r, nil
}

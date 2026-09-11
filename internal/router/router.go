package router

import "github.com/gin-gonic/gin"

func NewRouter() (*gin.Engine, error) {
	r := gin.New()
	gin.Default()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"hello": "world",
		})
	})
	return r, nil
}

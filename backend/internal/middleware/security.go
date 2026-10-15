package middleware

import (
	"net/http"
	"strings"

	"disk/pkg/app"
	"disk/pkg/errcode"

	"github.com/gin-gonic/gin"
)

func RequireHTTPS(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled || c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") ||
			c.Request.URL.Path == "/livez" || c.Request.URL.Path == "/readyz" {
			c.Next()
			return
		}
		app.NewResponse(c).ToErrorResponse(errcode.HTTPSRequired)
		c.Abort()
	}
}

func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		c.Header("Vary", "Origin")
		if _, ok := allowed[origin]; !ok {
			app.NewResponse(c).ToErrorResponse(errcode.CORSOriginForbidden)
			c.Abort()
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, X-Upload-Size")
		c.Header("Access-Control-Expose-Headers", "ETag, X-File-SHA256, Retry-After")
		c.Header("Access-Control-Max-Age", "600")
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

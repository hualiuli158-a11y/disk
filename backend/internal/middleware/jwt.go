package middleware

import (
	"disk/pkg/errcode"
	"errors"
	"strings"

	"disk/pkg/app"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func JWT(jwtService *app.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := getToken(c)
		if token == "" {
			response := app.NewResponse(c)
			response.ToErrorResponse(errcode.AuthenticationRequired)
			c.Abort()
			return
		}

		claims, err := jwtService.ParseToken(token)
		if err != nil {
			ecode := errcode.UnauthorizedTokenError
			if errors.Is(err, jwt.ErrTokenExpired) {
				ecode = errcode.UnauthorizedTokenTimeout
			}

			response := app.NewResponse(c)
			response.ToErrorResponse(ecode)
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)

		c.Next()
	}
}

func getToken(c *gin.Context) string {
	const bearerPrefix = "Bearer "

	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(authorization) >= len(bearerPrefix) &&
		strings.EqualFold(authorization[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(authorization[len(bearerPrefix):])
	}

	if token := strings.TrimSpace(c.GetHeader("token")); token != "" {
		return token
	}

	return ""
}

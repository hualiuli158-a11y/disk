package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"disk/pkg/app"
	"disk/pkg/setting"

	"github.com/gin-gonic/gin"
)

func TestJWTRejectsMissingTokenAsUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(JWT(app.NewJWTService(setting.JwtConfig{})))
	router.GET("/api/profile", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Code != 20004 {
		t.Fatalf("error code = %d, want 20004", payload.Code)
	}
}

func TestJWTDoesNotAcceptTokenInURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := app.NewJWTService(setting.JwtConfig{Secret: "0123456789abcdef0123456789abcdef", Issuer: "disk", Expire: time.Hour})
	token, err := service.GenerateToken(1)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(JWT(service))
	router.GET("/api/profile", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := httptest.NewRequest(http.MethodGet, "/api/profile?token="+token, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("query token status=%d, want 401", response.Code)
	}
}

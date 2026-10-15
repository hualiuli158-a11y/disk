package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS([]string{"https://disk.example.com"}))
	router.GET("/resource", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/resource", nil)
		req.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	allowed := request("https://disk.example.com")
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "https://disk.example.com" {
		t.Fatalf("allowed origin response: status=%d headers=%v", allowed.Code, allowed.Header())
	}
	blocked := request("https://evil.example")
	if blocked.Code != http.StatusForbidden || blocked.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("blocked origin response: status=%d headers=%v", blocked.Code, blocked.Header())
	}
}

func TestRequireHTTPSHonorsTrustedProxyScheme(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireHTTPS(true))
	router.GET("/resource", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	insecure := httptest.NewRecorder()
	router.ServeHTTP(insecure, httptest.NewRequest(http.MethodGet, "/resource", nil))
	if insecure.Code != http.StatusUpgradeRequired {
		t.Fatalf("insecure status=%d, want 426", insecure.Code)
	}
	secureRequest := httptest.NewRequest(http.MethodGet, "/resource", nil)
	secureRequest.Header.Set("X-Forwarded-Proto", "https")
	secure := httptest.NewRecorder()
	router.ServeHTTP(secure, secureRequest)
	if secure.Code != http.StatusNoContent {
		t.Fatalf("secure proxy request status=%d", secure.Code)
	}
}

func TestLoginAttemptGuardTemporarilyBlocksFailures(t *testing.T) {
	now := time.Date(2026, time.October, 8, 12, 0, 0, 0, time.UTC)
	guard := NewLoginAttemptGuard(2, 5*time.Minute)
	guard.now = func() time.Time { return now }
	guard.Failure("ip-user")
	if allowed, _ := guard.Allow("ip-user"); !allowed {
		t.Fatal("guard blocked before failure limit")
	}
	guard.Failure("ip-user")
	if allowed, retry := guard.Allow("ip-user"); allowed || retry != 5*time.Minute {
		t.Fatalf("guard did not block: allowed=%v retry=%s", allowed, retry)
	}
	now = now.Add(5 * time.Minute)
	if allowed, _ := guard.Allow("ip-user"); !allowed {
		t.Fatal("guard did not release expired block")
	}
}

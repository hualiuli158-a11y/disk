package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"disk/pkg/app"
	"disk/pkg/logger"
	"disk/pkg/setting"
)

func TestProductionSurfaceDoesNotExposeRootOrSwagger(t *testing.T) {
	router, err := NewRouter(
		logger.NewLogger(io.Discard),
		app.NewJWTService(setting.JwtConfig{
			Secret: "0123456789abcdef0123456789abcdef", Issuer: "disk", Expire: 15 * time.Minute,
		}),
		Handlers{},
		setting.AbuseProtectionConfig{
			TrustedProxies: []string{"127.0.0.1"}, AuthPerMinute: 1, AuthBurst: 1,
			UploadPerMinute: 1, UploadBurst: 1, SearchPerMinute: 1, SearchBurst: 1,
		},
		setting.UploadConfig{MaxFileBytes: 1024, MultipartOverheadBytes: 1024, ConcurrentPerUser: 1},
		setting.SecurityConfig{Environment: "production", SwaggerEnabled: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/swagger/index.html"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("GET %s status=%d, want 404", path, response.Code)
		}
	}
}

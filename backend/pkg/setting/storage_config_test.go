package setting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func storageConfigEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("LISTEN", "127.0.0.1")
	t.Setenv("PORT", "8080")
	t.Setenv("DB_HOST", "127.0.0.1:3306")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_NAME", "disk")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("JWT_SECRET_FILE", "")
	t.Setenv("JWT_ISSUER", "disk")
	t.Setenv("JWT_EXPIRE", "1h")
	t.Setenv("APP_ENV", "development")
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	t.Setenv("SWAGGER_ENABLED", "")
	t.Setenv("SWAGGER_USERNAME", "")
	t.Setenv("SWAGGER_PASSWORD", "")
	t.Setenv("STORAGE_BACKEND", "")
	t.Setenv("STORAGE_ADDRESS", "")
	t.Setenv("STORAGE_TIMEOUT", "")
	t.Setenv("STORAGE_TLS_CA", "")
	t.Setenv("STORAGE_TLS_SERVER_NAME", "")
	t.Setenv("STORAGE_TLS_CLIENT_CERT", "")
	t.Setenv("STORAGE_TLS_CLIENT_KEY", "")
	t.Setenv("HTTP_READ_HEADER_TIMEOUT", "")
	t.Setenv("HTTP_READ_TIMEOUT", "")
	t.Setenv("HTTP_WRITE_TIMEOUT", "")
	t.Setenv("HTTP_IDLE_TIMEOUT", "")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "")
	t.Setenv("CLEANUP_SHUTDOWN_TIMEOUT", "")
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("AUTH_RATE_PER_MINUTE", "")
	t.Setenv("AUTH_RATE_BURST", "")
	t.Setenv("LOGIN_FAILURE_LIMIT", "")
	t.Setenv("LOGIN_BLOCK_DURATION", "")
	t.Setenv("UPLOAD_RATE_PER_MINUTE", "")
	t.Setenv("UPLOAD_RATE_BURST", "")
	t.Setenv("SEARCH_RATE_PER_MINUTE", "")
	t.Setenv("SEARCH_RATE_BURST", "")
	t.Setenv("UPLOAD_MAX_BYTES", "")
	t.Setenv("UPLOAD_MULTIPART_OVERHEAD_BYTES", "")
	t.Setenv("UPLOAD_CONCURRENT_PER_USER", "")
}

func TestStorageBackendConfig(t *testing.T) {
	t.Run("short signing secret is rejected", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("JWT_SECRET", "short")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
			t.Fatalf("expected JWT_SECRET validation, got %v", err)
		}
	})
	t.Run("low entropy signing secret is rejected", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "randomly generated") {
			t.Fatalf("expected weak JWT secret rejection, got %v", err)
		}
	})
	t.Run("signing secret can be read from mounted file", func(t *testing.T) {
		storageConfigEnvironment(t)
		path := filepath.Join(t.TempDir(), "jwt-secret")
		if err := os.WriteFile(path, []byte("0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_SECRET_FILE", path)
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if config.Jwt.SecretSource != "file" || config.Jwt.Secret != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("unexpected file-backed JWT config: %+v", config.Jwt)
		}
	})
	t.Run("local remains default", func(t *testing.T) {
		storageConfigEnvironment(t)
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if config.Storage.Backend != "local" {
			t.Fatalf("default backend=%q err=%v", config.Storage.Backend, err)
		}
	})
	t.Run("grpc address and timeout", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_BACKEND", "grpc")
		t.Setenv("STORAGE_ADDRESS", "127.0.0.1:50051")
		t.Setenv("STORAGE_TIMEOUT", "2m")
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if config.Storage.GRPCAddress != "127.0.0.1:50051" || config.Storage.GRPCTimeout != 2*time.Minute {
			t.Fatalf("gRPC config=%+v", config.Storage)
		}
	})
	t.Run("grpc requires address", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_BACKEND", "grpc")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_ADDRESS") {
			t.Fatalf("expected missing address error, got %v", err)
		}
	})
	t.Run("client certificate requires key", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_BACKEND", "grpc")
		t.Setenv("STORAGE_ADDRESS", "127.0.0.1:50051")
		t.Setenv("STORAGE_TLS_CA", "ca.pem")
		t.Setenv("STORAGE_TLS_CLIENT_CERT", "client.pem")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_TLS_CLIENT_KEY") {
			t.Fatalf("expected client key validation, got %v", err)
		}
	})
	for _, backend := range []string{"tcp", "remote", "unknown"} {
		t.Run(backend+" backend is rejected", func(t *testing.T) {
			storageConfigEnvironment(t)
			t.Setenv("STORAGE_BACKEND", backend)
			if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_BACKEND") {
				t.Fatalf("expected invalid backend error, got %v", err)
			}
		})
	}
	t.Run("grpc timeout must be positive", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_TIMEOUT", "0s")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_TIMEOUT") {
			t.Fatalf("expected invalid timeout error, got %v", err)
		}
	})
	t.Run("HTTP production defaults", func(t *testing.T) {
		storageConfigEnvironment(t)
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		server := config.Server
		if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 30*time.Minute ||
			server.WriteTimeout != 30*time.Minute || server.IdleTimeout != 2*time.Minute ||
			server.MaxHeaderBytes != 64<<10 || server.ShutdownTimeout != 30*time.Second ||
			server.CleanupShutdownTimeout != 5*time.Second {
			t.Fatalf("unexpected HTTP defaults: %+v", server)
		}
	})
	t.Run("HTTP timeouts must be positive", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "0s")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "HTTP_SHUTDOWN_TIMEOUT") {
			t.Fatalf("expected invalid HTTP shutdown timeout, got %v", err)
		}
	})
	t.Run("abuse protection defaults", func(t *testing.T) {
		storageConfigEnvironment(t)
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if config.Upload.MaxFileBytes != 1<<30 || config.Upload.MaxRequestBytes() != (1<<30)+(1<<20) ||
			config.Upload.ConcurrentPerUser != 2 || config.Abuse.AuthPerMinute != 20 ||
			config.Abuse.UploadPerMinute != 30 || config.Abuse.SearchPerMinute != 120 ||
			len(config.Abuse.TrustedProxies) != 2 {
			t.Fatalf("unexpected abuse protection defaults: upload=%+v abuse=%+v", config.Upload, config.Abuse)
		}
	})
	t.Run("production security defaults", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("APP_ENV", "production")
		t.Setenv("JWT_EXPIRE", "15m")
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://disk.example.com")
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if !config.Security.RequireHTTPS || config.Security.SwaggerEnabled {
			t.Fatalf("unexpected production security config: %+v", config.Security)
		}
	})
	t.Run("production rejects long lived access token", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("APP_ENV", "production")
		t.Setenv("JWT_EXPIRE", "16m")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "15m") {
			t.Fatalf("expected production JWT expiry rejection, got %v", err)
		}
	})
	t.Run("production rejects public listener", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("APP_ENV", "production")
		t.Setenv("JWT_EXPIRE", "15m")
		t.Setenv("LISTEN", "0.0.0.0")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "private IP") {
			t.Fatalf("expected public listener rejection, got %v", err)
		}
	})
}

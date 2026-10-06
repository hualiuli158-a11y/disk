package setting

import (
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
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("JWT_ISSUER", "disk")
	t.Setenv("JWT_EXPIRE", "1h")
	t.Setenv("STORAGE_BACKEND", "")
	t.Setenv("STORAGE_ADDRESS", "")
	t.Setenv("STORAGE_TIMEOUT", "")
	t.Setenv("STORAGE_TLS_CA", "")
	t.Setenv("STORAGE_TLS_SERVER_NAME", "")
	t.Setenv("STORAGE_TLS_CLIENT_CERT", "")
	t.Setenv("STORAGE_TLS_CLIENT_KEY", "")
}

func TestStorageBackendConfig(t *testing.T) {
	t.Run("short signing secret is rejected", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("JWT_SECRET", "short")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
			t.Fatalf("expected JWT_SECRET validation, got %v", err)
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
}

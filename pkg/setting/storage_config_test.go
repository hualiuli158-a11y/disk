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
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("JWT_ISSUER", "disk")
	t.Setenv("JWT_EXPIRE", "1h")
	t.Setenv("STORAGE_BACKEND", "")
	t.Setenv("STORAGE_REMOTE_ADDRESS", "")
	t.Setenv("STORAGE_REMOTE_TIMEOUT", "")
	t.Setenv("STORAGE_ADDRESS", "")
	t.Setenv("STORAGE_TIMEOUT", "")
}

func TestStorageBackendConfig(t *testing.T) {
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
	t.Run("remote address and timeout", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_BACKEND", "remote")
		t.Setenv("STORAGE_REMOTE_ADDRESS", "127.0.0.1:9000")
		t.Setenv("STORAGE_REMOTE_TIMEOUT", "5m")
		config, err := LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if config.Storage.Backend != "remote" ||
			config.Storage.RemoteAddress != "127.0.0.1:9000" ||
			config.Storage.RemoteTimeout != 5*time.Minute {
			t.Fatalf("remote config=%+v err=%v", config.Storage, err)
		}
	})
	t.Run("remote requires address", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_BACKEND", "remote")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_REMOTE_ADDRESS") {
			t.Fatalf("expected missing address error, got %v", err)
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
	t.Run("unknown backend is rejected", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_BACKEND", "unknown")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_BACKEND") {
			t.Fatalf("expected invalid backend error, got %v", err)
		}
	})
	t.Run("timeout must be positive", func(t *testing.T) {
		storageConfigEnvironment(t)
		t.Setenv("STORAGE_REMOTE_TIMEOUT", "0s")
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "STORAGE_REMOTE_TIMEOUT") {
			t.Fatalf("expected invalid timeout error, got %v", err)
		}
	})
}

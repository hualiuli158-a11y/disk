package setting

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// 如何提取出来.env文件中的数据
// 将.env文件加载到当前进程的环境变量中和获取这些环境变量是两件事情，需要区分开来
// 这意味着即使不存在着.env文件，项目还是应该启动，只要在发现一些必须要的变量未获取到的时候才报错，进程终止
func LoadEnvironment() error {
	err := godotenv.Load(".env")
	if err != nil && !errors.Is(err, os.ErrNotExist) { // 错误存在 && 错误不是文件不存在
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}

type Config struct {
	Server   HttpConfig
	Database DatabaseConfig
	Jwt      JwtConfig
	Storage  StorageConfig
}

func LoadConfig() (*Config, error) {
	if err := LoadEnvironment(); err != nil {
		return nil, err
	}

	jwtExpireValue := strings.TrimSpace(os.Getenv("JWT_EXPIRE"))
	jwtExpire, err := time.ParseDuration(jwtExpireValue)
	if err != nil {
		// Keep compatibility with values such as JWT_EXPIRE=3600, interpreted as seconds.
		jwtExpire, err = time.ParseDuration(jwtExpireValue + "s")
	}
	if err != nil || jwtExpire <= 0 {
		return nil, fmt.Errorf("JWT_EXPIRE must be a positive duration (for example 3600 or 1h)")
	}

	databaseConfig := databaseConfigFromEnvironment()
	if err := databaseConfig.Validate(); err != nil {
		return nil, err
	}

	config := &Config{
		Server: HttpConfig{
			Port:   strings.TrimSpace(os.Getenv("PORT")),
			Listen: strings.TrimSpace(os.Getenv("LISTEN")),
		},
		Database: databaseConfig,
		Jwt: JwtConfig{
			Secret: strings.TrimSpace(os.Getenv("JWT_SECRET")),
			Issuer: strings.TrimSpace(os.Getenv("JWT_ISSUER")),
			Expire: jwtExpire,
		},
		Storage: StorageConfig{
			Root: strings.TrimSpace(os.Getenv("STORAGE_ROOT")),
		},
	}
	if config.Storage.Root == "" {
		config.Storage.Root = "data"
	}
	config.Storage.CleanupInterval, err = optionalPositiveDuration("STORAGE_CLEANUP_INTERVAL", 6*time.Hour)
	if err != nil {
		return nil, err
	}
	config.Storage.OrphanGrace, err = optionalPositiveDuration("STORAGE_ORPHAN_GRACE", 24*time.Hour)
	if err != nil {
		return nil, err
	}
	config.Storage.CleanupBatch, err = optionalPositiveInt("STORAGE_CLEANUP_BATCH", 1000)
	if err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate ensures the values required to start the application are present.
func (c *Config) Validate() error {
	if c.Server.Listen == "" {
		return fmt.Errorf("LISTEN must be set")
	}
	if c.Server.Port == "" {
		return fmt.Errorf("PORT must be set")
	}
	return nil
}

type HttpConfig struct {
	Port   string
	Listen string
}

type DatabaseConfig struct {
	Username     string
	Password     string
	Host         string
	DatabaseName string
}

func LoadDatabaseConfig() (DatabaseConfig, error) {
	if err := LoadEnvironment(); err != nil {
		return DatabaseConfig{}, err
	}
	config := databaseConfigFromEnvironment()
	return config, config.Validate()
}

func databaseConfigFromEnvironment() DatabaseConfig {
	return DatabaseConfig{
		Host:         strings.TrimSpace(os.Getenv("DB_HOST")),
		Username:     strings.TrimSpace(os.Getenv("DB_USER")),
		Password:     os.Getenv("DB_PASSWORD"),
		DatabaseName: strings.TrimSpace(os.Getenv("DB_NAME")),
	}
}

func (c DatabaseConfig) Validate() error {
	if c.Host == "" || c.Username == "" || c.DatabaseName == "" {
		return fmt.Errorf("DB_HOST, DB_USER and DB_NAME must be set")
	}
	return nil
}

type JwtConfig struct {
	Secret string
	Issuer string
	Expire time.Duration
}

type StorageConfig struct {
	Root            string
	CleanupInterval time.Duration
	OrphanGrace     time.Duration
	CleanupBatch    int
}

func optionalPositiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func optionalPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

package setting

import (
	"errors"
	"fmt"
	"net"
	"net/url"
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
	Abuse    AbuseProtectionConfig
	Upload   UploadConfig
	Security SecurityConfig
}

func LoadConfig() (*Config, error) {
	if err := LoadEnvironment(); err != nil {
		return nil, err
	}
	environment := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if environment == "" {
		environment = "development"
	}
	production := environment == "production"
	jwtSecret, jwtSecretSource, err := loadJWTSecret()
	if err != nil {
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
			Secret:       jwtSecret,
			SecretSource: jwtSecretSource,
			Issuer:       strings.TrimSpace(os.Getenv("JWT_ISSUER")),
			Expire:       jwtExpire,
		},
		Storage: StorageConfig{
			Root:              strings.TrimSpace(os.Getenv("STORAGE_ROOT")),
			Backend:           strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND"))),
			GRPCAddress:       strings.TrimSpace(os.Getenv("STORAGE_ADDRESS")),
			TLSCAFile:         strings.TrimSpace(os.Getenv("STORAGE_TLS_CA")),
			TLSServerName:     strings.TrimSpace(os.Getenv("STORAGE_TLS_SERVER_NAME")),
			TLSClientCertFile: strings.TrimSpace(os.Getenv("STORAGE_TLS_CLIENT_CERT")),
			TLSClientKeyFile:  strings.TrimSpace(os.Getenv("STORAGE_TLS_CLIENT_KEY")),
		},
		Abuse: AbuseProtectionConfig{
			TrustedProxies: splitCommaSeparated(os.Getenv("TRUSTED_PROXIES")),
		},
		Security: SecurityConfig{
			Environment:        environment,
			CORSAllowedOrigins: splitCommaSeparated(os.Getenv("CORS_ALLOWED_ORIGINS")),
			SwaggerUsername:    strings.TrimSpace(os.Getenv("SWAGGER_USERNAME")),
			SwaggerPassword:    os.Getenv("SWAGGER_PASSWORD"),
			RequireHTTPS:       production,
		},
	}
	config.Security.SwaggerEnabled, err = optionalBool("SWAGGER_ENABLED", !production)
	if err != nil {
		return nil, err
	}
	if len(config.Abuse.TrustedProxies) == 0 {
		config.Abuse.TrustedProxies = []string{"127.0.0.1", "::1"}
	}
	if config.Storage.Backend == "" {
		config.Storage.Backend = "local"
	}
	if config.Storage.Root == "" {
		config.Storage.Root = "data"
	}
	config.Server.ReadHeaderTimeout, err = optionalPositiveDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	config.Server.ReadTimeout, err = optionalPositiveDuration("HTTP_READ_TIMEOUT", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	config.Server.WriteTimeout, err = optionalPositiveDuration("HTTP_WRITE_TIMEOUT", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	config.Server.IdleTimeout, err = optionalPositiveDuration("HTTP_IDLE_TIMEOUT", 2*time.Minute)
	if err != nil {
		return nil, err
	}
	config.Server.ShutdownTimeout, err = optionalPositiveDuration("HTTP_SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	config.Server.CleanupShutdownTimeout, err = optionalPositiveDuration("CLEANUP_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	config.Server.MaxHeaderBytes, err = optionalPositiveInt("HTTP_MAX_HEADER_BYTES", 64<<10)
	if err != nil {
		return nil, err
	}
	config.Upload.MaxFileBytes, err = optionalPositiveInt64("UPLOAD_MAX_BYTES", 1<<30)
	if err != nil {
		return nil, err
	}
	config.Upload.MultipartOverheadBytes, err = optionalPositiveInt64("UPLOAD_MULTIPART_OVERHEAD_BYTES", 1<<20)
	if err != nil {
		return nil, err
	}
	config.Upload.ConcurrentPerUser, err = optionalPositiveInt("UPLOAD_CONCURRENT_PER_USER", 2)
	if err != nil {
		return nil, err
	}
	config.Abuse.AuthPerMinute, err = optionalPositiveInt("AUTH_RATE_PER_MINUTE", 20)
	if err != nil {
		return nil, err
	}
	config.Abuse.AuthBurst, err = optionalPositiveInt("AUTH_RATE_BURST", 5)
	if err != nil {
		return nil, err
	}
	config.Abuse.LoginFailureLimit, err = optionalPositiveInt("LOGIN_FAILURE_LIMIT", 5)
	if err != nil {
		return nil, err
	}
	config.Abuse.LoginBlockDuration, err = optionalPositiveDuration("LOGIN_BLOCK_DURATION", 5*time.Minute)
	if err != nil {
		return nil, err
	}
	config.Abuse.UploadPerMinute, err = optionalPositiveInt("UPLOAD_RATE_PER_MINUTE", 30)
	if err != nil {
		return nil, err
	}
	config.Abuse.UploadBurst, err = optionalPositiveInt("UPLOAD_RATE_BURST", 3)
	if err != nil {
		return nil, err
	}
	config.Abuse.SearchPerMinute, err = optionalPositiveInt("SEARCH_RATE_PER_MINUTE", 120)
	if err != nil {
		return nil, err
	}
	config.Abuse.SearchBurst, err = optionalPositiveInt("SEARCH_RATE_BURST", 20)
	if err != nil {
		return nil, err
	}
	config.Storage.GRPCTimeout, err = optionalPositiveDuration("STORAGE_TIMEOUT", 30*time.Minute)
	if err != nil {
		return nil, err
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
	if len([]byte(c.Jwt.Secret)) < 32 {
		return fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	if distinctByteCount([]byte(c.Jwt.Secret)) < 8 {
		return fmt.Errorf("JWT secret must be randomly generated, not a repeated or low-entropy value")
	}
	if c.Jwt.Issuer == "" {
		return fmt.Errorf("JWT_ISSUER must be set")
	}
	if c.Server.Listen == "" {
		return fmt.Errorf("LISTEN must be set")
	}
	if c.Server.Port == "" {
		return fmt.Errorf("PORT must be set")
	}
	if c.Security.Environment != "development" && c.Security.Environment != "test" && c.Security.Environment != "production" {
		return fmt.Errorf("APP_ENV must be development, test, or production")
	}
	for _, origin := range c.Security.CORSAllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("CORS_ALLOWED_ORIGINS contains invalid origin %q", origin)
		}
		if c.Security.Environment == "production" && parsed.Scheme != "https" {
			return fmt.Errorf("production CORS origin %q must use https", origin)
		}
	}
	if c.Security.Environment == "production" {
		if c.Jwt.Expire > 15*time.Minute {
			return fmt.Errorf("production JWT_EXPIRE must not exceed 15m")
		}
		if !isPrivateListenAddress(c.Server.Listen) {
			return fmt.Errorf("production LISTEN must be a loopback or private IP address")
		}
		for _, proxy := range c.Abuse.TrustedProxies {
			if proxy == "*" || proxy == "0.0.0.0/0" || proxy == "::/0" {
				return fmt.Errorf("production TRUSTED_PROXIES must not trust every address")
			}
		}
		if c.Security.SwaggerEnabled && (c.Security.SwaggerUsername == "" || len(c.Security.SwaggerPassword) < 16) {
			return fmt.Errorf("production Swagger requires SWAGGER_USERNAME and a SWAGGER_PASSWORD of at least 16 characters")
		}
	}
	switch c.Storage.Backend {
	case "local":
	case "grpc":
		if c.Storage.GRPCAddress == "" {
			return fmt.Errorf("STORAGE_ADDRESS must be set for gRPC storage")
		}
		if (c.Storage.TLSClientCertFile == "") != (c.Storage.TLSClientKeyFile == "") {
			return fmt.Errorf("STORAGE_TLS_CLIENT_CERT and STORAGE_TLS_CLIENT_KEY must be set together")
		}
		if c.Storage.TLSCAFile == "" && (c.Storage.TLSServerName != "" || c.Storage.TLSClientCertFile != "") {
			return fmt.Errorf("STORAGE_TLS_CA is required when TLS options are set")
		}
	default:
		return fmt.Errorf("STORAGE_BACKEND must be local or grpc")
	}
	return nil
}

type HttpConfig struct {
	Port                   string
	Listen                 string
	ReadHeaderTimeout      time.Duration
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	IdleTimeout            time.Duration
	ShutdownTimeout        time.Duration
	CleanupShutdownTimeout time.Duration
	MaxHeaderBytes         int
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
	Secret       string
	SecretSource string
	Issuer       string
	Expire       time.Duration
}

type StorageConfig struct {
	Root              string
	Backend           string
	GRPCAddress       string
	GRPCTimeout       time.Duration
	TLSCAFile         string
	TLSServerName     string
	TLSClientCertFile string
	TLSClientKeyFile  string
	CleanupInterval   time.Duration
	OrphanGrace       time.Duration
	CleanupBatch      int
}

type UploadConfig struct {
	MaxFileBytes           int64
	MultipartOverheadBytes int64
	ConcurrentPerUser      int
}

func (c UploadConfig) MaxRequestBytes() int64 {
	if c.MaxFileBytes > int64(^uint64(0)>>1)-c.MultipartOverheadBytes {
		return int64(^uint64(0) >> 1)
	}
	return c.MaxFileBytes + c.MultipartOverheadBytes
}

type AbuseProtectionConfig struct {
	TrustedProxies     []string
	AuthPerMinute      int
	AuthBurst          int
	LoginFailureLimit  int
	LoginBlockDuration time.Duration
	UploadPerMinute    int
	UploadBurst        int
	SearchPerMinute    int
	SearchBurst        int
}

type SecurityConfig struct {
	Environment        string
	CORSAllowedOrigins []string
	SwaggerEnabled     bool
	SwaggerUsername    string
	SwaggerPassword    string
	RequireHTTPS       bool
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

func optionalPositiveInt64(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func optionalBool(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func loadJWTSecret() (string, string, error) {
	direct := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	path := strings.TrimSpace(os.Getenv("JWT_SECRET_FILE"))
	if direct != "" && path != "" {
		return "", "", fmt.Errorf("set only one of JWT_SECRET or JWT_SECRET_FILE")
	}
	if path == "" {
		return direct, "environment", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read JWT_SECRET_FILE: %w", err)
	}
	return strings.TrimSpace(string(content)), "file", nil
}

func distinctByteCount(value []byte) int {
	seen := make(map[byte]struct{}, len(value))
	for _, character := range value {
		seen[character] = struct{}{}
	}
	return len(seen)
}

func isPrivateListenAddress(address string) bool {
	if strings.EqualFold(address, "localhost") {
		return true
	}
	ip := net.ParseIP(address)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

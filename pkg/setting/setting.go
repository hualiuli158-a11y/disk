package setting

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// 如何提取出来.env文件中的数据
func LoadEnvironment() error {
	return godotenv.Load(".env")
}

func LoadDatabase() {

}

type Config struct {
	Server   HttpConfig
	Database DatabaseConfig
}

func LoadConfig() (*Config, error) {
	if err := LoadEnvironment(); err != nil {
		return nil, fmt.Errorf("load .env: %w", err)
	}

	config := &Config{
		Server: HttpConfig{
			Port:   strings.TrimSpace(os.Getenv("PORT")),
			Listen: strings.TrimSpace(os.Getenv("LISTEN")),
		},
		Database: DatabaseConfig{
			Host:         strings.TrimSpace(os.Getenv("DB_HOST")),
			Username:     strings.TrimSpace(os.Getenv("DB_USER")),
			Password:     os.Getenv("DB_PASSWORD"),
			DatabaseName: strings.TrimSpace(os.Getenv("DB_NAME")),
		},
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

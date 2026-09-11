package setting

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
func LoadDatabase() {

}

type Config struct {
	Server   HttpConfig
	Database DatabaseConfig
}

func LoadConfig() (*Config, error) {
	if err := LoadEnvironment(); err != nil {
		return nil, err
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

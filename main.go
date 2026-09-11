package main

import (
	"disk/internal/router"
	"disk/pkg/logger"
	"disk/pkg/setting"
	"net"
	"os"
)

// main
// ├─ 创建基础 Logger
// ├─ 加载并校验 Config
// ├─ 初始化数据库等资源
// ├─ 构建 Router
// ├─ 创建 http.Server
// └─ 监听系统信号并优雅关闭

func main() {

	baseLogger := logger.NewLogger(os.Stdout)
	config, err := setting.LoadConfig()
	if err != nil {
		baseLogger.Fatal(err)
	}

	if err := InitRouter(config, baseLogger); err != nil {
		baseLogger.Fatal(err)
	}
}

func InitRouter(config *setting.Config, baseLogger *logger.Logger) error {
	r, err := router.NewRouter(baseLogger)
	if err != nil {
		baseLogger.Error(err, "router.NewRouter in error")
		return err
	}

	//post := config.Server.Listen + ":" + config.Server.Port
	addr := net.JoinHostPort(config.Server.Listen, config.Server.Port)
	err = r.Run(addr) //to do http
	if err != nil {
		baseLogger.Error(err, "r.Run in error")
		return err
	}
	return nil
}

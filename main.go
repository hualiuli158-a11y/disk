package main

import (
	_ "disk/docs/swagger"
	"disk/internal/bootstrap"
	"log"
)

// @title Disk 存储服务 API
// @version 1.0
// @description Disk 后端服务接口文档，提供用户认证和文件存储能力。
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 请输入 Bearer、一个空格和访问令牌，例如：Bearer eyJhbGciOiJIUzI1NiIs...
//
//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g main.go -o docs/swagger --parseInternal
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (runErr error) {
	application, err := bootstrap.New()
	if err != nil {
		return err
	}
	defer func() {
		if err := application.Close(); err != nil && runErr == nil {
			runErr = err
		}
	}()

	return application.Run()
}

package main

import (
	_ "disk/docs/swagger"
	"disk/internal/database"
	"disk/internal/handler"
	"disk/internal/repository"
	"disk/internal/router"
	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/logger"
	"disk/pkg/setting"
	"fmt"
	"net"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// main
// ├─ 创建基础 Logger
// ├─ 加载并校验 Config
// ├─ 初始化数据库等资源
// ├─ 构建 Router
// ├─ 创建 http.Server
// └─ 监听系统信号并优雅关闭

// @title Disk 存储服务 API
// @version 1.0
// @description Disk 后端服务接口文档，提供用户认证和文件存储能力。
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 请输入 Bearer 加一个空格再加访问令牌，例如：Bearer eyJhbGciOiJIUzI1NiIs...
//
//go:generate go run github.com/swaggo/swag/cmd/swag@v1.8.12 init -g main.go -o docs/swagger --parseInternal
func main() {
	baseLogger := logger.NewLogger(os.Stdout)
	config, err := setting.LoadConfig()
	if err != nil {
		baseLogger.Fatal(err)
	}

	db, err := InitDatabase(config)
	if err != nil {
		baseLogger.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		baseLogger.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		baseLogger.Fatal(err)
	}
	defer sqlDB.Close()

	jwtService := InitJWTService(config)
	loginService := InitLoginService(db, jwtService)
	loginHandler := handler.NewLoginHandler(loginService, baseLogger)

	registerservice := InitRegisterService(db, jwtService)
	registerHandler := handler.NewRegisterHandler(registerservice, baseLogger)

	if err := InitRouter(config, baseLogger, jwtService, loginHandler, registerHandler); err != nil {
		baseLogger.Fatal(err)
	}
}

func InitRouter(
	config *setting.Config,
	baseLogger *logger.Logger,
	jwtService *app.JWTService,
	loginHandler *handler.LoginHandler,
	registerHandler *handler.RegisterHandler,
) error {
	r, err := router.NewRouter(baseLogger, jwtService, loginHandler, registerHandler)
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

func InitJWTService(config *setting.Config) *app.JWTService {
	return app.NewJWTService(config.Jwt)
}

func InitDatabase(config *setting.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.Database.Username,
		config.Database.Password,
		config.Database.Host, // 例如 127.0.0.1:3306
		config.Database.DatabaseName,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

func InitLoginService(db *gorm.DB, jwtService *app.JWTService) *service.LoginService {
	userRepository := repository.NewUserRepository(db)
	return service.NewLoginService(userRepository, jwtService)
}

func InitRegisterService(db *gorm.DB, jwtService *app.JWTService) *service.RegisterService {
	userRepository := repository.NewUserRepository(db)
	return service.NewRegisterService(userRepository, jwtService)
}

package bootstrap

import (
	"database/sql"
	"fmt"
	"net"
	"os"

	"disk/internal/database"
	"disk/internal/handler"
	"disk/internal/repository"
	"disk/internal/router"
	"disk/internal/service"
	"disk/internal/storage"
	"disk/pkg/app"
	"disk/pkg/logger"
	"disk/pkg/setting"

	"github.com/gin-gonic/gin"
)

// App 保存应用运行所需的资源，并负责它们的生命周期。
type App struct {
	config *setting.Config
	router *gin.Engine
	sqlDB  *sql.DB
}

// New 加载配置、初始化基础设施并组装所有业务依赖。
func New() (*App, error) {
	baseLogger := logger.NewLogger(os.Stdout)

	config, err := setting.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	db, err := database.OpenMySQL(config.Database)
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql database: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = sqlDB.Close()
		}
	}()

	if err := database.AutoMigrate(db); err != nil {
		return nil, err
	}

	userRepository := repository.NewUserRepository(db)
	foldersRepository := repository.NewFoldersRepository(db)
	filesRepository := repository.NewFilesRepository(db)
	objectStorage, err := storage.NewStorage(config.Storage.Root)
	if err != nil {
		return nil, fmt.Errorf("create object storage: %w", err)
	}

	jwtService := app.NewJWTService(config.Jwt)
	loginService := service.NewLoginService(userRepository, jwtService)
	registerService := service.NewRegisterService(userRepository, jwtService)
	profileService := service.NewProfileService(userRepository)
	foldersService := service.NewChildFoldersCreateService(foldersRepository)
	listFoldersService := service.NewListFoldersService(foldersRepository)
	renameFolderService := service.NewRenameFolderService(foldersRepository)
	listFolderChildrenService := service.NewListFolderChildrenService(foldersRepository)
	deleteFolderService := service.NewDeleteFolderService(foldersRepository)
	moveFolderService := service.NewMoveFolderService(foldersRepository)
	uploadFileService := service.NewUploadFileService(filesRepository, objectStorage, service.DefaultMaxUploadSize)
	listFilesService := service.NewListFilesService(filesRepository)
	downloadFileService := service.NewDownloadFileService(filesRepository, objectStorage)
	deleteFileService := service.NewDeleteFileService(filesRepository, objectStorage)
	renameFileService := service.NewRenameFileService(filesRepository)

	handlers := router.Handlers{
		Login:        handler.NewLoginHandler(loginService, baseLogger),
		Register:     handler.NewRegisterHandler(registerService, baseLogger),
		Profile:      handler.NewProfileHandler(profileService, baseLogger),
		Folders:      handler.NewChildFoldersCreateHandler(foldersService, baseLogger),
		ListFolders:  handler.NewListFoldersHandler(listFoldersService, baseLogger),
		Rename:       handler.NewRenameFolderHandler(renameFolderService, baseLogger),
		Children:     handler.NewListFolderChildrenHandler(listFolderChildrenService, baseLogger),
		Delete:       handler.NewDeleteFolderHandler(deleteFolderService, baseLogger),
		Move:         handler.NewMoveFolderHandler(moveFolderService, baseLogger),
		UploadFile:   handler.NewUploadFileHandler(uploadFileService, baseLogger),
		ListFiles:    handler.NewListFilesHandler(listFilesService, baseLogger),
		DownloadFile: handler.NewDownloadFileHandler(downloadFileService, baseLogger),
		DeleteFile:   handler.NewDeleteFileHandler(deleteFileService, baseLogger),
		RenameFile:   handler.NewRenameFileHandler(renameFileService, baseLogger),
	}
	httpRouter, err := router.NewRouter(baseLogger, jwtService, handlers)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	cleanup = false
	return &App{
		config: config,
		router: httpRouter,
		sqlDB:  sqlDB,
	}, nil
}

// Run 启动 HTTP 服务。TODO
func (a *App) Run() error {
	addr := net.JoinHostPort(a.config.Server.Listen, a.config.Server.Port)
	if err := a.router.Run(addr); err != nil {
		return fmt.Errorf("run HTTP server: %w", err)
	}
	return nil
}

// Close 释放应用持有的资源。
func (a *App) Close() error {
	if err := a.sqlDB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

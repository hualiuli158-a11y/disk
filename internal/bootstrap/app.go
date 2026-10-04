package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"time"

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
	config        *setting.Config
	router        *gin.Engine
	sqlDB         *sql.DB
	cleanupCancel context.CancelFunc
	cleanupDone   chan struct{}
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

	if err := database.RequireCurrentVersion(context.Background(), sqlDB); err != nil {
		return nil, err
	}

	userRepository := repository.NewUserRepository(db)
	foldersRepository := repository.NewFoldersRepository(db)
	filesRepository := repository.NewFilesRepository(db)
	searchRepository := repository.NewSearchRepository(db)
	cleanupRepository := repository.NewCleanupRepository(db)
	trashRepository := repository.NewTrashRepository(db)
	var objectStorage storage.Storage
	switch config.Storage.Backend {
	case "local":
		objectStorage, err = storage.NewStorage(config.Storage.Root)
	case "remote":
		objectStorage, err = storage.NewRemoteStorage(
			config.Storage.RemoteAddress, config.Storage.RemoteTimeout)
	default:
		return nil, fmt.Errorf("unsupported storage backend %q", config.Storage.Backend)
	}
	if err != nil {
		return nil, fmt.Errorf("create object storage: %w", err)
	}

	jwtService := app.NewJWTService(config.Jwt)
	loginService := service.NewLoginService(userRepository, jwtService)
	registerService := service.NewRegisterService(userRepository, jwtService)
	profileService := service.NewProfileService(userRepository)
	quotaService := service.NewQuotaService(userRepository)
	foldersService := service.NewChildFoldersCreateService(foldersRepository)
	listFoldersService := service.NewListFoldersService(foldersRepository)
	renameFolderService := service.NewRenameFolderService(foldersRepository)
	listFolderChildrenService := service.NewListFolderChildrenService(foldersRepository)
	deleteFolderService := service.NewDeleteFolderService(trashRepository)
	moveFolderService := service.NewMoveFolderService(foldersRepository)
	folderContentsService := service.NewFolderContentsService(foldersRepository)
	uploadFileService := service.NewUploadFileService(filesRepository, objectStorage, service.DefaultMaxUploadSize)
	listFilesService := service.NewListFilesService(filesRepository)
	downloadFileService := service.NewDownloadFileService(filesRepository, objectStorage)
	deleteFileService := service.NewDeleteFileService(trashRepository)
	renameFileService := service.NewRenameFileService(filesRepository)
	moveFileService := service.NewMoveFileService(filesRepository)
	searchService := service.NewSearchService(searchRepository)
	trashService := service.NewTrashService(trashRepository, objectStorage)
	orphanCleanupService := service.NewOrphanCleanupService(
		cleanupRepository,
		objectStorage,
		config.Storage.OrphanGrace,
		config.Storage.CleanupBatch,
	)

	handlers := router.Handlers{
		Login:        handler.NewLoginHandler(loginService, baseLogger),
		Register:     handler.NewRegisterHandler(registerService, baseLogger),
		Profile:      handler.NewProfileHandler(profileService, baseLogger),
		Quota:        handler.NewQuotaHandler(quotaService, baseLogger),
		Folders:      handler.NewChildFoldersCreateHandler(foldersService, baseLogger),
		ListFolders:  handler.NewListFoldersHandler(listFoldersService, baseLogger),
		Rename:       handler.NewRenameFolderHandler(renameFolderService, baseLogger),
		Children:     handler.NewListFolderChildrenHandler(listFolderChildrenService, baseLogger),
		Delete:       handler.NewDeleteFolderHandler(deleteFolderService, baseLogger),
		Move:         handler.NewMoveFolderHandler(moveFolderService, baseLogger),
		Contents:     handler.NewFolderContentsHandler(folderContentsService, baseLogger),
		UploadFile:   handler.NewUploadFileHandler(uploadFileService, baseLogger),
		ListFiles:    handler.NewListFilesHandler(listFilesService, baseLogger),
		DownloadFile: handler.NewDownloadFileHandler(downloadFileService, baseLogger),
		DeleteFile:   handler.NewDeleteFileHandler(deleteFileService, baseLogger),
		RenameFile:   handler.NewRenameFileHandler(renameFileService, baseLogger),
		MoveFile:     handler.NewMoveFileHandler(moveFileService, baseLogger),
		Search:       handler.NewSearchHandler(searchService, baseLogger),
		Trash:        handler.NewTrashHandler(trashService, baseLogger),
	}
	httpRouter, err := router.NewRouter(baseLogger, jwtService, handlers)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{})
	go runOrphanCleanupWorker(
		cleanupCtx,
		orphanCleanupService,
		trashService,
		config.Storage.CleanupInterval,
		config.Storage.CleanupBatch,
		baseLogger.WithFields(logger.Fields{"module": "orphan_cleanup"}),
		cleanupDone,
	)

	cleanup = false
	return &App{
		config:        config,
		router:        httpRouter,
		sqlDB:         sqlDB,
		cleanupCancel: cleanupCancel,
		cleanupDone:   cleanupDone,
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
	if a.cleanupCancel != nil {
		a.cleanupCancel()
	}
	if a.cleanupDone != nil {
		<-a.cleanupDone
	}
	if err := a.sqlDB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

func runOrphanCleanupWorker(
	ctx context.Context,
	cleanup *service.OrphanCleanupService,
	trash *service.TrashService,
	interval time.Duration,
	batch int,
	workerLogger *logger.Logger,
	done chan<- struct{},
) {
	defer close(done)
	run := func() {
		result, err := cleanup.Run(ctx)
		if err != nil {
			if ctx.Err() == nil {
				workerLogger.Errorf("orphan cleanup failed: %v", err)
			}
			return
		}
		workerLogger.WithFields(logger.Fields{
			"database_candidates": result.DatabaseCandidates,
			"database_deleted":    result.DatabaseDeleted,
			"storage_scanned":     result.StorageScanned,
			"storage_deleted":     result.StorageDeleted,
			"failures":            result.Failures,
		}).Info("orphan cleanup completed")

		trashResult, err := trash.PurgeExpiredTrash(ctx, time.Now(), batch)
		if err != nil {
			if ctx.Err() == nil {
				workerLogger.Errorf("expired trash cleanup failed: %v", err)
			}
			return
		}
		workerLogger.WithFields(logger.Fields{
			"trash_candidates":       trashResult.Candidates,
			"trash_deleted":          trashResult.Deleted,
			"trash_cleanup_failures": trashResult.CleanupFailures,
			"trash_failures":         trashResult.Failures,
		}).Info("expired trash cleanup completed")
	}

	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

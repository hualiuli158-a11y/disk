package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"disk/internal/database"
	"disk/internal/handler"
	"disk/internal/repository"
	"disk/internal/router"
	"disk/internal/service"
	"disk/internal/storage"
	"disk/internal/storage/grpcstore"
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
	storageCloser io.Closer
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
	var storageCloser io.Closer
	cleanup := true
	defer func() {
		if cleanup {
			if storageCloser != nil {
				_ = storageCloser.Close()
			}
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
	objectStorage, storageCloser, err := newObjectStorage(config.Storage)
	if err != nil {
		return nil, fmt.Errorf("create object storage: %w", err)
	}
	if err := requireStorageReady(objectStorage, config.Storage.Backend, config.Storage.GRPCAddress); err != nil {
		return nil, err
	}
	baseLogger.Infof("object storage backend ready: %s", config.Storage.Backend)
	var storageUnavailable atomic.Bool

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
	httpRouter.GET("/livez", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	})
	httpRouter.GET("/readyz", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(checkCtx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "component": "database"})
			return
		}
		if checker, ok := objectStorage.(interface{ Check(context.Context) error }); ok {
			if err := checker.Check(checkCtx); err != nil {
				if !storageUnavailable.Swap(true) {
					baseLogger.Errorf("storage backend %q became unavailable: %v", config.Storage.Backend, err)
				}
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "component": "storage"})
				return
			}
			if storageUnavailable.Swap(false) {
				baseLogger.Infof("storage backend %q is available again", config.Storage.Backend)
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "storage_backend": config.Storage.Backend})
	})

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
		storageCloser: storageCloser,
	}, nil
}

func newObjectStorage(config setting.StorageConfig) (storage.Storage, io.Closer, error) {
	switch config.Backend {
	case "local":
		objects, err := storage.NewStorage(config.Root)
		return objects, nil, err
	case "grpc":
		objects, err := grpcstore.NewWithTLS(config.GRPCAddress, config.GRPCTimeout, grpcstore.TLSOptions{
			CAFile: config.TLSCAFile, ServerName: config.TLSServerName,
			ClientCertFile: config.TLSClientCertFile, ClientKeyFile: config.TLSClientKeyFile,
		})
		if err != nil {
			return nil, nil, err
		}
		return objects, objects, nil
	default:
		return nil, nil, fmt.Errorf("unsupported storage backend %q", config.Backend)
	}
}

func requireStorageReady(objects storage.Storage, backend, address string) error {
	checker, ok := objects.(interface{ Check(context.Context) error })
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := checker.Check(ctx); err != nil {
		return fmt.Errorf("storage backend %q at %q unavailable at startup: %w", backend, address, err)
	}
	return nil
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
	var storageErr error
	if a.storageCloser != nil {
		storageErr = a.storageCloser.Close()
	}
	return errors.Join(storageErr, a.sqlDB.Close())
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
		} else {
			workerLogger.WithFields(logger.Fields{
				"database_candidates": result.DatabaseCandidates,
				"database_deleted":    result.DatabaseDeleted,
				"storage_scanned":     result.StorageScanned,
				"storage_deleted":     result.StorageDeleted,
				"failures":            result.Failures,
			}).Info("orphan cleanup completed")
		}
		if ctx.Err() != nil {
			return
		}

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

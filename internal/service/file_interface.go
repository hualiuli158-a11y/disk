package service

import (
	"context"
	"io"
	"time"

	"disk/internal/model"
	"disk/internal/storage"
)

type UploadFileRepository interface {
	CreateUploadedFile(
		ctx context.Context,
		userID uint64,
		folderID uint64,
		name string,
		sha256Value string,
		size int64,
		storageKey string,
	) (*model.File, *model.PhysicalFile, bool, error)
}

type ListFilesRepository interface {
	ListFiles(ctx context.Context, userID uint64, folderID uint64) ([]model.File, error)
}

type DownloadFileRepository interface {
	FindFileForDownload(ctx context.Context, userID uint64, fileID uint64) (*model.File, error)
}

type DeleteFileRepository interface {
	TrashFile(ctx context.Context, userID uint64, fileID uint64, trashedAt time.Time, expiresAt time.Time) error
}

type RenameFileRepository interface {
	RenameFile(ctx context.Context, userID uint64, fileID uint64, name string) error
}

type MoveFileRepository interface {
	MoveFile(ctx context.Context, userID uint64, fileID uint64, folderID uint64) error
}

type UpdateFileContentRepository interface {
	ReplaceFileContent(
		ctx context.Context,
		userID uint64,
		fileID uint64,
		expectedSHA256 string,
		sha256Value string,
		size int64,
		storageKey string,
	) (*model.File, *model.PhysicalFile, bool, error)
}

type ObjectStorage interface {
	Save(ctx context.Context, key string, reader io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error)
}

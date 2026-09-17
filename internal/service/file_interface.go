package service

import (
	"context"
	"io"

	"disk/internal/model"
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
	DeleteFile(ctx context.Context, userID uint64, fileID uint64) (storageKey string, deletePhysical bool, err error)
}

type RenameFileRepository interface {
	RenameFile(ctx context.Context, userID uint64, fileID uint64, name string) error
}

type ObjectStorage interface {
	Save(ctx context.Context, key string, reader io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

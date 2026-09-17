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

type ObjectStorage interface {
	Save(ctx context.Context, key string, reader io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

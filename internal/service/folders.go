package service

import (
	"context"
	"errors"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
)

var (
	ErrFolderUserNotFound      = errors.New("用户不存在")
	ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")
)

type FoldersRepository interface {
	CreateFoldersByID(ctx context.Context, userID uint64) (*model.Folder, error)
}

type FoldersCreateService struct {
	folders FoldersRepository
}

type CreateRootFolderResult struct {
	ID        uint64
	UserID    uint64
	Name      string
	CreatedAt time.Time
}

func NewFoldersCreateService(folders FoldersRepository) *FoldersCreateService {
	return &FoldersCreateService{folders: folders}
}

func (s *FoldersCreateService) CreateFolder(ctx context.Context, userID uint64) (*CreateRootFolderResult, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}

	folder, err := s.folders.CreateFoldersByID(ctx, userID)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrFolderUserNotFound
	case errors.Is(err, repository.ErrRootFolderAlreadyExists):
		return nil, ErrRootFolderAlreadyExists
	case err != nil:
		return nil, err
	}

	return &CreateRootFolderResult{
		ID:        folder.ID,
		UserID:    folder.UserID,
		Name:      folder.Name,
		CreatedAt: folder.CreatedAt,
	}, nil
}

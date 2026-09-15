package service

import (
	"context"
	"disk/internal/repository"
	"errors"
	"strings"
	"time"
)

var (
	ErrFolderUserNotFound      = errors.New("用户不存在")
	ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")

	ErrInvalidFolderName   = errors.New("文件夹名称不合法")
	ErrParentNotFound      = errors.New("父目录不存在")
	ErrFolderAlreadyExists = errors.New("同名文件夹已存在")
)

type ChildFoldersCreateService struct {
	folders FoldersRepository
}

func NewChildFoldersCreateService(folders FoldersRepository) *ChildFoldersCreateService {
	return &ChildFoldersCreateService{folders: folders}
}

type CreateFolderInput struct {
	UserID   uint64
	ParentID uint64
	Name     string
}

type CreateFolderOutput struct {
	ID        uint64
	ParentID  uint64
	Name      string
	CreatedAt time.Time
}

func (ch *ChildFoldersCreateService) CreateChildFolder(ctx context.Context, input CreateFolderInput) (*CreateFolderOutput, error) {
	name := strings.TrimSpace(input.Name)

	if input.UserID == 0 ||
		input.ParentID == 0 ||
		name == "" ||
		name == "/" ||
		name == "." ||
		name == ".." ||
		strings.ContainsAny(name, `/\`) {
		return nil, ErrInvalidFolderName
	}
	//走到这里其实参数一定不会缺失，如果缺失的话，在上一层就会发生错误了
	output, err := ch.folders.CreateChildFolder(ctx, input.UserID, input.ParentID, input.Name)
	switch {
	case errors.Is(err, repository.ErrParentNotFound):
		return nil, ErrParentNotFound

	case errors.Is(err, repository.ErrFolderAlreadyExists):
		return nil, ErrFolderAlreadyExists

	case err != nil:
		return nil, err
	}
	return &CreateFolderOutput{
		ID:        output.ID,
		ParentID:  *output.ParentID,
		Name:      name,
		CreatedAt: output.CreatedAt,
	}, nil
}

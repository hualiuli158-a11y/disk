package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"disk/internal/repository"
)

var (
	ErrFolderUserNotFound  = errors.New("用户不存在")
	ErrInvalidFolderName   = errors.New("文件夹名称不合法")
	ErrParentNotFound      = errors.New("父目录不存在")
	ErrFolderAlreadyExists = errors.New("同名文件夹已存在")
	ErrRootFolderNotFound  = errors.New("根目录不存在")
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
	output, err := ch.folders.CreateChildFolder(ctx, input.UserID, input.ParentID, name)
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
		ParentID:  input.ParentID,
		Name:      output.Name,
		CreatedAt: output.CreatedAt,
	}, nil
}

type ListFoldersService struct {
	folders RootChildrenRepository
}

func NewListFoldersService(folders RootChildrenRepository) *ListFoldersService {
	return &ListFoldersService{folders: folders}
}

type FolderListItem struct {
	ID        uint64
	ParentID  *uint64
	Name      string
	CreatedAt time.Time
}

type ListFoldersOutput struct {
	Root     FolderListItem
	Children []FolderListItem
}

func (l *ListFoldersService) ListFolders(ctx context.Context, userID uint64) (*ListFoldersOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}

	root, err := l.folders.ListRootChildren(ctx, userID)
	switch {
	case errors.Is(err, repository.ErrRootFolderNotFound):
		return nil, ErrRootFolderNotFound
	case err != nil:
		return nil, err
	}
	if root == nil {
		return nil, ErrRootFolderNotFound
	}

	children := make([]FolderListItem, 0, len(root.Children))
	for _, child := range root.Children {
		children = append(children, FolderListItem{
			ID:        child.ID,
			ParentID:  child.ParentID,
			Name:      child.Name,
			CreatedAt: child.CreatedAt,
		})
	}

	return &ListFoldersOutput{
		Root: FolderListItem{
			ID:        root.ID,
			ParentID:  root.ParentID,
			Name:      root.Name,
			CreatedAt: root.CreatedAt,
		},
		Children: children,
	}, nil
}

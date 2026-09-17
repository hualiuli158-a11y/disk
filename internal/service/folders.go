package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"disk/internal/repository"
)

var (
	ErrFolderUserNotFound  = errors.New("用户不存在")
	ErrInvalidFolderName   = errors.New("文件夹名称不合法")
	ErrParentNotFound      = errors.New("父目录不存在")
	ErrFolderAlreadyExists = errors.New("同名文件夹已存在")
	ErrRootFolderNotFound  = errors.New("根目录不存在")
	ErrFolderNotFound      = errors.New("文件夹不存在")
)

func normalizeFolderName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" ||
		name == "/" ||
		name == "." ||
		name == ".." ||
		strings.ContainsAny(name, `/\`) ||
		utf8.RuneCountInString(name) > 255 {
		return "", ErrInvalidFolderName
	}
	return name, nil
}

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
	if input.UserID == 0 || input.ParentID == 0 {
		return nil, ErrInvalidFolderName
	}
	name, err := normalizeFolderName(input.Name)
	if err != nil {
		return nil, err
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

type RenameFolderService struct {
	folders RenameFolderRepository
}

func NewRenameFolderService(folders RenameFolderRepository) *RenameFolderService {
	return &RenameFolderService{folders: folders}
}

type RenameFolderInput struct {
	UserID   uint64
	FolderID uint64
	Name     string
}

type RenameFolderOutput struct {
	ID   uint64
	Name string
}

func (s *RenameFolderService) RenameFolder(ctx context.Context, input RenameFolderInput) (*RenameFolderOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FolderID == 0 {
		return nil, ErrFolderNotFound
	}

	name, err := normalizeFolderName(input.Name)
	if err != nil {
		return nil, err
	}

	err = s.folders.RenameFolder(ctx, input.UserID, input.FolderID, name)
	switch {
	case errors.Is(err, repository.ErrFolderNotFound):
		return nil, ErrFolderNotFound
	case errors.Is(err, repository.ErrFolderAlreadyExists):
		return nil, ErrFolderAlreadyExists
	case err != nil:
		return nil, err
	}

	return &RenameFolderOutput{ID: input.FolderID, Name: name}, nil
}

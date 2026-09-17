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
	ErrCannotDeleteRoot    = errors.New("根目录不能删除")
	ErrFolderNotEmpty      = errors.New("文件夹不为空")
	ErrCannotMoveRoot      = errors.New("根目录不能移动")
	ErrInvalidFolderMove   = errors.New("不能将文件夹移动到自身或其子目录")
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

type ListFolderChildrenService struct {
	folders FolderChildrenRepository
}

func NewListFolderChildrenService(folders FolderChildrenRepository) *ListFolderChildrenService {
	return &ListFolderChildrenService{folders: folders}
}

type ListFolderChildrenInput struct {
	UserID   uint64
	FolderID uint64
}

type ListFolderChildrenOutput struct {
	Folder   FolderListItem
	Children []FolderListItem
}

func (s *ListFolderChildrenService) ListChildren(ctx context.Context, input ListFolderChildrenInput) (*ListFolderChildrenOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FolderID == 0 {
		return nil, ErrFolderNotFound
	}

	folder, err := s.folders.ListChildren(ctx, input.UserID, input.FolderID)
	switch {
	case errors.Is(err, repository.ErrFolderNotFound):
		return nil, ErrFolderNotFound
	case err != nil:
		return nil, err
	}
	if folder == nil {
		return nil, ErrFolderNotFound
	}

	children := make([]FolderListItem, 0, len(folder.Children))
	for _, child := range folder.Children {
		children = append(children, FolderListItem{
			ID:        child.ID,
			ParentID:  child.ParentID,
			Name:      child.Name,
			CreatedAt: child.CreatedAt,
		})
	}

	return &ListFolderChildrenOutput{
		Folder: FolderListItem{
			ID:        folder.ID,
			ParentID:  folder.ParentID,
			Name:      folder.Name,
			CreatedAt: folder.CreatedAt,
		},
		Children: children,
	}, nil
}

type DeleteFolderService struct {
	folders DeleteFolderRepository
}

func NewDeleteFolderService(folders DeleteFolderRepository) *DeleteFolderService {
	return &DeleteFolderService{folders: folders}
}

type DeleteFolderInput struct {
	UserID   uint64
	FolderID uint64
}

func (s *DeleteFolderService) DeleteFolder(ctx context.Context, input DeleteFolderInput) error {
	if input.UserID == 0 {
		return ErrFolderUserNotFound
	}
	if input.FolderID == 0 {
		return ErrFolderNotFound
	}

	err := s.folders.DeleteEmptyFolder(ctx, input.UserID, input.FolderID)
	switch {
	case errors.Is(err, repository.ErrFolderNotFound):
		return ErrFolderNotFound
	case errors.Is(err, repository.ErrCannotDeleteRoot):
		return ErrCannotDeleteRoot
	case errors.Is(err, repository.ErrFolderNotEmpty):
		return ErrFolderNotEmpty
	case err != nil:
		return err
	default:
		return nil
	}
}

type MoveFolderService struct {
	folders MoveFolderRepository
}

func NewMoveFolderService(folders MoveFolderRepository) *MoveFolderService {
	return &MoveFolderService{folders: folders}
}

type MoveFolderInput struct {
	UserID      uint64
	FolderID    uint64
	NewParentID uint64
}

type MoveFolderOutput struct {
	ID       uint64
	ParentID uint64
}

func (s *MoveFolderService) MoveFolder(ctx context.Context, input MoveFolderInput) (*MoveFolderOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FolderID == 0 || input.NewParentID == 0 {
		return nil, ErrFolderNotFound
	}

	err := s.folders.MoveFolder(ctx, input.UserID, input.FolderID, input.NewParentID)
	switch {
	case errors.Is(err, repository.ErrFolderNotFound):
		return nil, ErrFolderNotFound
	case errors.Is(err, repository.ErrCannotMoveRoot):
		return nil, ErrCannotMoveRoot
	case errors.Is(err, repository.ErrInvalidFolderMove):
		return nil, ErrInvalidFolderMove
	case errors.Is(err, repository.ErrFolderAlreadyExists):
		return nil, ErrFolderAlreadyExists
	case err != nil:
		return nil, err
	}

	return &MoveFolderOutput{ID: input.FolderID, ParentID: input.NewParentID}, nil
}

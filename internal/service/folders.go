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
	ErrCannotMoveRoot      = errors.New("根目录不能移动")
	ErrInvalidFolderMove   = errors.New("不能将文件夹移动到自身或其子目录")
	ErrInvalidContentQuery = errors.New("目录内容查询参数不合法")
)

const DefaultTrashRetention = 30 * 24 * time.Hour

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
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrFolderUserNotFound
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
	now     func() time.Time
}

func NewDeleteFolderService(folders DeleteFolderRepository) *DeleteFolderService {
	return &DeleteFolderService{folders: folders, now: time.Now}
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

	now := s.now()
	err := s.folders.TrashFolder(ctx, input.UserID, input.FolderID, now, now.Add(DefaultTrashRetention))
	switch {
	case errors.Is(err, repository.ErrFolderNotFound):
		return ErrFolderNotFound
	case errors.Is(err, repository.ErrCannotDeleteRoot):
		return ErrCannotDeleteRoot
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

const (
	DefaultContentPageSize = 50
	MaxContentPageSize     = 100
)

type FolderContentsService struct {
	folders FolderContentsRepository
}

func NewFolderContentsService(folders FolderContentsRepository) *FolderContentsService {
	return &FolderContentsService{folders: folders}
}

type FolderContentsInput struct {
	UserID    uint64
	FolderID  uint64
	Page      int
	PageSize  int
	SortField string
	SortOrder string
}

type ContentItem struct {
	ID        uint64
	Kind      string
	ParentID  *uint64
	Name      string
	Size      *int64
	SHA256    *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FolderContentsOutput struct {
	Folder     FolderListItem
	Items      []ContentItem
	Page       int
	PageSize   int
	Total      int64
	TotalPages int64
	SortField  string
	SortOrder  string
}

func (s *FolderContentsService) ListContents(ctx context.Context, input FolderContentsInput) (*FolderContentsOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FolderID == 0 {
		return nil, ErrFolderNotFound
	}
	if input.Page == 0 {
		input.Page = 1
	}
	if input.PageSize == 0 {
		input.PageSize = DefaultContentPageSize
	}
	if input.SortField == "" {
		input.SortField = "name"
	}
	if input.SortOrder == "" {
		input.SortOrder = "asc"
	}
	validSort := input.SortField == "name" || input.SortField == "size" || input.SortField == "created_at" || input.SortField == "updated_at"
	validOrder := input.SortOrder == "asc" || input.SortOrder == "desc"
	maxInt := int(^uint(0) >> 1)
	if input.Page < 1 || input.PageSize < 1 || input.PageSize > MaxContentPageSize || !validSort || !validOrder || input.Page > maxInt/input.PageSize {
		return nil, ErrInvalidContentQuery
	}

	offset := (input.Page - 1) * input.PageSize
	folder, contents, total, err := s.folders.ListContents(
		ctx,
		input.UserID,
		input.FolderID,
		offset,
		input.PageSize,
		input.SortField,
		input.SortOrder,
	)
	switch {
	case errors.Is(err, repository.ErrFolderNotFound):
		return nil, ErrFolderNotFound
	case err != nil:
		return nil, err
	}
	if folder == nil {
		return nil, ErrFolderNotFound
	}

	items := make([]ContentItem, 0, len(contents))
	for _, item := range contents {
		items = append(items, ContentItem{
			ID:        item.ID,
			Kind:      item.Kind,
			ParentID:  item.ParentID,
			Name:      item.Name,
			Size:      item.Size,
			SHA256:    item.SHA256,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}

	totalPages := int64(0)
	if total > 0 {
		totalPages = (total + int64(input.PageSize) - 1) / int64(input.PageSize)
	}
	return &FolderContentsOutput{
		Folder: FolderListItem{
			ID:        folder.ID,
			ParentID:  folder.ParentID,
			Name:      folder.Name,
			CreatedAt: folder.CreatedAt,
		},
		Items:      items,
		Page:       input.Page,
		PageSize:   input.PageSize,
		Total:      total,
		TotalPages: totalPages,
		SortField:  input.SortField,
		SortOrder:  input.SortOrder,
	}, nil
}

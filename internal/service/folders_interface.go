package service

import (
	"context"
	"disk/internal/model"
)

type FoldersRepository interface {
	//CreateFoldersByID(ctx context.Context, userID uint64) (*model.Folder, error)
	CreateChildFolder(ctx context.Context, userID uint64, parentID uint64, name string) (*model.Folder, error)
}

type RootChildrenRepository interface {
	ListRootChildren(ctx context.Context, userID uint64) (*model.Folder, error)
}

type RenameFolderRepository interface {
	RenameFolder(ctx context.Context, userID uint64, folderID uint64, name string) error
}

type FolderChildrenRepository interface {
	ListChildren(ctx context.Context, userID uint64, folderID uint64) (*model.Folder, error)
}

type DeleteFolderRepository interface {
	DeleteEmptyFolder(ctx context.Context, userID uint64, folderID uint64) error
}

type MoveFolderRepository interface {
	MoveFolder(ctx context.Context, userID uint64, folderID uint64, newParentID uint64) error
}

type FolderContentsRepository interface {
	ListContents(
		ctx context.Context,
		userID uint64,
		folderID uint64,
		offset int,
		limit int,
		sortField string,
		sortOrder string,
	) (*model.Folder, []model.ContentItem, int64, error)
}

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

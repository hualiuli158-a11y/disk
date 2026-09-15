package service

import (
	"errors"
)

var (
	ErrFolderUserNotFound      = errors.New("用户不存在")
	ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")
)

type ChildFoldersCreateService struct {
	folders FoldersRepository
}

package repository

import (
	"context"
	"errors"
	"fmt"

	"disk/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrFileFolderNotFound = errors.New("文件夹不存在")
	ErrFileAlreadyExists  = errors.New("同名文件已存在")
	ErrFileNotFound       = errors.New("文件不存在")
)

type FilesRepository struct {
	db *gorm.DB
}

func NewFilesRepository(db *gorm.DB) *FilesRepository {
	return &FilesRepository{db: db}
}

// CreateUploadedFile 创建逻辑文件，并按 SHA-256 复用或创建物理文件。
// 返回值 physicalCreated 表示本次提供的 storageKey 是否被数据库采用。
func (r *FilesRepository) CreateUploadedFile(
	ctx context.Context,
	userID uint64,
	folderID uint64,
	name string,
	sha256Value string,
	size int64,
	storageKey string,
) (*model.File, *model.PhysicalFile, bool, error) {
	var logical model.File
	var physical model.PhysicalFile
	physicalCreated := false

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", userID).
			First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrUserNotFound
		case err != nil:
			return fmt.Errorf("find user %d for upload: %w", userID, err)
		}

		var folder model.Folder
		err = tx.
			Select("id").
			Where("id = ? AND user_id = ?", folderID, userID).
			First(&folder).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileFolderNotFound
		case err != nil:
			return fmt.Errorf("find upload folder %d: %w", folderID, err)
		}

		physical = model.PhysicalFile{
			SHA256:     sha256Value,
			Size:       size,
			StorageKey: storageKey,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&physical)
		if result.Error != nil {
			return fmt.Errorf("create physical file: %w", result.Error)
		}
		physicalCreated = result.RowsAffected == 1
		if !physicalCreated {
			if err := tx.
				Clauses(clause.Locking{Strength: "SHARE"}).
				Where("sha256 = ?", sha256Value).
				First(&physical).Error; err != nil {
				return fmt.Errorf("find physical file by sha256: %w", err)
			}
		}

		logical = model.File{
			UserID:         userID,
			FolderID:       folderID,
			PhysicalFileID: physical.ID,
			Name:           name,
		}
		if err := tx.Create(&logical).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrFileAlreadyExists
			}
			return fmt.Errorf("create logical file: %w", err)
		}

		if err := tx.Model(&model.User{}).
			Where("id = ?", userID).
			UpdateColumn("used_quota", gorm.Expr("used_quota + ?", size)).Error; err != nil {
			return fmt.Errorf("increase used quota for user %d: %w", userID, err)
		}

		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}

	logical.PhysicalFile = physical
	return &logical, &physical, physicalCreated, nil
}

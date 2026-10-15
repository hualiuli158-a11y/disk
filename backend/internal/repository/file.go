package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"disk/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrFileFolderNotFound  = errors.New("文件夹不存在")
	ErrFileAlreadyExists   = errors.New("同名文件已存在")
	ErrFileNotFound        = errors.New("文件不存在")
	ErrQuotaExceeded       = errors.New("存储额度不足")
	ErrFileVersionConflict = errors.New("文件版本冲突")
	ErrFileVersionNotFound = errors.New("文件版本不存在")
)

type FilesRepository struct {
	db *gorm.DB
}

const retainedVersionsPerFile = 50

// NewFilesRepository 使用指定数据库连接创建文件仓储。
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
		//1.用户是否存在
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "used_quota", "quota_limit").
			Where("id = ?", userID).
			First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrUserNotFound
		case err != nil:
			return fmt.Errorf("find user %d for upload: %w", userID, err)
		}

		//2.文件大小是否合理
		if size < 0 || user.UsedQuota > user.QuotaLimit || size > user.QuotaLimit-user.UsedQuota {
			return ErrQuotaExceeded
		}

		//3.用户名下存在该目录吗？？
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

		//4.sha256对应的物理文件是否已经存在？？
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
		if err := tx.Create(&model.FileVersion{
			FileID: logical.ID, PhysicalFileID: physical.ID, Source: "upload", CreatedAt: logical.CreatedAt,
		}).Error; err != nil {
			return fmt.Errorf("create initial version for file %d: %w", logical.ID, err)
		}

		quotaUpdate := tx.Model(&model.User{}).
			Where("id = ? AND used_quota + ? <= quota_limit", userID, size).
			UpdateColumn("used_quota", gorm.Expr("used_quota + ?", size))
		if quotaUpdate.Error != nil {
			return fmt.Errorf("increase used quota for user %d: %w", userID, quotaUpdate.Error)
		}
		if quotaUpdate.RowsAffected == 0 {
			return ErrQuotaExceeded
		}

		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}

	logical.PhysicalFile = physical
	return &logical, &physical, physicalCreated, nil
}

// ReplaceFileContent 在版本前置条件成立时切换逻辑文件的物理内容引用。
// 返回值 physicalCreated 表示候选 storageKey 是否被数据库采用；旧物理对象由孤儿清理任务回收。
func (r *FilesRepository) ReplaceFileContent(
	ctx context.Context,
	userID uint64,
	fileID uint64,
	expectedSHA256 string,
	sha256Value string,
	size int64,
	storageKey string,
	createVersion bool,
) (*model.File, *model.PhysicalFile, bool, error) {
	var logical model.File
	var physical model.PhysicalFile
	physicalCreated := false

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "used_quota", "quota_limit").
			Where("id = ?", userID).
			First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrUserNotFound
		case err != nil:
			return fmt.Errorf("find user %d for content update: %w", userID, err)
		}

		err = tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", fileID, userID).
			First(&logical).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileNotFound
		case err != nil:
			return fmt.Errorf("find file %d for content update: %w", fileID, err)
		}

		var current model.PhysicalFile
		err = tx.
			Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id = ?", logical.PhysicalFileID).
			First(&current).Error
		if err != nil {
			return fmt.Errorf("find current physical file %d: %w", logical.PhysicalFileID, err)
		}
		if current.SHA256 != expectedSHA256 {
			return ErrFileVersionConflict
		}
		if current.SHA256 == sha256Value {
			physical = current
			logical.PhysicalFile = current
			if createVersion {
				return createEditCheckpoint(tx, fileID, current.ID, time.Now().UTC())
			}
			return nil
		}

		delta := size - current.Size
		if size < 0 || user.UsedQuota > user.QuotaLimit || (delta > 0 && delta > user.QuotaLimit-user.UsedQuota) {
			return ErrQuotaExceeded
		}

		physical = model.PhysicalFile{SHA256: sha256Value, Size: size, StorageKey: storageKey}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&physical)
		if result.Error != nil {
			return fmt.Errorf("create replacement physical file: %w", result.Error)
		}
		physicalCreated = result.RowsAffected == 1
		if !physicalCreated {
			if err := tx.
				Clauses(clause.Locking{Strength: "SHARE"}).
				Where("sha256 = ?", sha256Value).
				First(&physical).Error; err != nil {
				return fmt.Errorf("find replacement physical file by sha256: %w", err)
			}
		}

		updatedAt := time.Now().UTC()
		update := tx.Model(&model.File{}).
			Where("id = ? AND user_id = ? AND physical_file_id = ?", fileID, userID, current.ID).
			Updates(map[string]any{"physical_file_id": physical.ID, "updated_at": updatedAt})
		if update.Error != nil {
			return fmt.Errorf("replace content for file %d: %w", fileID, update.Error)
		}
		if update.RowsAffected == 0 {
			return ErrFileVersionConflict
		}
		if createVersion {
			if err := createEditCheckpoint(tx, fileID, physical.ID, updatedAt); err != nil {
				return err
			}
		}

		if delta != 0 {
			nextUsedQuota := user.UsedQuota + delta
			if nextUsedQuota < 0 {
				nextUsedQuota = 0
			}
			quotaUpdate := tx.Model(&model.User{}).
				Where("id = ?", userID).
				UpdateColumn("used_quota", nextUsedQuota)
			if quotaUpdate.Error != nil {
				return fmt.Errorf("adjust used quota for user %d: %w", userID, quotaUpdate.Error)
			}
			if quotaUpdate.RowsAffected == 0 {
				return ErrUserNotFound
			}
		}

		logical.PhysicalFileID = physical.ID
		logical.PhysicalFile = physical
		logical.UpdatedAt = updatedAt
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	return &logical, &physical, physicalCreated, nil
}

// createEditCheckpoint runs under the owning file's transaction lock.
// Autosaves never call it; repeated manual saves of the same snapshot are idempotent.
func createEditCheckpoint(tx *gorm.DB, fileID, physicalID uint64, createdAt time.Time) error {
	var latest model.FileVersion
	err := tx.Where("file_id = ?", fileID).Order("created_at DESC, id DESC").First(&latest).Error
	if err == nil && latest.PhysicalFileID == physicalID {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := tx.Create(&model.FileVersion{FileID: fileID, PhysicalFileID: physicalID, Source: "edit", CreatedAt: createdAt}).Error; err != nil {
		return fmt.Errorf("create manual checkpoint for file %d: %w", fileID, err)
	}
	return pruneFileVersions(tx, fileID, retainedVersionsPerFile)
}

// ListFileVersions 返回文件的历史快照，标注与当前内容一致的最新快照。
func (r *FilesRepository) ListFileVersions(ctx context.Context, userID uint64, fileID uint64) ([]model.FileVersion, error) {
	var file model.File
	err := r.db.WithContext(ctx).Select("id", "physical_file_id").Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrFileNotFound
	case err != nil:
		return nil, fmt.Errorf("find file %d for version listing: %w", fileID, err)
	}

	versions := make([]model.FileVersion, 0)
	if err := r.db.WithContext(ctx).
		Preload("PhysicalFile").
		Where("file_id = ?", fileID).
		Order("created_at DESC, id DESC").
		Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("list versions for file %d: %w", fileID, err)
	}
	for index := range versions {
		if versions[index].PhysicalFileID == file.PhysicalFileID {
			versions[index].Current = true
			break
		}
	}
	return versions, nil
}

// FindFileVersionForDownload 查询属于当前用户文件的指定历史版本。
func (r *FilesRepository) FindFileVersionForDownload(ctx context.Context, userID uint64, fileID uint64, versionID uint64) (*model.File, *model.FileVersion, error) {
	var file model.File
	err := r.db.WithContext(ctx).Select("id", "name").Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, nil, ErrFileNotFound
	case err != nil:
		return nil, nil, fmt.Errorf("find file %d for version download: %w", fileID, err)
	}
	var version model.FileVersion
	err = r.db.WithContext(ctx).Preload("PhysicalFile").Where("id = ? AND file_id = ?", versionID, fileID).First(&version).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, nil, ErrFileVersionNotFound
	case err != nil:
		return nil, nil, fmt.Errorf("find version %d for file %d: %w", versionID, fileID, err)
	}
	return &file, &version, nil
}

// RestoreFileVersion 使用强版本前置条件恢复历史内容，并把恢复结果记录成一个新版本。
func (r *FilesRepository) RestoreFileVersion(ctx context.Context, userID uint64, fileID uint64, versionID uint64, expectedSHA256 string) (*model.File, *model.FileVersion, error) {
	var logical model.File
	var restored model.FileVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "used_quota", "quota_limit").Where("id = ?", userID).First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrUserNotFound
		case err != nil:
			return fmt.Errorf("find user %d for version restore: %w", userID, err)
		}

		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", fileID, userID).First(&logical).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileNotFound
		case err != nil:
			return fmt.Errorf("find file %d for version restore: %w", fileID, err)
		}

		var current model.PhysicalFile
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&current, logical.PhysicalFileID).Error; err != nil {
			return fmt.Errorf("find current physical file %d: %w", logical.PhysicalFileID, err)
		}
		if current.SHA256 != expectedSHA256 {
			return ErrFileVersionConflict
		}

		var target model.FileVersion
		err = tx.Clauses(clause.Locking{Strength: "SHARE"}).Preload("PhysicalFile").Where("id = ? AND file_id = ?", versionID, fileID).First(&target).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileVersionNotFound
		case err != nil:
			return fmt.Errorf("find version %d for restore: %w", versionID, err)
		}
		if target.PhysicalFileID == current.ID {
			logical.PhysicalFile = current
			restored = target
			return nil
		}

		delta := target.PhysicalFile.Size - current.Size
		if user.UsedQuota > user.QuotaLimit || (delta > 0 && delta > user.QuotaLimit-user.UsedQuota) {
			return ErrQuotaExceeded
		}
		updatedAt := time.Now().UTC()
		result := tx.Model(&model.File{}).Where("id = ? AND user_id = ? AND physical_file_id = ?", fileID, userID, current.ID).
			Updates(map[string]any{"physical_file_id": target.PhysicalFileID, "updated_at": updatedAt})
		if result.Error != nil {
			return fmt.Errorf("restore version %d for file %d: %w", versionID, fileID, result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrFileVersionConflict
		}
		if delta != 0 {
			nextUsedQuota := user.UsedQuota + delta
			if nextUsedQuota < 0 {
				nextUsedQuota = 0
			}
			if result := tx.Model(&model.User{}).Where("id = ?", userID).UpdateColumn("used_quota", nextUsedQuota); result.Error != nil || result.RowsAffected == 0 {
				if result.Error != nil {
					return fmt.Errorf("adjust used quota for version restore: %w", result.Error)
				}
				return ErrUserNotFound
			}
		}
		restored = model.FileVersion{FileID: fileID, PhysicalFileID: target.PhysicalFileID, Source: "restore", RestoredFromVersionID: &target.ID, CreatedAt: updatedAt}
		if err := tx.Create(&restored).Error; err != nil {
			return fmt.Errorf("record restored version for file %d: %w", fileID, err)
		}
		if err := pruneFileVersions(tx, fileID, retainedVersionsPerFile); err != nil {
			return err
		}
		restored.PhysicalFile = target.PhysicalFile
		logical.PhysicalFileID = target.PhysicalFileID
		logical.PhysicalFile = target.PhysicalFile
		logical.UpdatedAt = updatedAt
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &logical, &restored, nil
}

func pruneFileVersions(tx *gorm.DB, fileID uint64, keep int) error {
	if keep <= 0 {
		return nil
	}
	var versionIDs []uint64
	if err := tx.Model(&model.FileVersion{}).
		Where("file_id = ?", fileID).
		Order("created_at DESC, id DESC").
		Pluck("id", &versionIDs).Error; err != nil {
		return fmt.Errorf("find stale versions for file %d: %w", fileID, err)
	}
	if len(versionIDs) <= keep {
		return nil
	}
	staleIDs := versionIDs[keep:]
	if err := tx.Where("id IN ?", staleIDs).Delete(&model.FileVersion{}).Error; err != nil {
		return fmt.Errorf("prune stale versions for file %d: %w", fileID, err)
	}
	return nil
}

// ListFiles 查询当前用户指定目录中的直接文件，不包含子目录中的文件。
func (r *FilesRepository) ListFiles(ctx context.Context, userID uint64, folderID uint64) ([]model.File, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Select("id").
		Where("id = ? AND user_id = ?", folderID, userID).
		First(&folder).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrFileFolderNotFound
	case err != nil:
		return nil, fmt.Errorf("find folder %d for file listing: %w", folderID, err)
	}

	files := make([]model.File, 0)
	if err := r.db.WithContext(ctx).
		Preload("PhysicalFile").
		Where("user_id = ? AND folder_id = ?", userID, folderID).
		Order("created_at ASC, id ASC").
		Find(&files).Error; err != nil {
		return nil, fmt.Errorf("list files in folder %d: %w", folderID, err)
	}

	return files, nil
}

// FindFileForDownload 查询当前用户可下载的逻辑文件及其物理存储信息。
func (r *FilesRepository) FindFileForDownload(ctx context.Context, userID uint64, fileID uint64) (*model.File, error) {
	var file model.File
	err := r.db.WithContext(ctx).
		Preload("PhysicalFile").
		Where("id = ? AND user_id = ?", fileID, userID).
		First(&file).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrFileNotFound
	case err != nil:
		return nil, fmt.Errorf("find file %d for download: %w", fileID, err)
	default:
		return &file, nil
	}
}

// RenameFile 重命名当前用户拥有的逻辑文件。
func (r *FilesRepository) RenameFile(ctx context.Context, userID uint64, fileID uint64, name string) error {
	result := r.db.WithContext(ctx).
		Model(&model.File{}).
		Where("id = ? AND user_id = ?", fileID, userID).
		Update("name", name)
	if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		return ErrFileAlreadyExists
	}
	if result.Error != nil {
		return fmt.Errorf("rename file %d: %w", fileID, result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).
			Model(&model.File{}).
			Where("id = ? AND user_id = ?", fileID, userID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check file %d after rename: %w", fileID, err)
		}
		if count == 0 {
			return ErrFileNotFound
		}
	}
	return nil
}

// MoveFile 将当前用户拥有的逻辑文件移动到新的目录。
func (r *FilesRepository) MoveFile(ctx context.Context, userID uint64, fileID uint64, folderID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return fmt.Errorf("lock user %d for file move: %w", userID, err)
		}

		var file model.File
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "folder_id", "name").
			Where("id = ? AND user_id = ?", fileID, userID).
			First(&file).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileNotFound
		case err != nil:
			return fmt.Errorf("find file %d for move: %w", fileID, err)
		}

		var folder model.Folder
		err = tx.
			Clauses(clause.Locking{Strength: "SHARE"}).
			Select("id").
			Where("id = ? AND user_id = ?", folderID, userID).
			First(&folder).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileFolderNotFound
		case err != nil:
			return fmt.Errorf("find target folder %d for file move: %w", folderID, err)
		}

		if file.FolderID == folder.ID {
			return nil
		}

		result := tx.Model(&model.File{}).
			Where("id = ? AND user_id = ?", fileID, userID).
			Update("folder_id", folder.ID)
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return ErrFileAlreadyExists
		}
		if result.Error != nil {
			return fmt.Errorf("move file %d to folder %d: %w", fileID, folder.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrFileNotFound
		}

		return nil
	})
}

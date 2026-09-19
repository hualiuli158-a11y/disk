package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"disk/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTrashNotFound        = errors.New("回收站条目不存在")
	ErrTrashRestoreConflict = errors.New("恢复位置存在同名资源")
)

type TrashRepository struct {
	db *gorm.DB
}

// NewTrashRepository 使用指定数据库连接创建回收站仓储。
func NewTrashRepository(db *gorm.DB) *TrashRepository {
	return &TrashRepository{db: db}
}

// TrashFile 为文件创建回收站条目，并将逻辑文件标记为已删除。
func (r *TrashRepository) TrashFile(ctx context.Context, userID uint64, fileID uint64, trashedAt time.Time, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockTrashUser(tx, userID); err != nil {
			return err
		}
		var file model.File
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", fileID, userID).
			First(&file).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFileNotFound
		case err != nil:
			return fmt.Errorf("find file %d for trash: %w", fileID, err)
		}

		parentID := file.FolderID
		entry := model.TrashEntry{
			UserID: userID, ResourceType: model.TrashResourceFile, ResourceID: file.ID,
			OriginalParentID: &parentID, Name: file.Name, TrashedAt: trashedAt, ExpiresAt: expiresAt,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return fmt.Errorf("create file trash entry: %w", err)
		}
		key := fmt.Sprintf("%d:%d", entry.ID, file.ID)
		result := tx.Model(&model.File{}).
			Where("id = ? AND user_id = ?", file.ID, userID).
			Updates(map[string]any{"deleted_at": trashedAt, "trash_id": entry.ID, "deletion_key": key})
		if result.Error != nil {
			return fmt.Errorf("move file %d to trash: %w", file.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrFileNotFound
		}
		return nil
	})
}

// TrashFolder 为文件夹创建回收站条目，并递归标记目录树及其中的文件为已删除。
func (r *TrashRepository) TrashFolder(ctx context.Context, userID uint64, folderID uint64, trashedAt time.Time, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockTrashUser(tx, userID); err != nil {
			return err
		}
		var root model.Folder
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", folderID, userID).
			First(&root).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFolderNotFound
		case err != nil:
			return fmt.Errorf("find folder %d for trash: %w", folderID, err)
		}
		if root.ParentID == nil {
			return ErrCannotDeleteRoot
		}

		folderIDs, err := activeFolderTreeIDs(tx, userID, folderID)
		if err != nil {
			return err
		}
		var files []model.File
		if err := tx.Select("id").Where("user_id = ? AND folder_id IN ?", userID, folderIDs).Find(&files).Error; err != nil {
			return fmt.Errorf("find files in folder tree %d: %w", folderID, err)
		}

		entry := model.TrashEntry{
			UserID: userID, ResourceType: model.TrashResourceFolder, ResourceID: root.ID,
			OriginalParentID: root.ParentID, Name: root.Name, TrashedAt: trashedAt, ExpiresAt: expiresAt,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return fmt.Errorf("create folder trash entry: %w", err)
		}
		keyPrefix := strconv.FormatUint(entry.ID, 10) + ":"
		if len(files) > 0 {
			fileIDs := make([]uint64, 0, len(files))
			for _, file := range files {
				fileIDs = append(fileIDs, file.ID)
			}
			if err := tx.Model(&model.File{}).Where("id IN ?", fileIDs).Updates(map[string]any{
				"deleted_at":   trashedAt,
				"trash_id":     entry.ID,
				"deletion_key": gorm.Expr("CONCAT(?, id)", keyPrefix),
			}).Error; err != nil {
				return fmt.Errorf("move files in folder tree %d to trash: %w", folderID, err)
			}
		}
		if err := tx.Model(&model.Folder{}).Where("id IN ?", folderIDs).Updates(map[string]any{
			"deleted_at":   trashedAt,
			"trash_id":     entry.ID,
			"deletion_key": gorm.Expr("CONCAT(?, id)", keyPrefix),
		}).Error; err != nil {
			return fmt.Errorf("move folder tree %d to trash: %w", folderID, err)
		}
		return nil
	})
}

// ListTrash 分页查询当前用户的回收站条目，并返回条目总数。
func (r *TrashRepository) ListTrash(ctx context.Context, userID uint64, offset int, limit int) ([]model.TrashEntry, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.TrashEntry{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count trash entries: %w", err)
	}
	entries := make([]model.TrashEntry, 0)
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("trashed_at DESC, id DESC").
		Limit(limit).Offset(offset).
		Find(&entries).Error; err != nil {
		return nil, 0, fmt.Errorf("list trash entries: %w", err)
	}
	return entries, total, nil
}

// ListExpiredTrashEntries 按过期时间查询待清理的回收站条目。
func (r *TrashRepository) ListExpiredTrashEntries(ctx context.Context, now time.Time, limit int) ([]model.TrashEntry, error) {
	entries := make([]model.TrashEntry, 0)
	if err := r.db.WithContext(ctx).
		Where("expires_at <= ?", now).
		Order("id ASC").Limit(limit).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("list expired trash entries: %w", err)
	}
	return entries, nil
}

// RestoreTrash 将回收站中的文件或文件夹恢复到原目录；原目录不存在时恢复到根目录。
func (r *TrashRepository) RestoreTrash(ctx context.Context, userID uint64, trashID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockTrashUser(tx, userID); err != nil {
			return err
		}
		entry, err := findTrashEntryForUpdate(tx, userID, trashID)
		if err != nil {
			return err
		}
		targetID, err := restoreTargetFolderID(tx, userID, entry.OriginalParentID)
		if err != nil {
			return err
		}

		switch entry.ResourceType {
		case model.TrashResourceFile:
			var file model.File
			if err := tx.Unscoped().Where("id = ? AND user_id = ? AND trash_id = ?", entry.ResourceID, userID, entry.ID).First(&file).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTrashNotFound
				}
				return fmt.Errorf("find trashed file %d: %w", entry.ResourceID, err)
			}
			exists, err := activeFileNameExists(tx, userID, targetID, file.Name)
			if err != nil {
				return err
			}
			if exists {
				return ErrTrashRestoreConflict
			}
			if err := tx.Unscoped().Model(&model.File{}).Where("id = ?", file.ID).UpdateColumns(map[string]any{
				"folder_id": targetID, "deleted_at": nil, "trash_id": nil, "deletion_key": "",
			}).Error; err != nil {
				return fmt.Errorf("restore file %d: %w", file.ID, err)
			}
		case model.TrashResourceFolder:
			var folder model.Folder
			if err := tx.Unscoped().Where("id = ? AND user_id = ? AND trash_id = ?", entry.ResourceID, userID, entry.ID).First(&folder).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTrashNotFound
				}
				return fmt.Errorf("find trashed folder %d: %w", entry.ResourceID, err)
			}
			exists, err := activeFolderNameExists(tx, userID, targetID, folder.Name)
			if err != nil {
				return err
			}
			if exists {
				return ErrTrashRestoreConflict
			}
			if err := tx.Unscoped().Model(&model.Folder{}).Where("id = ?", folder.ID).UpdateColumn("parent_id", targetID).Error; err != nil {
				return fmt.Errorf("restore folder parent %d: %w", folder.ID, err)
			}
			if err := tx.Unscoped().Model(&model.File{}).Where("trash_id = ?", entry.ID).UpdateColumns(map[string]any{
				"deleted_at": nil, "trash_id": nil, "deletion_key": "",
			}).Error; err != nil {
				return fmt.Errorf("restore files for trash %d: %w", entry.ID, err)
			}
			if err := tx.Unscoped().Model(&model.Folder{}).Where("trash_id = ?", entry.ID).UpdateColumns(map[string]any{
				"deleted_at": nil, "trash_id": nil, "deletion_key": "",
			}).Error; err != nil {
				return fmt.Errorf("restore folders for trash %d: %w", entry.ID, err)
			}
		default:
			return ErrTrashNotFound
		}

		if err := tx.Delete(entry).Error; err != nil {
			return fmt.Errorf("delete restored trash entry %d: %w", entry.ID, err)
		}
		return nil
	})
}

// PermanentlyDeleteTrash 永久删除回收站资源，并返回不再被引用、需要从对象存储删除的存储键。
func (r *TrashRepository) PermanentlyDeleteTrash(ctx context.Context, userID uint64, trashID uint64) ([]string, error) {
	storageKeys := make([]string, 0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockTrashUser(tx, userID); err != nil {
			return err
		}
		entry, err := findTrashEntryForUpdate(tx, userID, trashID)
		if err != nil {
			return err
		}

		switch entry.ResourceType {
		case model.TrashResourceFile:
			var file model.File
			if err := tx.Unscoped().Where("id = ? AND user_id = ?", entry.ResourceID, userID).First(&file).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTrashNotFound
				}
				return err
			}
			keys, err := permanentlyDeleteFiles(tx, userID, []model.File{file})
			if err != nil {
				return err
			}
			storageKeys = append(storageKeys, keys...)
		case model.TrashResourceFolder:
			nodes, err := allFolderTreeNodes(tx, userID, entry.ResourceID)
			if err != nil {
				return err
			}
			folderIDs := make([]uint64, 0, len(nodes))
			for _, node := range nodes {
				folderIDs = append(folderIDs, node.ID)
			}
			var files []model.File
			if err := tx.Unscoped().Where("user_id = ? AND folder_id IN ?", userID, folderIDs).Find(&files).Error; err != nil {
				return fmt.Errorf("find files for permanent folder delete: %w", err)
			}
			keys, err := permanentlyDeleteFiles(tx, userID, files)
			if err != nil {
				return err
			}
			storageKeys = append(storageKeys, keys...)

			fileIDs := make([]uint64, 0, len(files))
			for _, file := range files {
				fileIDs = append(fileIDs, file.ID)
			}
			query := tx.Where("user_id = ? AND resource_type = ? AND resource_id IN ?", userID, model.TrashResourceFolder, folderIDs)
			if len(fileIDs) > 0 {
				query = query.Or("user_id = ? AND resource_type = ? AND resource_id IN ?", userID, model.TrashResourceFile, fileIDs)
			}
			if err := query.Delete(&model.TrashEntry{}).Error; err != nil {
				return fmt.Errorf("delete nested trash entries: %w", err)
			}

			sort.Slice(nodes, func(i, j int) bool { return nodes[i].Depth > nodes[j].Depth })
			for _, node := range nodes {
				if err := tx.Unscoped().Where("id = ?", node.ID).Delete(&model.Folder{}).Error; err != nil {
					return fmt.Errorf("permanently delete folder %d: %w", node.ID, err)
				}
			}
		default:
			return ErrTrashNotFound
		}

		if err := tx.Where("id = ?", entry.ID).Delete(&model.TrashEntry{}).Error; err != nil {
			return fmt.Errorf("delete trash entry %d: %w", entry.ID, err)
		}
		return nil
	})
	return storageKeys, err
}

type folderTreeNode struct {
	ID    uint64
	Depth int
}

// activeFolderTreeIDs 查询以指定目录为根的未删除目录树 ID。
func activeFolderTreeIDs(tx *gorm.DB, userID uint64, rootID uint64) ([]uint64, error) {
	var ids []uint64
	query := `WITH RECURSIVE tree AS (
		SELECT id FROM folders WHERE id = ? AND user_id = ? AND deleted_at IS NULL
		UNION ALL
		SELECT f.id FROM folders AS f JOIN tree ON f.parent_id = tree.id
		WHERE f.user_id = ? AND f.deleted_at IS NULL
	) SELECT id FROM tree`
	if err := tx.Raw(query, rootID, userID, userID).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("find active folder tree %d: %w", rootID, err)
	}
	return ids, nil
}

// allFolderTreeNodes 查询完整目录树的节点及深度，包含已软删除的目录。
func allFolderTreeNodes(tx *gorm.DB, userID uint64, rootID uint64) ([]folderTreeNode, error) {
	nodes := make([]folderTreeNode, 0)
	query := `WITH RECURSIVE tree AS (
		SELECT id, 0 AS depth FROM folders WHERE id = ? AND user_id = ?
		UNION ALL
		SELECT f.id, tree.depth + 1 FROM folders AS f JOIN tree ON f.parent_id = tree.id
		WHERE f.user_id = ?
	) SELECT id, depth FROM tree`
	if err := tx.Raw(query, rootID, userID, userID).Scan(&nodes).Error; err != nil {
		return nil, fmt.Errorf("find complete folder tree %d: %w", rootID, err)
	}
	if len(nodes) == 0 {
		return nil, ErrTrashNotFound
	}
	return nodes, nil
}

// lockTrashUser 锁定用户记录，使同一用户的回收站写操作串行执行。
func lockTrashUser(tx *gorm.DB, userID uint64) error {
	var user model.User
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("lock user %d for trash operation: %w", userID, err)
	}
	return nil
}

// findTrashEntryForUpdate 查询并锁定当前用户的指定回收站条目。
func findTrashEntryForUpdate(tx *gorm.DB, userID uint64, trashID uint64) (*model.TrashEntry, error) {
	var entry model.TrashEntry
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", trashID, userID).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTrashNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find trash entry %d: %w", trashID, err)
	}
	return &entry, nil
}

// restoreTargetFolderID 确定资源恢复目录，原父目录不存在时回退到用户根目录。
func restoreTargetFolderID(tx *gorm.DB, userID uint64, originalParentID *uint64) (uint64, error) {
	if originalParentID != nil {
		var folder model.Folder
		if err := tx.Select("id").Where("id = ? AND user_id = ?", *originalParentID, userID).First(&folder).Error; err == nil {
			return folder.ID, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
	}
	var root model.Folder
	if err := tx.Select("id").Where("user_id = ? AND parent_id IS NULL", userID).First(&root).Error; err != nil {
		return 0, fmt.Errorf("find root folder for restore: %w", err)
	}
	return root.ID, nil
}

// activeFileNameExists 判断目标目录中是否存在同名的未删除文件。
func activeFileNameExists(tx *gorm.DB, userID uint64, folderID uint64, name string) (bool, error) {
	var count int64
	if err := tx.Model(&model.File{}).Where("user_id = ? AND folder_id = ? AND name = ?", userID, folderID, name).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check file restore conflict: %w", err)
	}
	return count > 0, nil
}

// activeFolderNameExists 判断目标目录中是否存在同名的未删除文件夹。
func activeFolderNameExists(tx *gorm.DB, userID uint64, parentID uint64, name string) (bool, error) {
	var count int64
	if err := tx.Model(&model.Folder{}).Where("user_id = ? AND parent_id = ? AND name = ?", userID, parentID, name).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check folder restore conflict: %w", err)
	}
	return count > 0, nil
}

// permanentlyDeleteFiles 永久删除逻辑文件、回收用户额度，并清理无引用的物理文件记录。
// 返回值为需要由上层从对象存储中删除的存储键。
func permanentlyDeleteFiles(tx *gorm.DB, userID uint64, files []model.File) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	fileIDs := make([]uint64, 0, len(files))
	physicalIDs := make(map[uint64]struct{})
	var quota int64
	for i := range files {
		var physical model.PhysicalFile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", files[i].PhysicalFileID).First(&physical).Error; err != nil {
			return nil, fmt.Errorf("find physical file %d for permanent delete: %w", files[i].PhysicalFileID, err)
		}
		fileIDs = append(fileIDs, files[i].ID)
		physicalIDs[physical.ID] = struct{}{}
		quota += physical.Size
	}
	if err := tx.Unscoped().Where("id IN ?", fileIDs).Delete(&model.File{}).Error; err != nil {
		return nil, fmt.Errorf("permanently delete logical files: %w", err)
	}
	if err := tx.Model(&model.User{}).Where("id = ?", userID).
		UpdateColumn("used_quota", gorm.Expr("GREATEST(used_quota - ?, 0)", quota)).Error; err != nil {
		return nil, fmt.Errorf("decrease quota after permanent delete: %w", err)
	}

	keys := make([]string, 0)
	for physicalID := range physicalIDs {
		var references int64
		if err := tx.Unscoped().Model(&model.File{}).Where("physical_file_id = ?", physicalID).Count(&references).Error; err != nil {
			return nil, err
		}
		if references > 0 {
			continue
		}
		var physical model.PhysicalFile
		if err := tx.Where("id = ?", physicalID).First(&physical).Error; err != nil {
			return nil, err
		}
		if err := tx.Delete(&physical).Error; err != nil {
			return nil, err
		}
		keys = append(keys, physical.StorageKey)
	}
	return keys, nil
}

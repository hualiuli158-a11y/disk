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
	ErrParentNotFound      = errors.New("父目录不存在")
	ErrFolderAlreadyExists = errors.New("同名文件夹已存在")
	ErrRootFolderNotFound  = errors.New("根目录不存在")
	ErrFolderNotFound      = errors.New("文件夹不存在")
	ErrCannotDeleteRoot    = errors.New("根目录不能删除")
	ErrCannotMoveRoot      = errors.New("根目录不能移动")
	ErrInvalidFolderMove   = errors.New("不能将文件夹移动到自身或其子目录")
)

// FoldersRepository 负责文件夹数据的持久化操作。
type FoldersRepository struct {
	db *gorm.DB
}

// NewFoldersRepository 使用指定数据库连接创建文件夹仓储。
func NewFoldersRepository(db *gorm.DB) *FoldersRepository {
	return &FoldersRepository{db: db}
}

// CreateChildFolder 在当前用户的指定父目录下创建子目录。
func (r *FoldersRepository) CreateChildFolder(ctx context.Context, userID uint64, parentID uint64, name string) (*model.Folder, error) {
	var child model.Folder

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return fmt.Errorf("lock user %d for folder creation: %w", userID, err)
		}

		var parent model.Folder

		err := tx.
			Select("id").
			Where("id = ? AND user_id = ?", parentID, userID).
			First(&parent).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrParentNotFound
		case err != nil:
			return fmt.Errorf("find parent folder: %w", err)
		}

		child = model.Folder{
			UserID:   userID,
			ParentID: &parent.ID,
			Name:     name,
		}

		if err := tx.Create(&child).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrFolderAlreadyExists
			}
			return fmt.Errorf("create child folder: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &child, nil
}

// ListRootChildren 查询用户根目录及其直接子目录，不递归加载更深层级。
func (r *FoldersRepository) ListRootChildren(ctx context.Context, userID uint64) (*model.Folder, error) {
	var root model.Folder
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND parent_id IS NULL", userID).
		First(&root).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrRootFolderNotFound
	case err != nil:
		return nil, fmt.Errorf("find root folder: %w", err)
	}

	children := make([]model.Folder, 0)
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND parent_id = ?", userID, root.ID).
		Order("created_at ASC, id ASC").
		Find(&children).Error; err != nil {
		return nil, fmt.Errorf("find root children: %w", err)
	}

	root.Children = children
	return &root, nil
}

// RenameFolder 重命名当前用户拥有的非根目录。
func (r *FoldersRepository) RenameFolder(ctx context.Context, userID uint64, folderID uint64, name string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Where("id = ? AND user_id = ? AND parent_id IS NOT NULL", folderID, userID).
		Update("name", name)

	if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		return ErrFolderAlreadyExists
	}
	if result.Error != nil {
		return fmt.Errorf("rename folder %d: %w", folderID, result.Error)
	}
	if result.RowsAffected == 0 {
		// MySQL 在名称没有发生变化时可能报告 0 行受影响；此时再次确认目录是否存在，
		// 使重复提交相同名称保持幂等。
		var count int64
		if err := r.db.WithContext(ctx).
			Model(&model.Folder{}).
			Where("id = ? AND user_id = ? AND parent_id IS NOT NULL", folderID, userID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check folder %d after rename: %w", folderID, err)
		}
		if count == 0 {
			return ErrFolderNotFound
		}
	}

	return nil
}

// ListChildren 查询指定目录及其直接子目录，不递归加载更深层级。
func (r *FoldersRepository) ListChildren(ctx context.Context, userID uint64, folderID uint64) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", folderID, userID).
		First(&folder).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrFolderNotFound
	case err != nil:
		return nil, fmt.Errorf("find folder %d: %w", folderID, err)
	}

	children := make([]model.Folder, 0)
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND parent_id = ?", userID, folderID).
		Order("created_at ASC, id ASC").
		Find(&children).Error; err != nil {
		return nil, fmt.Errorf("find children of folder %d: %w", folderID, err)
	}

	folder.Children = children
	return &folder, nil
}

// MoveFolder 将当前用户拥有的非根目录移动到新的父目录。
func (r *FoldersRepository) MoveFolder(ctx context.Context, userID uint64, folderID uint64, newParentID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 同一用户的移动操作串行执行，避免两个并发移动共同制造目录环。
		var user model.User
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", userID).
			First(&user).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFolderNotFound
		case err != nil:
			return fmt.Errorf("lock user %d for folder move: %w", userID, err)
		}

		var folder model.Folder
		err = tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "parent_id", "name").
			Where("id = ? AND user_id = ?", folderID, userID).
			First(&folder).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFolderNotFound
		case err != nil:
			return fmt.Errorf("find folder %d for move: %w", folderID, err)
		}

		if folder.ParentID == nil {
			return ErrCannotMoveRoot
		}
		if folder.ID == newParentID {
			return ErrInvalidFolderMove
		}

		var target model.Folder
		err = tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "parent_id").
			Where("id = ? AND user_id = ?", newParentID, userID).
			First(&target).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrFolderNotFound
		case err != nil:
			return fmt.Errorf("find target folder %d for move: %w", newParentID, err)
		}

		// 已经位于目标目录下时保持幂等。
		if folder.ParentID != nil && *folder.ParentID == target.ID {
			return nil
		}

		// 沿目标目录向根目录回溯；遇到源目录说明目标是源目录的后代。
		current := target
		visited := make(map[uint64]struct{})
		for {
			if current.ID == folder.ID {
				return ErrInvalidFolderMove
			}
			if _, exists := visited[current.ID]; exists {
				return ErrInvalidFolderMove
			}
			visited[current.ID] = struct{}{}

			if current.ParentID == nil {
				break
			}

			var parent model.Folder
			err = tx.
				Select("id", "parent_id").
				Where("id = ? AND user_id = ?", *current.ParentID, userID).
				First(&parent).Error
			if err != nil {
				return fmt.Errorf("walk ancestors from folder %d: %w", current.ID, err)
			}
			current = parent
		}

		result := tx.Model(&model.Folder{}).
			Where("id = ? AND user_id = ? AND parent_id IS NOT NULL", folderID, userID).
			Update("parent_id", target.ID)
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return ErrFolderAlreadyExists
		}
		if result.Error != nil {
			return fmt.Errorf("move folder %d to %d: %w", folderID, target.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrFolderNotFound
		}

		return nil
	})
}

// ListContents 分页查询指定目录中的直接子目录和文件，文件夹始终排在文件之前。
func (r *FoldersRepository) ListContents(
	ctx context.Context,
	userID uint64,
	folderID uint64,
	offset int,
	limit int,
	sortField string,
	sortOrder string,
) (*model.Folder, []model.ContentItem, int64, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", folderID, userID).
		First(&folder).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, nil, 0, ErrFolderNotFound
	case err != nil:
		return nil, nil, 0, fmt.Errorf("find folder %d for contents: %w", folderID, err)
	}

	sortColumns := map[string]string{
		"name":       "name",
		"size":       "size",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	column, ok := sortColumns[sortField]
	if !ok {
		return nil, nil, 0, fmt.Errorf("unsupported content sort field %q", sortField)
	}
	order := "ASC"
	if sortOrder == "desc" {
		order = "DESC"
	} else if sortOrder != "asc" {
		return nil, nil, 0, fmt.Errorf("unsupported content sort order %q", sortOrder)
	}

	baseQuery := `
		SELECT id, 'folder' AS kind, 0 AS kind_order, parent_id, name,
		       CAST(NULL AS SIGNED) AS size, NULL AS sha256, created_at, updated_at
		FROM folders
		WHERE user_id = ? AND parent_id = ? AND deleted_at IS NULL
		UNION ALL
		SELECT f.id, 'file' AS kind, 1 AS kind_order, f.folder_id AS parent_id, f.name,
		       p.size AS size, p.sha256 AS sha256, f.created_at, f.updated_at
		FROM files AS f
		JOIN physical_files AS p ON p.id = f.physical_file_id
		WHERE f.user_id = ? AND f.folder_id = ? AND f.deleted_at IS NULL`

	var total int64
	countSQL := "SELECT COUNT(*) FROM (" + baseQuery + ") AS contents"
	if err := r.db.WithContext(ctx).
		Raw(countSQL, userID, folderID, userID, folderID).
		Scan(&total).Error; err != nil {
		return nil, nil, 0, fmt.Errorf("count contents of folder %d: %w", folderID, err)
	}

	items := make([]model.ContentItem, 0)
	listSQL := "SELECT id, kind, parent_id, name, size, sha256, created_at, updated_at FROM (" + baseQuery +
		") AS contents ORDER BY kind_order ASC, " + column + " " + order + ", name ASC, id ASC LIMIT ? OFFSET ?"
	if err := r.db.WithContext(ctx).
		Raw(listSQL, userID, folderID, userID, folderID, limit, offset).
		Scan(&items).Error; err != nil {
		return nil, nil, 0, fmt.Errorf("list contents of folder %d: %w", folderID, err)
	}

	return &folder, items, total, nil
}

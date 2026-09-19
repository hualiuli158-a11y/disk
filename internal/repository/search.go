package repository

import (
	"context"
	"fmt"

	"disk/internal/model"

	"gorm.io/gorm"
)

type SearchRepository struct {
	db *gorm.DB
}

// NewSearchRepository 使用指定数据库连接创建搜索仓储。
func NewSearchRepository(db *gorm.DB) *SearchRepository {
	return &SearchRepository{db: db}
}

// Search 按名称关键字和资源类型分页搜索当前用户的文件夹与文件，并返回结果总数。
func (r *SearchRepository) Search(
	ctx context.Context,
	userID uint64,
	query string,
	kind string,
	offset int,
	limit int,
	sortField string,
	sortOrder string,
) ([]model.ContentItem, int64, error) {
	sortColumns := map[string]string{
		"name":       "name",
		"size":       "size",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}
	column, ok := sortColumns[sortField]
	if !ok {
		return nil, 0, fmt.Errorf("unsupported search sort field %q", sortField)
	}
	order := "ASC"
	if sortOrder == "desc" {
		order = "DESC"
	} else if sortOrder != "asc" {
		return nil, 0, fmt.Errorf("unsupported search sort order %q", sortOrder)
	}

	pattern := "%" + query + "%"
	folderQuery := `SELECT id, 'folder' AS kind, 0 AS kind_order, parent_id, name,
		CAST(NULL AS SIGNED) AS size, created_at, updated_at
		FROM folders WHERE user_id = ? AND parent_id IS NOT NULL AND deleted_at IS NULL AND name LIKE ?`
	fileQuery := `SELECT f.id, 'file' AS kind, 1 AS kind_order, f.folder_id AS parent_id, f.name,
		p.size AS size, f.created_at, f.updated_at
		FROM files AS f JOIN physical_files AS p ON p.id = f.physical_file_id
		WHERE f.user_id = ? AND f.deleted_at IS NULL AND f.name LIKE ?`

	var baseQuery string
	var args []any
	switch kind {
	case "folder":
		baseQuery = folderQuery
		args = []any{userID, pattern}
	case "file":
		baseQuery = fileQuery
		args = []any{userID, pattern}
	case "all":
		baseQuery = folderQuery + " UNION ALL " + fileQuery
		args = []any{userID, pattern, userID, pattern}
	default:
		return nil, 0, fmt.Errorf("unsupported search kind %q", kind)
	}

	var total int64
	if err := r.db.WithContext(ctx).
		Raw("SELECT COUNT(*) FROM ("+baseQuery+") AS results", args...).
		Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}

	items := make([]model.ContentItem, 0)
	listSQL := "SELECT id, kind, parent_id, name, size, created_at, updated_at FROM (" + baseQuery +
		") AS results ORDER BY kind_order ASC, " + column + " " + order + ", name ASC, id ASC LIMIT ? OFFSET ?"
	listArgs := append(append([]any{}, args...), limit, offset)
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("search files and folders: %w", err)
	}
	return items, total, nil
}

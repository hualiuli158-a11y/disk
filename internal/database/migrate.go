package database

import (
	"disk/internal/model"
	"fmt"

	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&model.User{},
		&model.Folder{},
		&model.File{},
		&model.PhysicalFile{},
		&model.TrashEntry{},
	)
	if err != nil {
		return fmt.Errorf("AutoMigrate err: %w", err)
	}

	// 旧版唯一索引不包含 deletion_key，会阻止软删除后创建同名资源。
	for _, legacy := range []struct {
		model any
		name  string
	}{
		{model: &model.Folder{}, name: "idx_user_parent_folder_name"},
		{model: &model.File{}, name: "idx_user_folder_file_name"},
	} {
		if db.Migrator().HasIndex(legacy.model, legacy.name) {
			if err := db.Migrator().DropIndex(legacy.model, legacy.name); err != nil {
				return fmt.Errorf("drop legacy index %s: %w", legacy.name, err)
			}
		}
	}
	return nil
}

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
	)
	if err != nil {
		return fmt.Errorf("AutoMigrate err: %w", err)
	}
	return nil
}

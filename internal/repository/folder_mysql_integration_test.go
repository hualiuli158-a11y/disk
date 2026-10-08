package repository

import (
	"context"
	"os"
	"testing"

	"disk/internal/database"
	"disk/internal/model"
	"disk/pkg/setting"
)

// This test is intentionally opt-in because it uses the developer's configured MySQL database.
// It only performs reads and exists to catch SQL-dialect and projection issues missed by unit tests.
func TestFolderContentsMySQLProjection(t *testing.T) {
	if os.Getenv("DISK_MYSQL_INTEGRATION") != "1" {
		t.Skip("set DISK_MYSQL_INTEGRATION=1 to run against configured MySQL")
	}
	config, err := setting.LoadDatabaseConfig()
	if err != nil {
		t.Fatalf("load database config: %v", err)
	}
	db, err := database.OpenMySQL(config)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	defer sqlDB.Close()

	var folder model.Folder
	if err := db.WithContext(context.Background()).Where("id = ?", 2).First(&folder).Error; err != nil {
		t.Fatalf("find fixture folder 2: %v", err)
	}

	folders := NewFoldersRepository(db)
	_, _, _, err = folders.ListContents(context.Background(), folder.UserID, folder.ID, 0, 100, "name", "asc")
	if err != nil {
		t.Fatalf("list folder contents: %v", err)
	}

	search := NewSearchRepository(db)
	_, _, err = search.Search(context.Background(), folder.UserID, "", "all", 0, 100, "name", "asc")
	if err != nil {
		t.Fatalf("search files and folders: %v", err)
	}
}

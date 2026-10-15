package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"disk/internal/model"
)

// This test checks the MySQL-specific UNION projection with data it creates itself.
func TestFolderContentsMySQLProjection(t *testing.T) {
	db := openRepositoryTestDB(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin test transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("rollback test transaction: %v", err)
		}
	})

	ctx := context.Background()
	user := &model.User{
		Username:   fmt.Sprintf("folder_repository_test_%d", time.Now().UnixNano()),
		Password:   "test-password-hash",
		QuotaLimit: model.DefaultQuotaLimit,
	}
	if err := NewUserRepository(tx).CreateRoot(ctx, user); err != nil {
		t.Fatalf("create fixture user: %v", err)
	}

	folders := NewFoldersRepository(tx)
	root, err := folders.ListRootChildren(ctx, user.ID)
	if err != nil {
		t.Fatalf("find root folder: %v", err)
	}
	child, err := folders.CreateChildFolder(ctx, user.ID, root.ID, "My documents")
	if err != nil {
		t.Fatalf("create child folder: %v", err)
	}

	parent, contents, total, err := folders.ListContents(ctx, user.ID, root.ID, 0, 100, "name", "asc")
	if err != nil {
		t.Fatalf("list folder contents: %v", err)
	}
	if parent == nil || parent.ID != root.ID || total != 1 || len(contents) != 1 ||
		contents[0].ID != child.ID || contents[0].Kind != "folder" ||
		contents[0].Name != child.Name || contents[0].ParentID == nil ||
		*contents[0].ParentID != root.ID {
		t.Fatalf("unexpected folder contents: parent=%+v, total=%d, items=%+v", parent, total, contents)
	}

	items, count, err := NewSearchRepository(tx).Search(ctx, user.ID, "My documents", "all", 0, 100, "name", "asc")
	if err != nil {
		t.Fatalf("search files and folders: %v", err)
	}
	if count != 1 || len(items) != 1 || items[0].ID != child.ID || items[0].Kind != "folder" {
		t.Fatalf("unexpected search result: count=%d, items=%+v", count, items)
	}
}

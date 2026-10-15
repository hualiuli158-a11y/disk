package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"disk/internal/model"
)

// TestUserRepositoryCreateRootMySQL exercises the registration persistence path
// against a migrated MySQL database. The outer transaction keeps the fixture local
// to this test even though CreateRoot starts its own nested transaction.
func TestUserRepositoryCreateRootMySQL(t *testing.T) {
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
	repo := NewUserRepository(tx)
	username := fmt.Sprintf("repository_test_%d", time.Now().UnixNano())
	user := &model.User{
		Username:   username,
		Password:   "test-password-hash",
		QuotaLimit: model.DefaultQuotaLimit,
	}
	if err := repo.CreateRoot(ctx, user); err != nil {
		t.Fatalf("create user and root folder: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("created user has no ID")
	}

	byName, err := repo.FindByUsername(ctx, username)
	if err != nil {
		t.Fatalf("find user by username: %v", err)
	}
	byID, err := repo.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("find user by ID: %v", err)
	}
	if byName.ID != user.ID || byID.Username != username {
		t.Fatalf("unexpected users: by name=%+v, by ID=%+v", byName, byID)
	}

	var roots []model.Folder
	if err := tx.WithContext(ctx).
		Where("user_id = ? AND parent_id IS NULL", user.ID).
		Find(&roots).Error; err != nil {
		t.Fatalf("query root folders: %v", err)
	}
	if len(roots) != 1 || roots[0].Name != RootFolderName {
		t.Fatalf("expected one root folder named %q, got %+v", RootFolderName, roots)
	}

	if _, err := repo.FindByUsername(ctx, username+"_missing"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user: expected ErrUserNotFound, got %v", err)
	}
}

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"disk/internal/model"

	"gorm.io/gorm"
)

func createFilesTestUser(t *testing.T, db *gorm.DB, quota int64) (*model.User, *model.Folder) {
	t.Helper()
	user := &model.User{
		Username:   fmt.Sprintf("files_repository_test_%d", time.Now().UnixNano()),
		Password:   "test-password-hash",
		QuotaLimit: quota,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	folder := &model.Folder{UserID: user.ID, Name: "test-folder"}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("create fixture folder: %v", err)
	}
	return user, folder
}

func filesTestSHA(n int) string {
	return fmt.Sprintf("%064x", n)
}

func mustUploadFilesTestFile(t *testing.T, fx *filesTestFixture, name string, sha string, size int64) (*model.File, *model.PhysicalFile, bool) {
	t.Helper()
	key := fmt.Sprintf("objects/test/files/%d/%s/%s", fx.userID, name, sha)
	file, physical, created, err := fx.repo.CreateUploadedFile(context.Background(), fx.userID, fx.folderID, name, sha, size, key)
	if err != nil || file == nil || physical == nil {
		t.Fatalf("upload fixture file %q: file=%+v, physical=%+v, err=%v", name, file, physical, err)
	}
	return file, physical, created
}

func mustReplaceFilesTestContent(t *testing.T, fx *filesTestFixture, fileID uint64, expectedSHA, nextSHA string, size int64) (*model.File, *model.PhysicalFile, bool) {
	t.Helper()
	key := fmt.Sprintf("objects/test/files/%d/replacement/%s", fx.userID, nextSHA)
	file, physical, created, err := fx.repo.ReplaceFileContent(context.Background(), fx.userID, fileID, expectedSHA, nextSHA, size, key, true)
	if err != nil || file == nil || physical == nil {
		t.Fatalf("replace fixture file %d: file=%+v, physical=%+v, err=%v", fileID, file, physical, err)
	}
	return file, physical, created
}

func filesTestVersionCount(t *testing.T, db *gorm.DB, fileID uint64) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.FileVersion{}).Where("file_id = ?", fileID).Count(&count).Error; err != nil {
		t.Fatalf("count file versions: %v", err)
	}
	return count
}

func filesTestSavedFile(t *testing.T, db *gorm.DB, fileID uint64) model.File {
	t.Helper()
	var file model.File
	if err := db.First(&file, fileID).Error; err != nil {
		t.Fatalf("reload file %d: %v", fileID, err)
	}
	return file
}

func filesTestUsedQuota(t *testing.T, db *gorm.DB, userID uint64) int64 {
	t.Helper()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatalf("reload user %d: %v", userID, err)
	}
	return user.UsedQuota
}

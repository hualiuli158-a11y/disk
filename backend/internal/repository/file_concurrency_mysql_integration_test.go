package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"disk/internal/model"

	"gorm.io/gorm"
)

type filesConcurrentResult struct {
	file     *model.File
	physical *model.PhysicalFile
	created  bool
	err      error
}

type filesConcurrentAction func(context.Context, *gorm.DB) filesConcurrentResult

// These tests need committed fixtures: a transaction owned by the test would
// hide its rows from the two independent connections used below.
func newCommittedFilesTestFixture(t *testing.T, quota int64) *filesTestFixture {
	t.Helper()
	db, users, folders := newCommittedFilesTestUsers(t, quota)
	return &filesTestFixture{tx: db, repo: NewFilesRepository(db), userID: users[0].ID, folderID: folders[0].ID}
}

func newCommittedFilesTestUsers(t *testing.T, quotas ...int64) (*gorm.DB, []*model.User, []*model.Folder) {
	t.Helper()
	db := openRepositoryTestDB(t)
	users := make([]*model.User, 0, len(quotas))
	folders := make([]*model.Folder, 0, len(quotas))
	for _, quota := range quotas {
		user, folder := createFilesTestUser(t, db, quota)
		users = append(users, user)
		folders = append(folders, folder)
	}
	t.Cleanup(func() {
		for _, user := range users {
			fileIDs := db.Unscoped().Model(&model.File{}).Select("id").Where("user_id = ?", user.ID)
			if err := db.Where("file_id IN (?)", fileIDs).Delete(&model.FileVersion{}).Error; err != nil {
				t.Errorf("clean fixture versions for user %d: %v", user.ID, err)
			}
			if err := db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.File{}).Error; err != nil {
				t.Errorf("clean fixture files for user %d: %v", user.ID, err)
			}
			if err := db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.Folder{}).Error; err != nil {
				t.Errorf("clean fixture folders for user %d: %v", user.ID, err)
			}
			if err := db.Delete(&model.User{}, user.ID).Error; err != nil {
				t.Errorf("clean fixture user %d: %v", user.ID, err)
			}
		}
		for _, user := range users {
			keyPrefix := fmt.Sprintf("objects/test/files/%d/%%", user.ID)
			if err := db.Where("storage_key LIKE ?", keyPrefix).Delete(&model.PhysicalFile{}).Error; err != nil {
				t.Errorf("clean fixture physical files for user %d: %v", user.ID, err)
			}
		}
	})
	return db, users, folders
}

func runFilesConcurrently(t *testing.T, db *gorm.DB, actions [2]filesConcurrentAction) [2]filesConcurrentResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ready := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var results [2]filesConcurrentResult
	for i := range actions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			entered := false
			err := db.WithContext(ctx).Connection(func(connDB *gorm.DB) error {
				entered = true
				ready <- nil
				<-start
				results[i] = actions[i](ctx, connDB)
				return nil
			})
			if !entered {
				ready <- err
			}
			if err != nil {
				results[i].err = err
			}
		}(i)
	}
	var connectionErr error
	for range actions {
		if err := <-ready; err != nil {
			connectionErr = err
		}
	}
	close(start)
	wg.Wait()
	if connectionErr != nil {
		t.Fatalf("open independent database connections: %v", connectionErr)
	}
	return results
}

func TestFilesRepositoryCreateUploadedFileConcurrentSameSHA(t *testing.T) {
	db, users, folders := newCommittedFilesTestUsers(t, 100, 100)
	sha := filesTestSHA(301)
	var actions [2]filesConcurrentAction
	for i, name := range []string{"first.txt", "second.txt"} {
		i, name := i, name
		actions[i] = func(ctx context.Context, db *gorm.DB) filesConcurrentResult {
			key := fmt.Sprintf("objects/test/files/%d/concurrent/%s", users[i].ID, name)
			file, physical, created, err := NewFilesRepository(db).CreateUploadedFile(ctx, users[i].ID, folders[i].ID, name, sha, 10, key)
			return filesConcurrentResult{file, physical, created, err}
		}
	}
	results := runFilesConcurrently(t, db, actions)
	createdCount := 0
	for i, result := range results {
		if result.err != nil || result.file == nil || result.physical == nil {
			t.Fatalf("concurrent upload %d: %+v", i, result)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 || results[0].file.ID == results[1].file.ID ||
		results[0].physical.ID != results[1].physical.ID ||
		results[0].physical.StorageKey != results[1].physical.StorageKey {
		t.Fatalf("same-SHA results: %+v", results)
	}
	assertUploadState(t, db, users[0].ID, 2, 1, 2, 10)
	if got := filesTestUsedQuota(t, db, users[1].ID); got != 10 {
		t.Fatalf("second user's quota: got %d, want 10", got)
	}
}

func TestFilesRepositoryCreateUploadedFileConcurrentQuotaLimit(t *testing.T) {
	fx := newCommittedFilesTestFixture(t, 10)
	var actions [2]filesConcurrentAction
	for i, name := range []string{"first.txt", "second.txt"} {
		i, name := i, name
		actions[i] = func(ctx context.Context, db *gorm.DB) filesConcurrentResult {
			sha := filesTestSHA(302 + i)
			key := fmt.Sprintf("objects/test/files/%d/concurrent/%s", fx.userID, name)
			file, physical, created, err := NewFilesRepository(db).CreateUploadedFile(ctx, fx.userID, fx.folderID, name, sha, 10, key)
			return filesConcurrentResult{file, physical, created, err}
		}
	}
	results := runFilesConcurrently(t, fx.tx, actions)
	successes, quotaErrors := 0, 0
	for i, result := range results {
		switch {
		case result.err == nil && result.file != nil && result.physical != nil && result.created:
			successes++
		case errors.Is(result.err, ErrQuotaExceeded) && result.file == nil && result.physical == nil && !result.created:
			quotaErrors++
		default:
			t.Fatalf("unexpected concurrent quota result %d: %+v", i, result)
		}
	}
	if successes != 1 || quotaErrors != 1 {
		t.Fatalf("concurrent quota results: %+v", results)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 10)
}

func TestFilesRepositoryReplaceFileContentConcurrentExpectedSHA(t *testing.T) {
	fx := newCommittedFilesTestFixture(t, 100)
	oldSHA := filesTestSHA(304)
	file, _, _ := mustUploadFilesTestFile(t, fx, "editable.txt", oldSHA, 10)
	var actions [2]filesConcurrentAction
	for i := range actions {
		i := i
		actions[i] = func(ctx context.Context, db *gorm.DB) filesConcurrentResult {
			nextSHA := filesTestSHA(305 + i)
			key := fmt.Sprintf("objects/test/files/%d/concurrent-replacement/%d", fx.userID, i)
			updated, physical, created, err := NewFilesRepository(db).ReplaceFileContent(
				ctx, fx.userID, file.ID, oldSHA, nextSHA, int64(20+10*i), key, true,
			)
			return filesConcurrentResult{updated, physical, created, err}
		}
	}
	results := runFilesConcurrently(t, fx.tx, actions)
	successes, conflicts := 0, 0
	var winner filesConcurrentResult
	for i, result := range results {
		switch {
		case result.err == nil && result.file != nil && result.physical != nil && result.created:
			successes++
			winner = result
		case errors.Is(result.err, ErrFileVersionConflict) && result.file == nil && result.physical == nil && !result.created:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent replacement result %d: %+v", i, result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent replacement results: %+v", results)
	}
	if saved := filesTestSavedFile(t, fx.tx, file.ID); saved.PhysicalFileID != winner.physical.ID {
		t.Fatalf("winning physical reference: got %d, want %d", saved.PhysicalFileID, winner.physical.ID)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 2, 2, winner.physical.Size)
}

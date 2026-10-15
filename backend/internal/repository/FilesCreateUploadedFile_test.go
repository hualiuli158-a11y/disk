package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"disk/internal/model"

	"gorm.io/gorm"
)

const uploadTestSize int64 = 10
const uploadTestStorageKey = "objects/test/uploaded-file/original"

// Each case gets its own user, folder and transaction.
type filesTestFixture struct {
	tx       *gorm.DB
	repo     *FilesRepository
	userID   uint64
	folderID uint64
}

func newFilesTestFixture(t *testing.T) *filesTestFixture {
	t.Helper()
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

	// 直接写入前置数据，避免依赖 UserRepository.CreateRoot。
	user, folder := createFilesTestUser(t, tx, 100)
	return &filesTestFixture{
		tx:       tx,
		repo:     NewFilesRepository(tx),
		userID:   user.ID,
		folderID: folder.ID,
	}
}

func TestFilesRepositoryCreateUploadedFileNewPhysical(t *testing.T) {
	fx := newFilesTestFixture(t)
	sha := strings.Repeat("a", 64)

	file, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "first.txt", sha, uploadTestSize, uploadTestStorageKey,
	)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if file == nil || physical == nil || file.ID == 0 || physical.ID == 0 || !created {
		t.Fatalf("first upload returned file=%+v, physical=%+v, created=%t", file, physical, created)
	}
	if file.UserID != fx.userID || file.FolderID != fx.folderID || file.Name != "first.txt" ||
		file.PhysicalFileID != physical.ID || physical.SHA256 != sha ||
		physical.Size != uploadTestSize || physical.StorageKey != uploadTestStorageKey {
		t.Fatalf("incorrect first upload metadata: file=%+v, physical=%+v", file, physical)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, uploadTestSize)
	assertInitialUploadVersion(t, fx.tx, file.ID, physical.ID)
}

func TestFilesRepositoryCreateUploadedFileReusesPhysical(t *testing.T) {
	fx := newFilesTestFixture(t)
	sha := strings.Repeat("a", 64)
	first, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "first.txt", sha, uploadTestSize, uploadTestStorageKey,
	)
	if err != nil || first == nil || physical == nil || !created {
		t.Fatalf("prepare first upload: file=%+v, physical=%+v, created=%t, err=%v", first, physical, created, err)
	}

	second, reused, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "second.txt", sha, uploadTestSize, "objects/test/uploaded-file/candidate",
	)
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if second == nil || reused == nil || second.ID == first.ID || second.Name != "second.txt" ||
		second.PhysicalFileID != physical.ID || reused.ID != physical.ID ||
		reused.StorageKey != uploadTestStorageKey || created {
		t.Fatalf("deduplication returned file=%+v, physical=%+v, created=%t", second, reused, created)
	}
	assertUploadState(t, fx.tx, fx.userID, 2, 1, 2, 2*uploadTestSize)
	assertInitialUploadVersion(t, fx.tx, first.ID, physical.ID)
	assertInitialUploadVersion(t, fx.tx, second.ID, physical.ID)
}

func TestFilesRepositoryCreateUploadedFileDuplicateNameRollsBack(t *testing.T) {
	fx := newFilesTestFixture(t)
	first, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "first.txt", strings.Repeat("a", 64), uploadTestSize, uploadTestStorageKey,
	)
	if err != nil || first == nil || physical == nil || !created {
		t.Fatalf("prepare first upload: file=%+v, physical=%+v, created=%t, err=%v", first, physical, created, err)
	}

	// The physical insert precedes the duplicate-name check, so it must roll back.
	file, stored, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "first.txt", strings.Repeat("b", 64), 7, "objects/test/uploaded-file/duplicate-name",
	)
	if !errors.Is(err, ErrFileAlreadyExists) || file != nil || stored != nil || created {
		t.Fatalf("duplicate name: file=%+v, physical=%+v, created=%t, err=%v", file, stored, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, uploadTestSize)
}

func TestFilesRepositoryCreateUploadedFileQuotaExceeded(t *testing.T) {
	fx := newFilesTestFixture(t)

	file, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "too-large.txt", strings.Repeat("c", 64), 101, "objects/test/uploaded-file/too-large",
	)
	if !errors.Is(err, ErrQuotaExceeded) || file != nil || physical != nil || created {
		t.Fatalf("quota exceeded: file=%+v, physical=%+v, created=%t, err=%v", file, physical, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 0, 0, 0, 0)
}

func TestFilesRepositoryCreateUploadedFileMissingFolder(t *testing.T) {
	fx := newFilesTestFixture(t)

	file, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, 0, "wrong-folder.txt", strings.Repeat("d", 64), 5, "objects/test/uploaded-file/wrong-folder",
	)
	if !errors.Is(err, ErrFileFolderNotFound) || file != nil || physical != nil || created {
		t.Fatalf("missing folder: file=%+v, physical=%+v, created=%t, err=%v", file, physical, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 0, 0, 0, 0)
}

func TestFilesRepositoryCreateUploadedFileOtherUsersFolder(t *testing.T) {
	fx := newFilesTestFixture(t)
	_, otherFolder := createFilesTestUser(t, fx.tx, 100)
	file, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, otherFolder.ID, "private.txt", filesTestSHA(10), 5, "objects/test/files/private",
	)
	if !errors.Is(err, ErrFileFolderNotFound) || file != nil || physical != nil || created {
		t.Fatalf("other user's folder: file=%+v, physical=%+v, created=%t, err=%v", file, physical, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 0, 0, 0, 0)
}

func TestFilesRepositoryCreateUploadedFileMissingUser(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), 0, fx.folderID, "orphan.txt", filesTestSHA(11), 5, "objects/test/files/orphan",
	)
	if !errors.Is(err, ErrUserNotFound) || file != nil || physical != nil || created {
		t.Fatalf("missing user: file=%+v, physical=%+v, created=%t, err=%v", file, physical, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 0, 0, 0, 0)
}

func TestFilesRepositoryCreateUploadedFileNegativeSize(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, created, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, fx.folderID, "negative.txt", filesTestSHA(12), -1, "objects/test/files/negative",
	)
	if !errors.Is(err, ErrQuotaExceeded) || file != nil || physical != nil || created {
		t.Fatalf("negative size: file=%+v, physical=%+v, created=%t, err=%v", file, physical, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 0, 0, 0, 0)
}

func TestFilesRepositoryCreateUploadedFileExactlyFillsQuota(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, created := mustUploadFilesTestFile(t, fx, "full.txt", filesTestSHA(13), 100)
	if !created || file.PhysicalFileID != physical.ID {
		t.Fatalf("exact quota upload: file=%+v, physical=%+v, created=%t", file, physical, created)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 100)
}

func assertInitialUploadVersion(t *testing.T, db *gorm.DB, fileID, physicalID uint64) {
	t.Helper()
	var version model.FileVersion
	if err := db.Where("file_id = ?", fileID).First(&version).Error; err != nil {
		t.Fatalf("find initial version for file %d: %v", fileID, err)
	}
	if version.PhysicalFileID != physicalID || version.Source != "upload" {
		t.Fatalf("incorrect initial version for file %d: %+v", fileID, version)
	}
}

func assertUploadState(t *testing.T, db *gorm.DB, userID uint64, wantFiles, wantPhysical, wantVersions, wantQuota int64) {
	t.Helper()
	for _, check := range []struct {
		name  string
		model any
		want  int64
	}{
		{"logical files", &model.File{}, wantFiles},
		{"physical files", &model.PhysicalFile{}, wantPhysical},
		{"file versions", &model.FileVersion{}, wantVersions},
	} {
		var count int64
		if err := db.Model(check.model).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != check.want {
			t.Errorf("%s: got %d, want %d", check.name, count, check.want)
		}
	}

	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.UsedQuota != wantQuota {
		t.Errorf("used quota: got %d, want %d", user.UsedQuota, wantQuota)
	}
}

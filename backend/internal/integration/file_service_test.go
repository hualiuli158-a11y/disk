package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
	"disk/internal/service"
	"disk/internal/storage"

	"gorm.io/gorm"
)

type fileServiceFixture struct {
	tx          *gorm.DB
	objects     storage.Storage
	storageRoot string
	upload      *service.UploadFileService
	download    *service.DownloadFileService
	userID      uint64
	folderID    uint64
}

func newFileServiceFixture(t *testing.T) *fileServiceFixture {
	t.Helper()
	if os.Getenv("DISK_MYSQL_INTEGRATION") != "1" {
		t.Skip("set DISK_MYSQL_INTEGRATION=1 to run MySQL and local storage integration tests")
	}
	if fileServiceTestDB == nil {
		t.Fatal("TestMain did not initialize the file service test MySQL")
	}

	root := t.TempDir()
	objects, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("create local object storage: %v", err)
	}
	tx := fileServiceTestDB.Begin()
	if tx.Error != nil {
		t.Fatalf("begin test transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Errorf("rollback test transaction: %v", err)
		}
	})

	user := &model.User{
		Username:   fmt.Sprintf("file_service_integration_%d", time.Now().UnixNano()),
		Password:   "test-password-hash",
		QuotaLimit: 1024,
	}
	if err := tx.Create(user).Error; err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	folder := &model.Folder{UserID: user.ID, Name: "uploads"}
	if err := tx.Create(folder).Error; err != nil {
		t.Fatalf("create fixture folder: %v", err)
	}
	files := repository.NewFilesRepository(tx)
	return &fileServiceFixture{
		tx:          tx,
		objects:     objects,
		storageRoot: root,
		upload:      service.NewUploadFileService(files, objects, 1024),
		download:    service.NewDownloadFileService(files, objects),
		userID:      user.ID,
		folderID:    folder.ID,
	}
}

func uploadFixtureBytes(t *testing.T, fx *fileServiceFixture, name string, content []byte) *service.UploadFileOutput {
	t.Helper()
	result, err := fx.upload.UploadFile(context.Background(), service.UploadFileInput{
		UserID:          fx.userID,
		FolderID:        fx.folderID,
		Name:            name,
		Content:         bytes.NewReader(content),
		DeclaredSize:    int64(len(content)),
		HasDeclaredSize: true,
	})
	if err != nil || result == nil {
		t.Fatalf("upload %q: result=%+v, err=%v", name, result, err)
	}
	return result
}

func assertDownloadedBytes(t *testing.T, fx *fileServiceFixture, fileID uint64, wantName string, want []byte) {
	t.Helper()
	result, err := fx.download.DownloadFile(context.Background(), fx.userID, fileID)
	if err != nil || result == nil || result.Content == nil {
		t.Fatalf("download file %d: result=%+v, err=%v", fileID, result, err)
	}
	got, readErr := io.ReadAll(result.Content)
	closeErr := result.Content.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read downloaded file %d: read=%v, close=%v", fileID, readErr, closeErr)
	}
	wantSHA := sha256.Sum256(want)
	if result.Name != wantName || result.Size != int64(len(want)) ||
		result.SHA256 != hex.EncodeToString(wantSHA[:]) || !bytes.Equal(got, want) {
		t.Fatalf("download file %d: name=%q, size=%d, sha=%q, bytes=%q; want name=%q, size=%d, sha=%x, bytes=%q",
			fileID, result.Name, result.Size, result.SHA256, got, wantName, len(want), wantSHA, want)
	}
}

func fileServiceRecords(t *testing.T, fx *fileServiceFixture, fileID uint64) (model.File, model.PhysicalFile) {
	t.Helper()
	var logical model.File
	if err := fx.tx.Where("id = ? AND user_id = ?", fileID, fx.userID).First(&logical).Error; err != nil {
		t.Fatalf("database logical file %d: %v", fileID, err)
	}
	var physical model.PhysicalFile
	if err := fx.tx.First(&physical, logical.PhysicalFileID).Error; err != nil {
		t.Fatalf("database physical file %d: %v", logical.PhysicalFileID, err)
	}
	return logical, physical
}

func assertFileServiceDatabaseState(t *testing.T, fx *fileServiceFixture, wantLogical, wantPhysical, wantVersions, wantQuota int64) {
	t.Helper()
	for _, check := range []struct {
		name  string
		model any
		want  int64
	}{
		{"logical files", &model.File{}, wantLogical},
		{"physical files", &model.PhysicalFile{}, wantPhysical},
		{"file versions", &model.FileVersion{}, wantVersions},
	} {
		var count int64
		if err := fx.tx.Model(check.model).Count(&count).Error; err != nil {
			t.Fatalf("database count %s: %v", check.name, err)
		}
		if count != check.want {
			t.Errorf("database %s: got %d, want %d", check.name, count, check.want)
		}
	}
	var user model.User
	if err := fx.tx.First(&user, fx.userID).Error; err != nil {
		t.Fatalf("database reload user: %v", err)
	}
	if user.UsedQuota != wantQuota {
		t.Errorf("database used quota: got %d, want %d", user.UsedQuota, wantQuota)
	}
}

func assertInitialVersion(t *testing.T, fx *fileServiceFixture, fileID, physicalID uint64) {
	t.Helper()
	var versions []model.FileVersion
	if err := fx.tx.Where("file_id = ?", fileID).Find(&versions).Error; err != nil {
		t.Fatalf("database versions for file %d: %v", fileID, err)
	}
	if len(versions) != 1 || versions[0].Source != "upload" || versions[0].PhysicalFileID != physicalID {
		t.Fatalf("database initial version for file %d: %+v", fileID, versions)
	}
}

func assertLocalObjects(t *testing.T, fx *fileServiceFixture, wantKeys ...string) {
	t.Helper()
	objects, err := fx.objects.List(context.Background(), "objects")
	if err != nil {
		t.Fatalf("local object inventory: %v", err)
	}
	if len(objects) != len(wantKeys) {
		t.Fatalf("local object count: got %d (%+v), want %d (%v)", len(objects), objects, len(wantKeys), wantKeys)
	}
	for i, key := range wantKeys {
		if objects[i].Key != key {
			t.Errorf("local object %d: got %q, want %q", i, objects[i].Key, key)
		}
	}
}

func assertLocalObjectBytes(t *testing.T, fx *fileServiceFixture, key string, want []byte) {
	t.Helper()
	path := filepath.Join(fx.storageRoot, filepath.FromSlash(key))
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("local object %q does not exist: %v", key, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("local object %q: got %q, want %q", key, got, want)
	}
}

func TestUploadFileServiceUploadFileRoundTrip(t *testing.T) {
	fx := newFileServiceFixture(t)
	content := []byte("known upload bytes\n")
	wantSHA := sha256.Sum256(content)
	result := uploadFixtureBytes(t, fx, "hello.txt", content)
	if result.ID == 0 || result.FolderID != fx.folderID || result.Name != "hello.txt" ||
		result.Size != int64(len(content)) || result.SHA256 != hex.EncodeToString(wantSHA[:]) {
		t.Fatalf("upload result: %+v", result)
	}
	assertDownloadedBytes(t, fx, result.ID, "hello.txt", content)
	assertFileServiceDatabaseState(t, fx, 1, 1, 1, int64(len(content)))
	logical, physical := fileServiceRecords(t, fx, result.ID)
	if logical.Name != "hello.txt" || logical.FolderID != fx.folderID ||
		logical.PhysicalFileID != physical.ID || physical.Size != int64(len(content)) ||
		physical.SHA256 != hex.EncodeToString(wantSHA[:]) || physical.StorageKey == "" {
		t.Fatalf("database upload records: logical=%+v, physical=%+v", logical, physical)
	}
	assertInitialVersion(t, fx, result.ID, physical.ID)
	assertLocalObjects(t, fx, physical.StorageKey)
	assertLocalObjectBytes(t, fx, physical.StorageKey, content)
}

func TestUploadFileServiceUploadFileRemovesCandidateOnDuplicateName(t *testing.T) {
	fx := newFileServiceFixture(t)
	firstContent := []byte("first payload")
	first := uploadFixtureBytes(t, fx, "first.txt", firstContent)
	_, originalPhysical := fileServiceRecords(t, fx, first.ID)

	second, err := fx.upload.UploadFile(context.Background(), service.UploadFileInput{
		UserID:          fx.userID,
		FolderID:        fx.folderID,
		Name:            "first.txt",
		Content:         bytes.NewReader([]byte("different payload")),
		DeclaredSize:    int64(len("different payload")),
		HasDeclaredSize: true,
	})
	if !errors.Is(err, service.ErrFileAlreadyExists) || second != nil {
		t.Fatalf("duplicate upload: result=%+v, err=%v; want ErrFileAlreadyExists", second, err)
	}
	assertDownloadedBytes(t, fx, first.ID, "first.txt", firstContent)
	assertFileServiceDatabaseState(t, fx, 1, 1, 1, int64(len(firstContent)))
	logical, physical := fileServiceRecords(t, fx, first.ID)
	if logical.PhysicalFileID != originalPhysical.ID || physical.StorageKey != originalPhysical.StorageKey {
		t.Fatalf("database original reference changed: logical=%+v, physical=%+v", logical, physical)
	}
	assertInitialVersion(t, fx, first.ID, originalPhysical.ID)
	assertLocalObjects(t, fx, originalPhysical.StorageKey)
	assertLocalObjectBytes(t, fx, originalPhysical.StorageKey, firstContent)
}

func TestUploadFileServiceUploadFileDeduplicatesSameContent(t *testing.T) {
	fx := newFileServiceFixture(t)
	content := []byte("the same bytes for both names")
	first := uploadFixtureBytes(t, fx, "first.txt", content)
	second := uploadFixtureBytes(t, fx, "second.txt", content)
	if first.ID == second.ID || first.SHA256 != second.SHA256 || first.Size != second.Size {
		t.Fatalf("deduplicated upload results: first=%+v, second=%+v", first, second)
	}
	assertDownloadedBytes(t, fx, first.ID, "first.txt", content)
	assertDownloadedBytes(t, fx, second.ID, "second.txt", content)
	assertFileServiceDatabaseState(t, fx, 2, 1, 2, 2*int64(len(content)))
	firstLogical, firstPhysical := fileServiceRecords(t, fx, first.ID)
	secondLogical, secondPhysical := fileServiceRecords(t, fx, second.ID)
	if firstLogical.Name != "first.txt" || secondLogical.Name != "second.txt" ||
		firstLogical.FolderID != fx.folderID || secondLogical.FolderID != fx.folderID ||
		firstLogical.PhysicalFileID != secondLogical.PhysicalFileID ||
		firstPhysical.ID != secondPhysical.ID || firstPhysical.StorageKey != secondPhysical.StorageKey {
		t.Fatalf("database did not reuse physical file: first=%+v/%+v, second=%+v/%+v",
			firstLogical, firstPhysical, secondLogical, secondPhysical)
	}
	assertInitialVersion(t, fx, first.ID, firstPhysical.ID)
	assertInitialVersion(t, fx, second.ID, firstPhysical.ID)
	assertLocalObjects(t, fx, firstPhysical.StorageKey)
	assertLocalObjectBytes(t, fx, firstPhysical.StorageKey, content)
}

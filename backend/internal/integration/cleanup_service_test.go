package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
	"disk/internal/service"
)

func ageLocalObject(t *testing.T, fx *fileServiceFixture, key string, age time.Time) {
	t.Helper()
	path := filepath.Join(fx.storageRoot, filepath.FromSlash(key))
	if err := os.Chtimes(path, age, age); err != nil {
		t.Fatalf("age local object %q: %v", key, err)
	}
}

func TestOrphanCleanupServiceRunRemovesOrphansAndKeepsReferences(t *testing.T) {
	fx := newFileServiceFixture(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour)
	keptContent := []byte("referenced content")
	kept := uploadFixtureBytes(t, fx, "kept.txt", keptContent)
	_, referenced := fileServiceRecords(t, fx, kept.ID)
	if err := fx.tx.Model(&model.PhysicalFile{}).Where("id = ?", referenced.ID).
		UpdateColumn("created_at", old).Error; err != nil {
		t.Fatalf("age referenced physical record: %v", err)
	}
	ageLocalObject(t, fx, referenced.StorageKey, old)

	orphanContent := []byte("unreferenced database content")
	orphanSHA := sha256.Sum256(orphanContent)
	orphan := &model.PhysicalFile{
		SHA256:     hex.EncodeToString(orphanSHA[:]),
		Size:       int64(len(orphanContent)),
		StorageKey: "objects/aa/database-orphan",
		CreatedAt:  old,
	}
	if err := fx.tx.Create(orphan).Error; err != nil {
		t.Fatalf("create orphan physical record: %v", err)
	}
	if err := fx.objects.Save(ctx, orphan.StorageKey, bytes.NewReader(orphanContent)); err != nil {
		t.Fatalf("save database orphan object: %v", err)
	}
	ageLocalObject(t, fx, orphan.StorageKey, old)

	const storageOnlyKey = "objects/bb/storage-orphan"
	if err := fx.objects.Save(ctx, storageOnlyKey, bytes.NewReader([]byte("storage only"))); err != nil {
		t.Fatalf("save storage-only orphan: %v", err)
	}
	ageLocalObject(t, fx, storageOnlyKey, old)

	cleanup := service.NewOrphanCleanupService(repository.NewCleanupRepository(fx.tx), fx.objects, 24*time.Hour, 100)
	result, err := cleanup.Run(ctx)
	if err != nil {
		t.Fatalf("run orphan cleanup: %v", err)
	}
	if result.DatabaseCandidates != 1 || result.DatabaseDeleted != 1 ||
		result.StorageScanned != 2 || result.StorageDeleted != 1 || result.Failures != 0 {
		t.Fatalf("cleanup result: %+v", result)
	}
	assertFileServiceDatabaseState(t, fx, 1, 1, 1, int64(len(keptContent)))
	if _, physical := fileServiceRecords(t, fx, kept.ID); physical.ID != referenced.ID {
		t.Fatalf("referenced physical record changed: got %d, want %d", physical.ID, referenced.ID)
	}
	assertLocalObjects(t, fx, referenced.StorageKey)
	assertLocalObjectBytes(t, fx, referenced.StorageKey, keptContent)
	for _, key := range []string{orphan.StorageKey, storageOnlyKey} {
		path := filepath.Join(fx.storageRoot, filepath.FromSlash(key))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("orphan local object %q still exists or cannot be checked: %v", key, err)
		}
	}
}

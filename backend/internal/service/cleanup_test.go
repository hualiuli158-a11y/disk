package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/storage"
)

type cleanupRepositoryFake struct {
	candidates []model.PhysicalFile
	deleteKeys map[uint64]string
	existing   map[string]bool
	listErr    error
}

func (f *cleanupRepositoryFake) ListUnreferencedPhysicalFiles(context.Context, time.Time, int) ([]model.PhysicalFile, error) {
	return f.candidates, f.listErr
}

func (f *cleanupRepositoryFake) DeletePhysicalFileIfUnreferenced(_ context.Context, id uint64) (string, bool, error) {
	key, ok := f.deleteKeys[id]
	return key, ok, nil
}

func (f *cleanupRepositoryFake) PhysicalStorageKeyExists(_ context.Context, key string) (bool, error) {
	return f.existing[key], nil
}

type cleanupStorageFake struct {
	objects []storage.ObjectInfo
	deleted []string
}

type cleanupPagedStorageFake struct {
	*cleanupStorageFake
	after []string
}

func (f *cleanupPagedStorageFake) ListPage(_ context.Context, _ string, afterKey string, limit int) ([]storage.ObjectInfo, error) {
	f.after = append(f.after, afterKey)
	page := make([]storage.ObjectInfo, 0, limit)
	for _, object := range f.objects {
		if object.Key > afterKey {
			page = append(page, object)
			if len(page) == limit {
				break
			}
		}
	}
	return page, nil
}

func (f *cleanupStorageFake) Save(context.Context, string, io.Reader) error { return nil }

func (f *cleanupStorageFake) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (f *cleanupStorageFake) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

func (f *cleanupStorageFake) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return f.objects, nil
}

func TestOrphanCleanupServiceRunDeletesUnreferencedObjects(t *testing.T) {
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	repository := &cleanupRepositoryFake{
		candidates: []model.PhysicalFile{{ID: 1, StorageKey: "objects/aa/db-orphan"}},
		deleteKeys: map[uint64]string{1: "objects/aa/db-orphan"},
		existing: map[string]bool{
			"objects/cc/referenced": true,
		},
	}
	objects := &cleanupStorageFake{objects: []storage.ObjectInfo{
		{Key: "objects/aa/db-orphan", ModifiedAt: now.Add(-48 * time.Hour)},
		{Key: "objects/bb/storage-orphan", ModifiedAt: now.Add(-48 * time.Hour)},
		{Key: "objects/cc/referenced", ModifiedAt: now.Add(-48 * time.Hour)},
		{Key: "objects/dd/recent-upload", ModifiedAt: now.Add(-time.Hour)},
	}}
	cleanup := NewOrphanCleanupService(repository, objects, 24*time.Hour, 100)
	cleanup.now = func() time.Time { return now }

	result, err := cleanup.Run(context.Background())
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if result.DatabaseCandidates != 1 || result.DatabaseDeleted != 1 || result.StorageScanned != 2 || result.StorageDeleted != 1 || result.Failures != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if len(objects.deleted) != 2 || objects.deleted[0] != "objects/aa/db-orphan" || objects.deleted[1] != "objects/bb/storage-orphan" {
		t.Fatalf("unexpected deleted objects: %v", objects.deleted)
	}
}

func TestOrphanCleanupServiceRunReturnsListError(t *testing.T) {
	wantErr := errors.New("database unavailable")
	cleanup := NewOrphanCleanupService(
		&cleanupRepositoryFake{listErr: wantErr},
		&cleanupStorageFake{},
		time.Hour,
		10,
	)
	_, err := cleanup.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("cleanup error = %v, want %v", err, wantErr)
	}
}

func TestOrphanCleanupServiceRunAdvancesAcrossPages(t *testing.T) {
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	repository := &cleanupRepositoryFake{existing: map[string]bool{
		"objects/aa/first": true, "objects/bb/second": true, "objects/cc/third": true,
	}}
	objects := &cleanupPagedStorageFake{cleanupStorageFake: &cleanupStorageFake{objects: []storage.ObjectInfo{
		{Key: "objects/aa/first", ModifiedAt: now.Add(-48 * time.Hour)},
		{Key: "objects/bb/second", ModifiedAt: now.Add(-48 * time.Hour)},
		{Key: "objects/cc/third", ModifiedAt: now.Add(-48 * time.Hour)},
	}}}
	cleanup := NewOrphanCleanupService(repository, objects, 24*time.Hour, 1)
	cleanup.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		result, err := cleanup.Run(context.Background())
		if err != nil || result.StorageScanned != 1 {
			t.Fatalf("run %d: %+v %v", i, result, err)
		}
	}
	want := []string{"", "objects/aa/first", "objects/bb/second"}
	if !reflect.DeepEqual(objects.after, want) {
		t.Fatalf("page cursors=%v, want %v", objects.after, want)
	}
	end, err := cleanup.Run(context.Background())
	if err != nil || end.StorageScanned != 0 {
		t.Fatalf("end of scan: %+v %v", end, err)
	}
	restarted, err := cleanup.Run(context.Background())
	if err != nil || restarted.StorageScanned != 1 || objects.after[len(objects.after)-1] != "" {
		t.Fatalf("scan did not wrap: %+v %v cursors=%v", restarted, err, objects.after)
	}
}

func TestOrphanCleanupServiceRunAdvancesPastRecentObjects(t *testing.T) {
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	objects := &cleanupPagedStorageFake{cleanupStorageFake: &cleanupStorageFake{objects: []storage.ObjectInfo{
		{Key: "objects/aa/recent", ModifiedAt: now.Add(-time.Hour)},
		{Key: "objects/bb/older", ModifiedAt: now.Add(-48 * time.Hour)},
	}}}
	cleanup := NewOrphanCleanupService(&cleanupRepositoryFake{existing: map[string]bool{"objects/bb/older": true}}, objects, 24*time.Hour, 1)
	cleanup.now = func() time.Time { return now }
	first, err := cleanup.Run(context.Background())
	if err != nil || first.StorageScanned != 0 {
		t.Fatalf("recent first page: %+v %v", first, err)
	}
	second, err := cleanup.Run(context.Background())
	if err != nil || second.StorageScanned != 1 {
		t.Fatalf("older second page: %+v %v", second, err)
	}
}

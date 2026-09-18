package service

import (
	"context"
	"testing"
	"time"

	"disk/internal/model"
)

type trashServiceRepositoryFake struct {
	expired       []model.TrashEntry
	permanentKeys map[uint64][]string
	restored      []uint64
	deleted       []uint64
}

func (f *trashServiceRepositoryFake) ListTrash(context.Context, uint64, int, int) ([]model.TrashEntry, int64, error) {
	return []model.TrashEntry{}, 0, nil
}

func (f *trashServiceRepositoryFake) ListExpiredTrashEntries(context.Context, time.Time, int) ([]model.TrashEntry, error) {
	return f.expired, nil
}

func (f *trashServiceRepositoryFake) RestoreTrash(_ context.Context, _ uint64, trashID uint64) error {
	f.restored = append(f.restored, trashID)
	return nil
}

func (f *trashServiceRepositoryFake) PermanentlyDeleteTrash(_ context.Context, _ uint64, trashID uint64) ([]string, error) {
	f.deleted = append(f.deleted, trashID)
	return f.permanentKeys[trashID], nil
}

func TestTrashServicePurgeExpired(t *testing.T) {
	repository := &trashServiceRepositoryFake{
		expired: []model.TrashEntry{{ID: 1, UserID: 42}, {ID: 2, UserID: 43}},
		permanentKeys: map[uint64][]string{
			1: {"objects/aa/one"},
			2: {"objects/bb/two"},
		},
	}
	objects := &cleanupStorageFake{}
	trash := NewTrashService(repository, objects)
	result, err := trash.PurgeExpiredTrash(context.Background(), time.Now(), 100)
	if err != nil {
		t.Fatalf("purge expired trash failed: %v", err)
	}
	if result.Candidates != 2 || result.Deleted != 2 || result.Failures != 0 || result.CleanupFailures != 0 {
		t.Fatalf("unexpected purge result: %+v", result)
	}
	if len(repository.deleted) != 2 || len(objects.deleted) != 2 {
		t.Fatalf("unexpected deletions: trash=%v objects=%v", repository.deleted, objects.deleted)
	}
}

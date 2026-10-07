package bootstrap

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/service"
	"disk/internal/storage"
	"disk/pkg/logger"
)

type failingOrphanRepository struct{}

func (failingOrphanRepository) ListUnreferencedPhysicalFiles(context.Context, time.Time, int) ([]model.PhysicalFile, error) {
	return nil, errors.New("orphan database query failed")
}
func (failingOrphanRepository) DeletePhysicalFileIfUnreferenced(context.Context, uint64) (string, bool, error) {
	return "", false, nil
}
func (failingOrphanRepository) PhysicalStorageKeyExists(context.Context, string) (bool, error) {
	return false, nil
}

type noOpStorage struct{}

func (noOpStorage) Save(context.Context, string, io.Reader) error              { return nil }
func (noOpStorage) Open(context.Context, string) (io.ReadCloser, error)        { return nil, nil }
func (noOpStorage) Delete(context.Context, string) error                       { return nil }
func (noOpStorage) List(context.Context, string) ([]storage.ObjectInfo, error) { return nil, nil }

type failedHealthStorage struct {
	noOpStorage
	err error
}

func (s failedHealthStorage) Check(context.Context) error { return s.err }

func TestStartupRejectsUnavailableStorage(t *testing.T) {
	want := errors.New("storage tunnel unavailable")
	err := requireStorageReady(failedHealthStorage{err: want}, "grpc", "127.0.0.1:50051")
	if !errors.Is(err, want) {
		t.Fatalf("startup error=%v, want wrapped storage failure", err)
	}
	if err := requireStorageReady(noOpStorage{}, "local", ""); err != nil {
		t.Fatalf("local storage rejected: %v", err)
	}
}

type observedTrashRepository struct{ called chan struct{} }

func (r observedTrashRepository) ListTrash(context.Context, uint64, int, int) ([]model.TrashEntry, int64, error) {
	return nil, 0, nil
}
func (r observedTrashRepository) ListExpiredTrashEntries(context.Context, time.Time, int) ([]model.TrashEntry, error) {
	select {
	case r.called <- struct{}{}:
	default:
	}
	return nil, nil
}
func (r observedTrashRepository) RestoreTrash(context.Context, uint64, uint64) error { return nil }
func (r observedTrashRepository) PermanentlyDeleteTrash(context.Context, uint64, uint64) ([]string, error) {
	return nil, nil
}

func TestTrashPurgeRunsWhenOrphanCleanupFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	done := make(chan struct{})
	go runOrphanCleanupWorker(ctx,
		service.NewOrphanCleanupService(failingOrphanRepository{}, noOpStorage{}, time.Hour, 1),
		service.NewTrashService(observedTrashRepository{called: called}, noOpStorage{}),
		time.Hour, 1, logger.NewLogger(io.Discard), done)
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("trash purge did not run after orphan cleanup failed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup worker did not stop")
	}
}

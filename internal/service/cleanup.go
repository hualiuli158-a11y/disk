package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"disk/internal/model"
	"disk/internal/storage"
)

const (
	DefaultOrphanGrace  = 24 * time.Hour
	DefaultCleanupBatch = 1000
)

type OrphanCleanupRepository interface {
	ListUnreferencedPhysicalFiles(ctx context.Context, olderThan time.Time, limit int) ([]model.PhysicalFile, error)
	DeletePhysicalFileIfUnreferenced(ctx context.Context, physicalID uint64) (storageKey string, deleted bool, err error)
	PhysicalStorageKeyExists(ctx context.Context, storageKey string) (bool, error)
}

type OrphanCleanupService struct {
	repository OrphanCleanupRepository
	objects    ObjectStorage
	grace      time.Duration
	batch      int
	now        func() time.Time
	runMu      sync.Mutex
	scanCursor string
}

func NewOrphanCleanupService(repository OrphanCleanupRepository, objects ObjectStorage, grace time.Duration, batch int) *OrphanCleanupService {
	if grace <= 0 {
		grace = DefaultOrphanGrace
	}
	if batch <= 0 {
		batch = DefaultCleanupBatch
	}
	return &OrphanCleanupService{
		repository: repository, objects: objects, grace: grace, batch: batch, now: time.Now,
	}
}

type OrphanCleanupResult struct {
	DatabaseCandidates int
	DatabaseDeleted    int
	StorageScanned     int
	StorageDeleted     int
	Failures           int
}

func (s *OrphanCleanupService) Run(ctx context.Context) (*OrphanCleanupResult, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	cutoff := s.now().Add(-s.grace)
	result := &OrphanCleanupResult{}
	attemptedKeys := make(map[string]struct{})

	physicalFiles, err := s.repository.ListUnreferencedPhysicalFiles(ctx, cutoff, s.batch)
	if err != nil {
		return nil, err
	}
	result.DatabaseCandidates = len(physicalFiles)
	for _, physical := range physicalFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		storageKey, deleted, err := s.repository.DeletePhysicalFileIfUnreferenced(ctx, physical.ID)
		if err != nil {
			result.Failures++
			continue
		}
		if !deleted {
			continue
		}
		result.DatabaseDeleted++
		attemptedKeys[storageKey] = struct{}{}
		if err := s.objects.Delete(context.WithoutCancel(ctx), storageKey); err != nil {
			result.Failures++
		}
	}

	if err := s.scanStoredObjects(ctx, cutoff, attemptedKeys, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *OrphanCleanupService) scanStoredObjects(ctx context.Context, cutoff time.Time,
	attemptedKeys map[string]struct{}, result *OrphanCleanupResult) error {
	processed := 0
	inspect := func(object storage.ObjectInfo) {
		if !object.ModifiedAt.Before(cutoff) {
			return
		}
		if _, attempted := attemptedKeys[object.Key]; attempted {
			return
		}
		processed++
		result.StorageScanned++
		exists, err := s.repository.PhysicalStorageKeyExists(ctx, object.Key)
		if err != nil {
			result.Failures++
			return
		}
		if exists {
			return
		}
		if err := s.objects.Delete(context.WithoutCancel(ctx), object.Key); err != nil {
			result.Failures++
			return
		}
		result.StorageDeleted++
	}

	pager, ok := s.objects.(storage.Pager)
	if !ok {
		objects, err := s.objects.List(ctx, "objects")
		if err != nil {
			return fmt.Errorf("list stored objects for cleanup: %w", err)
		}
		for _, object := range objects {
			if processed >= s.batch {
				break
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			inspect(object)
		}
		return nil
	}

	pageSize := s.batch
	if pageSize > 1000 {
		pageSize = 1000
	}
	visited := 0
	cursor := s.scanCursor
	for {
		page, err := pager.ListPage(ctx, "objects", cursor, pageSize)
		if err != nil {
			return fmt.Errorf("list stored object page for cleanup: %w", err)
		}
		if len(page) == 0 {
			s.scanCursor = ""
			return nil
		}
		for _, object := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			if object.Key <= cursor {
				return fmt.Errorf("storage page did not advance after %q", cursor)
			}
			cursor = object.Key
			visited++
			inspect(object)
			if visited >= s.batch {
				s.scanCursor = cursor
				return nil
			}
		}
		if len(page) < pageSize {
			s.scanCursor = ""
			return nil
		}
	}
}

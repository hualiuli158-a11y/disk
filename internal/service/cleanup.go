package service

import (
	"context"
	"fmt"
	"time"

	"disk/internal/model"
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

	objects, err := s.objects.List(ctx, "objects")
	if err != nil {
		return nil, fmt.Errorf("list stored objects for cleanup: %w", err)
	}
	processed := 0
	for _, object := range objects {
		if processed >= s.batch {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !object.ModifiedAt.Before(cutoff) {
			continue
		}
		if _, attempted := attemptedKeys[object.Key]; attempted {
			continue
		}
		processed++
		result.StorageScanned++
		exists, err := s.repository.PhysicalStorageKeyExists(ctx, object.Key)
		if err != nil {
			result.Failures++
			continue
		}
		if exists {
			continue
		}
		if err := s.objects.Delete(context.WithoutCancel(ctx), object.Key); err != nil {
			result.Failures++
			continue
		}
		result.StorageDeleted++
	}

	return result, nil
}

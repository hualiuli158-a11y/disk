package service

import (
	"context"
	"errors"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
)

var (
	ErrTrashNotFound        = errors.New("回收站条目不存在")
	ErrTrashRestoreConflict = errors.New("恢复位置存在同名资源")
)

type TrashRepository interface {
	ListTrash(ctx context.Context, userID uint64, offset int, limit int) ([]model.TrashEntry, int64, error)
	ListExpiredTrashEntries(ctx context.Context, now time.Time, limit int) ([]model.TrashEntry, error)
	RestoreTrash(ctx context.Context, userID uint64, trashID uint64) error
	PermanentlyDeleteTrash(ctx context.Context, userID uint64, trashID uint64) ([]string, error)
}

type TrashService struct {
	trash   TrashRepository
	objects ObjectStorage
}

func NewTrashService(trash TrashRepository, objects ObjectStorage) *TrashService {
	return &TrashService{trash: trash, objects: objects}
}

type TrashItem struct {
	ID               uint64
	ResourceType     string
	ResourceID       uint64
	OriginalParentID *uint64
	Name             string
	TrashedAt        time.Time
	ExpiresAt        time.Time
}

type ListTrashOutput struct {
	Items      []TrashItem
	Page       int
	PageSize   int
	Total      int64
	TotalPages int64
}

func (s *TrashService) ListTrash(ctx context.Context, userID uint64, page int, pageSize int) (*ListTrashOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = DefaultContentPageSize
	}
	maxInt := int(^uint(0) >> 1)
	if page < 1 || pageSize < 1 || pageSize > MaxContentPageSize || page > maxInt/pageSize {
		return nil, ErrInvalidContentQuery
	}
	entries, total, err := s.trash.ListTrash(ctx, userID, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]TrashItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, TrashItem{
			ID: entry.ID, ResourceType: entry.ResourceType, ResourceID: entry.ResourceID,
			OriginalParentID: entry.OriginalParentID, Name: entry.Name,
			TrashedAt: entry.TrashedAt, ExpiresAt: entry.ExpiresAt,
		})
	}
	totalPages := int64(0)
	if total > 0 {
		totalPages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	return &ListTrashOutput{Items: items, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

func (s *TrashService) RestoreTrash(ctx context.Context, userID uint64, trashID uint64) error {
	if userID == 0 {
		return ErrFolderUserNotFound
	}
	if trashID == 0 {
		return ErrTrashNotFound
	}
	err := s.trash.RestoreTrash(ctx, userID, trashID)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return ErrFolderUserNotFound
	case errors.Is(err, repository.ErrTrashNotFound):
		return ErrTrashNotFound
	case errors.Is(err, repository.ErrTrashRestoreConflict):
		return ErrTrashRestoreConflict
	default:
		return err
	}
}

type PermanentDeleteOutput struct {
	CleanupFailures int
}

func (s *TrashService) PermanentlyDeleteTrash(ctx context.Context, userID uint64, trashID uint64) (*PermanentDeleteOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if trashID == 0 {
		return nil, ErrTrashNotFound
	}
	keys, err := s.trash.PermanentlyDeleteTrash(ctx, userID, trashID)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrFolderUserNotFound
	case errors.Is(err, repository.ErrTrashNotFound):
		return nil, ErrTrashNotFound
	case err != nil:
		return nil, err
	}

	output := &PermanentDeleteOutput{}
	for _, key := range keys {
		if err := s.objects.Delete(context.WithoutCancel(ctx), key); err != nil {
			output.CleanupFailures++
		}
	}
	return output, nil
}

type PurgeExpiredTrashResult struct {
	Candidates      int
	Deleted         int
	CleanupFailures int
	Failures        int
}

func (s *TrashService) PurgeExpiredTrash(ctx context.Context, now time.Time, limit int) (*PurgeExpiredTrashResult, error) {
	entries, err := s.trash.ListExpiredTrashEntries(ctx, now, limit)
	if err != nil {
		return nil, err
	}
	result := &PurgeExpiredTrashResult{Candidates: len(entries)}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		output, err := s.PermanentlyDeleteTrash(ctx, entry.UserID, entry.ID)
		if errors.Is(err, ErrTrashNotFound) {
			continue
		}
		if err != nil {
			result.Failures++
			continue
		}
		result.Deleted++
		result.CleanupFailures += output.CleanupFailures
	}
	return result, nil
}

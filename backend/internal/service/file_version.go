package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"disk/internal/repository"
)

var ErrFileVersionNotFound = errors.New("文件版本不存在")

type FileVersionItem struct {
	ID                    uint64
	Size                  int64
	SHA256                string
	Source                string
	RestoredFromVersionID *uint64
	CreatedAt             time.Time
	Current               bool
}

type FileVersionService struct {
	files   FileVersionRepository
	objects ObjectStorage
}

func NewFileVersionService(files FileVersionRepository, objects ObjectStorage) *FileVersionService {
	return &FileVersionService{files: files, objects: objects}
}

func (s *FileVersionService) ListFileVersions(ctx context.Context, userID uint64, fileID uint64) ([]FileVersionItem, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if fileID == 0 {
		return nil, ErrFileNotFound
	}
	versions, err := s.files.ListFileVersions(ctx, userID, fileID)
	switch {
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case err != nil:
		return nil, err
	}
	items := make([]FileVersionItem, 0, len(versions))
	for _, version := range versions {
		items = append(items, FileVersionItem{
			ID: version.ID, Size: version.PhysicalFile.Size, SHA256: version.PhysicalFile.SHA256,
			Source: version.Source, RestoredFromVersionID: version.RestoredFromVersionID,
			CreatedAt: version.CreatedAt, Current: version.Current,
		})
	}
	return items, nil
}

func (s *FileVersionService) DownloadFileVersion(ctx context.Context, userID uint64, fileID uint64, versionID uint64) (*DownloadFileOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if fileID == 0 {
		return nil, ErrFileNotFound
	}
	if versionID == 0 {
		return nil, ErrFileVersionNotFound
	}
	file, version, err := s.files.FindFileVersionForDownload(ctx, userID, fileID, versionID)
	switch {
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case errors.Is(err, repository.ErrFileVersionNotFound):
		return nil, ErrFileVersionNotFound
	case err != nil:
		return nil, err
	}
	if file == nil || version == nil || version.PhysicalFile.StorageKey == "" {
		return nil, ErrFileVersionNotFound
	}
	content, err := s.objects.Open(ctx, version.PhysicalFile.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("open file version %d: %w", versionID, err)
	}
	return &DownloadFileOutput{
		Name: file.Name, Size: version.PhysicalFile.Size, SHA256: version.PhysicalFile.SHA256,
		UpdatedAt: version.CreatedAt, Content: content,
	}, nil
}

func (s *FileVersionService) RestoreFileVersion(ctx context.Context, userID uint64, fileID uint64, versionID uint64, expectedSHA256 string) (*UpdateFileContentOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if fileID == 0 {
		return nil, ErrFileNotFound
	}
	if versionID == 0 {
		return nil, ErrFileVersionNotFound
	}
	expected, err := normalizeSHA256(expectedSHA256)
	if err != nil {
		return nil, err
	}
	file, version, err := s.files.RestoreFileVersion(ctx, userID, fileID, versionID, expected)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrFolderUserNotFound
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case errors.Is(err, repository.ErrFileVersionNotFound):
		return nil, ErrFileVersionNotFound
	case errors.Is(err, repository.ErrFileVersionConflict):
		return nil, ErrFileVersionConflict
	case errors.Is(err, repository.ErrQuotaExceeded):
		return nil, ErrQuotaExceeded
	case err != nil:
		return nil, err
	}
	if file == nil || version == nil {
		return nil, ErrFileVersionNotFound
	}
	return &UpdateFileContentOutput{
		ID: file.ID, Name: file.Name, Size: file.PhysicalFile.Size,
		SHA256: file.PhysicalFile.SHA256, UpdatedAt: file.UpdatedAt,
	}, nil
}

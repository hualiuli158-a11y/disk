package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
	"disk/internal/storage"
)

type fileVersionRepositoryStub struct {
	versions  []model.FileVersion
	file      *model.File
	version   *model.FileVersion
	err       error
	expected  string
	versionID uint64
}

func (s *fileVersionRepositoryStub) ListFileVersions(context.Context, uint64, uint64) ([]model.FileVersion, error) {
	return s.versions, s.err
}
func (s *fileVersionRepositoryStub) FindFileVersionForDownload(context.Context, uint64, uint64, uint64) (*model.File, *model.FileVersion, error) {
	return s.file, s.version, s.err
}
func (s *fileVersionRepositoryStub) RestoreFileVersion(_ context.Context, _ uint64, _ uint64, versionID uint64, expected string) (*model.File, *model.FileVersion, error) {
	s.versionID, s.expected = versionID, expected
	return s.file, s.version, s.err
}

type fileVersionStorageStub struct{ content string }

func (*fileVersionStorageStub) Save(context.Context, string, io.Reader) error {
	return errors.New("not implemented")
}
func (s *fileVersionStorageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.content)), nil
}
func (*fileVersionStorageStub) Delete(context.Context, string) error {
	return errors.New("not implemented")
}
func (*fileVersionStorageStub) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return nil, nil
}

func TestFileVersionServiceListFileVersionsMarksNewestCurrent(t *testing.T) {
	repositoryStub := &fileVersionRepositoryStub{versions: []model.FileVersion{
		{ID: 2, Current: true, Source: "edit", PhysicalFile: model.PhysicalFile{Size: 4, SHA256: strings.Repeat("b", 64)}},
		{ID: 1, Source: "upload", PhysicalFile: model.PhysicalFile{Size: 3, SHA256: strings.Repeat("a", 64)}},
	}}
	service := NewFileVersionService(repositoryStub, &fileVersionStorageStub{})
	items, err := service.ListFileVersions(context.Background(), 7, 9)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(items) != 2 || !items[0].Current || items[1].Current || items[0].ID != 2 {
		t.Fatalf("unexpected versions: %+v", items)
	}
}

func TestFileVersionServiceAutosavedContentHasNoCurrentSnapshot(t *testing.T) {
	repositoryStub := &fileVersionRepositoryStub{versions: []model.FileVersion{{ID: 1, Source: "upload", Current: false}}}
	service := NewFileVersionService(repositoryStub, &fileVersionStorageStub{})
	items, err := service.ListFileVersions(context.Background(), 7, 9)
	if err != nil || len(items) != 1 || items[0].Current {
		t.Fatalf("unexpected snapshot state: %+v, %v", items, err)
	}
}

func TestFileVersionServiceDownloadFileVersionReadsHistoricalObject(t *testing.T) {
	createdAt := time.Date(2026, time.October, 9, 10, 0, 0, 0, time.UTC)
	version := &model.FileVersion{ID: 3, CreatedAt: createdAt, PhysicalFile: model.PhysicalFile{Size: 7, SHA256: strings.Repeat("c", 64), StorageKey: "objects/cc/version"}}
	repositoryStub := &fileVersionRepositoryStub{file: &model.File{Name: "note.md"}, version: version}
	service := NewFileVersionService(repositoryStub, &fileVersionStorageStub{content: "history"})
	result, err := service.DownloadFileVersion(context.Background(), 7, 9, 3)
	if err != nil {
		t.Fatalf("download version: %v", err)
	}
	defer result.Content.Close()
	content, _ := io.ReadAll(result.Content)
	if string(content) != "history" || result.Name != "note.md" || !result.UpdatedAt.Equal(createdAt) {
		t.Fatalf("unexpected download: %+v content=%q", result, content)
	}
}

func TestFileVersionServiceRestoreFileVersionUsesExpectedSHA(t *testing.T) {
	sha := strings.Repeat("d", 64)
	physical := model.PhysicalFile{Size: 5, SHA256: strings.Repeat("e", 64)}
	version := &model.FileVersion{ID: 4, PhysicalFile: physical}
	file := &model.File{ID: 9, Name: "note.md", PhysicalFile: physical, UpdatedAt: time.Now()}
	repositoryStub := &fileVersionRepositoryStub{file: file, version: version}
	service := NewFileVersionService(repositoryStub, &fileVersionStorageStub{})
	result, err := service.RestoreFileVersion(context.Background(), 7, 9, 4, sha)
	if err != nil {
		t.Fatalf("restore version: %v", err)
	}
	if repositoryStub.expected != sha || repositoryStub.versionID != 4 || result.SHA256 != physical.SHA256 {
		t.Fatalf("unexpected restore: stub=%+v result=%+v", repositoryStub, result)
	}
}

func TestFileVersionServiceDownloadFileVersionMapsMissingVersion(t *testing.T) {
	repositoryStub := &fileVersionRepositoryStub{err: repository.ErrFileVersionNotFound}
	service := NewFileVersionService(repositoryStub, &fileVersionStorageStub{})
	_, err := service.DownloadFileVersion(context.Background(), 7, 9, 99)
	if !errors.Is(err, ErrFileVersionNotFound) {
		t.Fatalf("error=%v, want ErrFileVersionNotFound", err)
	}
}

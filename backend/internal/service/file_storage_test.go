package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"disk/internal/model"
	"disk/internal/storage"
)

type rejectedStorageObject struct{}

func (rejectedStorageObject) Save(context.Context, string, io.Reader) error {
	return storage.ErrTooLarge
}
func (rejectedStorageObject) Open(context.Context, string) (io.ReadCloser, error) {
	panic("Open must not run during upload")
}
func (rejectedStorageObject) Delete(context.Context, string) error {
	panic("Delete must not run before Save succeeds")
}
func (rejectedStorageObject) List(context.Context, string) ([]storage.ObjectInfo, error) {
	panic("List must not run during upload")
}

type rejectedUploadRepository struct{}

func (rejectedUploadRepository) CreateUploadedFile(context.Context, uint64, uint64,
	string, string, int64, string) (*model.File, *model.PhysicalFile, bool, error) {
	panic("database must not run after rejected Save")
}

func TestUploadFileServiceUploadFileMapsStorageTooLarge(t *testing.T) {
	service := NewUploadFileService(rejectedUploadRepository{}, rejectedStorageObject{}, DefaultMaxUploadSize)
	_, err := service.UploadFile(context.Background(), UploadFileInput{
		UserID: 1, FolderID: 1, Name: "file.bin", Content: strings.NewReader("data"),
	})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("storage size rejection became %v, want ErrFileTooLarge", err)
	}
}

type recordedUploadStorage struct {
	handlerData []byte
	deleted     bool
}

func (s *recordedUploadStorage) Save(_ context.Context, _ string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	s.handlerData = data
	return err
}
func (*recordedUploadStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *recordedUploadStorage) Delete(context.Context, string) error {
	s.deleted = true
	return nil
}
func (*recordedUploadStorage) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return nil, nil
}

func TestUploadFileServiceUploadFileRejectsDeclaredSizeMismatch(t *testing.T) {
	objects := &recordedUploadStorage{}
	service := NewUploadFileService(rejectedUploadRepository{}, objects, DefaultMaxUploadSize)
	_, err := service.UploadFile(context.Background(), UploadFileInput{
		UserID: 1, FolderID: 1, Name: "file.bin", Content: strings.NewReader("data"),
		DeclaredSize: 5, HasDeclaredSize: true,
	})
	if !errors.Is(err, ErrUploadSizeMismatch) {
		t.Fatalf("error=%v, want ErrUploadSizeMismatch", err)
	}
	if !objects.deleted {
		t.Fatal("size-mismatched candidate was not deleted")
	}
}

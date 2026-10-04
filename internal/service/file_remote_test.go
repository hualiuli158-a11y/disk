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

type rejectedRemoteObject struct{}

func (rejectedRemoteObject) Save(context.Context, string, io.Reader) error {
	return storage.ErrTooLarge
}
func (rejectedRemoteObject) Open(context.Context, string) (io.ReadCloser, error) {
	panic("Open must not run during upload")
}
func (rejectedRemoteObject) Delete(context.Context, string) error {
	panic("Delete must not run before Save succeeds")
}
func (rejectedRemoteObject) List(context.Context, string) ([]storage.ObjectInfo, error) {
	panic("List must not run during upload")
}

type rejectedUploadRepository struct{}

func (rejectedUploadRepository) CreateUploadedFile(context.Context, uint64, uint64,
	string, string, int64, string) (*model.File, *model.PhysicalFile, bool, error) {
	panic("database must not run after rejected Save")
}

func TestUploadMapsRemoteTooLarge(t *testing.T) {
	service := NewUploadFileService(rejectedUploadRepository{}, rejectedRemoteObject{}, DefaultMaxUploadSize)
	_, err := service.UploadFile(context.Background(), UploadFileInput{
		UserID: 1, FolderID: 1, Name: "file.bin", Content: strings.NewReader("data"),
	})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("remote size rejection became %v, want ErrFileTooLarge", err)
	}
}

package service

import (
	"bytes"
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

type updateContentRepositoryStub struct {
	createVersion      bool
	err                error
	physicalCreated    bool
	receivedUserID     uint64
	receivedFileID     uint64
	receivedExpected   string
	receivedSHA256     string
	receivedSize       int64
	receivedStorageKey string
	updatedAt          time.Time
}

func (s *updateContentRepositoryStub) ReplaceFileContent(
	_ context.Context,
	userID uint64,
	fileID uint64,
	expectedSHA256 string,
	sha256Value string,
	size int64,
	storageKey string,
	createVersion bool,
) (*model.File, *model.PhysicalFile, bool, error) {
	s.receivedUserID = userID
	s.receivedFileID = fileID
	s.receivedExpected = expectedSHA256
	s.receivedSHA256 = sha256Value
	s.receivedSize = size
	s.createVersion = createVersion
	s.receivedStorageKey = storageKey
	if s.err != nil {
		return nil, nil, false, s.err
	}
	physical := &model.PhysicalFile{ID: 9, SHA256: sha256Value, Size: size, StorageKey: storageKey}
	file := &model.File{ID: fileID, UserID: userID, FolderID: 7, PhysicalFileID: physical.ID, Name: "note.md", UpdatedAt: s.updatedAt}
	return file, physical, s.physicalCreated, nil
}

type updateContentStorageStub struct {
	savedKey    string
	savedData   []byte
	deletedKeys []string
	saveErr     error
}

func (s *updateContentStorageStub) Save(_ context.Context, key string, reader io.Reader) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.savedKey = key
	s.savedData = data
	return nil
}

func (*updateContentStorageStub) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *updateContentStorageStub) Delete(_ context.Context, key string) error {
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
}

func (*updateContentStorageStub) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return nil, nil
}

func TestUpdateFileContentServiceUpdateFileContentStoresNewVersion(t *testing.T) {
	const expectedSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const nextSHA = "c6c1c9a9c8543f1e4cd980064cf1625eeb61a90703b2464fff039f21682508b3"
	updatedAt := time.Date(2026, time.October, 8, 14, 0, 0, 0, time.UTC)
	repositoryStub := &updateContentRepositoryStub{physicalCreated: true, updatedAt: updatedAt}
	objects := &updateContentStorageStub{}
	files := NewUpdateFileContentService(repositoryStub, objects, DefaultMaxEditableTextSize)

	result, err := files.UpdateFileContent(context.Background(), UpdateFileContentInput{
		UserID: 42, FileID: 5, ExpectedSHA256: expectedSHA, Content: strings.NewReader("next"),
	})
	if err != nil {
		t.Fatalf("update content: %v", err)
	}
	if result.ID != 5 || result.Name != "note.md" || result.Size != 4 || result.SHA256 != nextSHA || !result.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repositoryStub.receivedUserID != 42 || repositoryStub.receivedFileID != 5 || repositoryStub.receivedExpected != expectedSHA {
		t.Fatalf("unexpected repository identity/version: %+v", repositoryStub)
	}
	if repositoryStub.receivedSHA256 != nextSHA || repositoryStub.receivedSize != 4 || repositoryStub.receivedStorageKey == "" {
		t.Fatalf("unexpected candidate metadata: %+v", repositoryStub)
	}
	if !bytes.Equal(objects.savedData, []byte("next")) || objects.savedKey != repositoryStub.receivedStorageKey {
		t.Fatalf("saved candidate mismatch: key=%q data=%q", objects.savedKey, objects.savedData)
	}
	if len(objects.deletedKeys) != 0 {
		t.Fatalf("adopted object was deleted: %v", objects.deletedKeys)
	}
}

func TestUpdateFileContentServiceUpdateFileContentRejectsStaleVersionAndCleansCandidate(t *testing.T) {
	repositoryStub := &updateContentRepositoryStub{err: repository.ErrFileVersionConflict}
	objects := &updateContentStorageStub{}
	files := NewUpdateFileContentService(repositoryStub, objects, DefaultMaxEditableTextSize)

	_, err := files.UpdateFileContent(context.Background(), UpdateFileContentInput{
		UserID:         42,
		FileID:         5,
		ExpectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Content:        strings.NewReader("next"),
	})
	if !errors.Is(err, ErrFileVersionConflict) {
		t.Fatalf("stale update error = %v, want ErrFileVersionConflict", err)
	}
	if len(objects.deletedKeys) != 1 || objects.deletedKeys[0] != objects.savedKey {
		t.Fatalf("stale candidate cleanup = %v, saved=%q", objects.deletedKeys, objects.savedKey)
	}
}

func TestUpdateFileContentServiceUpdateFileContentCleansDeduplicatedCandidate(t *testing.T) {
	repositoryStub := &updateContentRepositoryStub{physicalCreated: false, updatedAt: time.Now()}
	objects := &updateContentStorageStub{}
	files := NewUpdateFileContentService(repositoryStub, objects, DefaultMaxEditableTextSize)

	_, err := files.UpdateFileContent(context.Background(), UpdateFileContentInput{
		UserID:         42,
		FileID:         5,
		ExpectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Content:        strings.NewReader("next"),
	})
	if err != nil {
		t.Fatalf("update content: %v", err)
	}
	if len(objects.deletedKeys) != 1 || objects.deletedKeys[0] != objects.savedKey {
		t.Fatalf("deduplicated candidate cleanup = %v, saved=%q", objects.deletedKeys, objects.savedKey)
	}
}

func TestUpdateFileContentServiceUpdateFileContentRejectsInvalidOrOversizedText(t *testing.T) {
	tests := []struct {
		name           string
		content        []byte
		expectedSHA256 string
		wantErr        error
	}{
		{name: "invalid version", content: []byte("ok"), expectedSHA256: "short", wantErr: ErrInvalidFileVersion},
		{name: "invalid utf8", content: []byte{0xff, 0xfe, 0xfd}, expectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: ErrInvalidTextContent},
		{name: "too large", content: []byte("12345"), expectedSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantErr: ErrFileContentTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repositoryStub := &updateContentRepositoryStub{}
			objects := &updateContentStorageStub{}
			maxSize := int64(1024)
			if tt.name == "too large" {
				maxSize = 4
			}
			files := NewUpdateFileContentService(repositoryStub, objects, maxSize)
			_, err := files.UpdateFileContent(context.Background(), UpdateFileContentInput{
				UserID:         42,
				FileID:         5,
				ExpectedSHA256: tt.expectedSHA256,
				Content:        bytes.NewReader(tt.content),
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if objects.savedKey != "" {
				t.Fatalf("invalid content reached storage: %q", objects.savedKey)
			}
		})
	}
}

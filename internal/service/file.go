package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"disk/internal/repository"
)

const DefaultMaxUploadSize int64 = 1 << 30 // 1 GiB

var (
	ErrInvalidFileName      = errors.New("文件名不合法")
	ErrUploadFolderNotFound = errors.New("上传目录不存在")
	ErrFileAlreadyExists    = errors.New("同名文件已存在")
	ErrFileTooLarge         = errors.New("文件超过上传大小限制")
	ErrFileFolderNotFound   = errors.New("目录不存在")
	ErrFileNotFound         = errors.New("文件不存在")
)

type UploadFileService struct {
	files         UploadFileRepository
	objects       ObjectStorage
	maxUploadSize int64
}

func NewUploadFileService(files UploadFileRepository, objects ObjectStorage, maxUploadSize int64) *UploadFileService {
	if maxUploadSize <= 0 {
		maxUploadSize = DefaultMaxUploadSize
	}
	return &UploadFileService{files: files, objects: objects, maxUploadSize: maxUploadSize}
}

type UploadFileInput struct {
	UserID   uint64
	FolderID uint64
	Name     string
	Content  io.Reader
}

type UploadFileOutput struct {
	ID        uint64
	FolderID  uint64
	Name      string
	Size      int64
	SHA256    string
	CreatedAt time.Time
}

func (s *UploadFileService) UploadFile(ctx context.Context, input UploadFileInput) (*UploadFileOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FolderID == 0 {
		return nil, ErrUploadFolderNotFound
	}
	name, err := normalizeFileName(input.Name)
	if err != nil {
		return nil, err
	}
	if input.Content == nil {
		return nil, ErrInvalidFileName
	}

	storageKey, err := newStorageKey()
	if err != nil {
		return nil, fmt.Errorf("generate storage key: %w", err)
	}

	hasher := sha256.New()
	limited := &io.LimitedReader{R: input.Content, N: s.maxUploadSize + 1}
	counter := &countingHashReader{reader: limited, hasher: hasher}
	if err := s.objects.Save(ctx, storageKey, counter); err != nil {
		return nil, fmt.Errorf("save upload object: %w", err)
	}
	if counter.size > s.maxUploadSize {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
		return nil, ErrFileTooLarge
	}

	sha256Value := hex.EncodeToString(hasher.Sum(nil))
	logical, physical, physicalCreated, err := s.files.CreateUploadedFile(
		ctx,
		input.UserID,
		input.FolderID,
		name,
		sha256Value,
		counter.size,
		storageKey,
	)
	if err != nil {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
		switch {
		case errors.Is(err, repository.ErrUserNotFound):
			return nil, ErrFolderUserNotFound
		case errors.Is(err, repository.ErrFileFolderNotFound):
			return nil, ErrUploadFolderNotFound
		case errors.Is(err, repository.ErrFileAlreadyExists):
			return nil, ErrFileAlreadyExists
		default:
			return nil, err
		}
	}
	if !physicalCreated {
		// 数据库复用了已有物理文件，候选对象已无引用；清理失败不应把已提交的上传报告为失败。
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
	}

	return &UploadFileOutput{
		ID:        logical.ID,
		FolderID:  logical.FolderID,
		Name:      logical.Name,
		Size:      physical.Size,
		SHA256:    physical.SHA256,
		CreatedAt: logical.CreatedAt,
	}, nil
}

func normalizeFileName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" ||
		name == "." ||
		name == ".." ||
		strings.ContainsAny(name, `/\`) ||
		strings.ContainsRune(name, '\x00') ||
		utf8.RuneCountInString(name) > 255 {
		return "", ErrInvalidFileName
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return "", ErrInvalidFileName
		}
	}
	return name, nil
}

func newStorageKey() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	value := hex.EncodeToString(random)
	return "objects/" + value[:2] + "/" + value, nil
}

type countingHashReader struct {
	reader io.Reader
	hasher hash.Hash
	size   int64
}

type ListFilesService struct {
	files ListFilesRepository
}

func NewListFilesService(files ListFilesRepository) *ListFilesService {
	return &ListFilesService{files: files}
}

type FileListItem struct {
	ID        uint64
	FolderID  uint64
	Name      string
	Size      int64
	SHA256    string
	CreatedAt time.Time
}

type ListFilesOutput struct {
	FolderID uint64
	Files    []FileListItem
}

func (s *ListFilesService) ListFiles(ctx context.Context, userID uint64, folderID uint64) (*ListFilesOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if folderID == 0 {
		return nil, ErrFileFolderNotFound
	}

	files, err := s.files.ListFiles(ctx, userID, folderID)
	switch {
	case errors.Is(err, repository.ErrFileFolderNotFound):
		return nil, ErrFileFolderNotFound
	case err != nil:
		return nil, err
	}

	items := make([]FileListItem, 0, len(files))
	for _, file := range files {
		items = append(items, FileListItem{
			ID:        file.ID,
			FolderID:  file.FolderID,
			Name:      file.Name,
			Size:      file.PhysicalFile.Size,
			SHA256:    file.PhysicalFile.SHA256,
			CreatedAt: file.CreatedAt,
		})
	}

	return &ListFilesOutput{FolderID: folderID, Files: items}, nil
}

type DownloadFileService struct {
	files   DownloadFileRepository
	objects ObjectStorage
}

func NewDownloadFileService(files DownloadFileRepository, objects ObjectStorage) *DownloadFileService {
	return &DownloadFileService{files: files, objects: objects}
}

type DownloadFileOutput struct {
	Name    string
	Size    int64
	Content io.ReadCloser
}

func (s *DownloadFileService) DownloadFile(ctx context.Context, userID uint64, fileID uint64) (*DownloadFileOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if fileID == 0 {
		return nil, ErrFileNotFound
	}

	file, err := s.files.FindFileForDownload(ctx, userID, fileID)
	switch {
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case err != nil:
		return nil, err
	}
	if file == nil || file.PhysicalFile.StorageKey == "" {
		return nil, ErrFileNotFound
	}

	content, err := s.objects.Open(ctx, file.PhysicalFile.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("open physical file %d: %w", file.PhysicalFileID, err)
	}

	return &DownloadFileOutput{
		Name:    file.Name,
		Size:    file.PhysicalFile.Size,
		Content: content,
	}, nil
}

type DeleteFileService struct {
	files   DeleteFileRepository
	objects ObjectStorage
}

func NewDeleteFileService(files DeleteFileRepository, objects ObjectStorage) *DeleteFileService {
	return &DeleteFileService{files: files, objects: objects}
}

type DeleteFileOutput struct {
	CleanupError error
}

func (s *DeleteFileService) DeleteFile(ctx context.Context, userID uint64, fileID uint64) (*DeleteFileOutput, error) {
	if userID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if fileID == 0 {
		return nil, ErrFileNotFound
	}

	storageKey, deletePhysical, err := s.files.DeleteFile(ctx, userID, fileID)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrFolderUserNotFound
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case err != nil:
		return nil, err
	}

	output := &DeleteFileOutput{}
	if deletePhysical && storageKey != "" {
		// 数据库已经提交；存储清理失败时保留成功语义，交由日志和后续清理任务处理孤儿对象。
		output.CleanupError = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
	}
	return output, nil
}

type RenameFileService struct {
	files RenameFileRepository
}

func NewRenameFileService(files RenameFileRepository) *RenameFileService {
	return &RenameFileService{files: files}
}

type RenameFileInput struct {
	UserID uint64
	FileID uint64
	Name   string
}

type RenameFileOutput struct {
	ID   uint64
	Name string
}

func (s *RenameFileService) RenameFile(ctx context.Context, input RenameFileInput) (*RenameFileOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FileID == 0 {
		return nil, ErrFileNotFound
	}
	name, err := normalizeFileName(input.Name)
	if err != nil {
		return nil, err
	}

	err = s.files.RenameFile(ctx, input.UserID, input.FileID, name)
	switch {
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case errors.Is(err, repository.ErrFileAlreadyExists):
		return nil, ErrFileAlreadyExists
	case err != nil:
		return nil, err
	}

	return &RenameFileOutput{ID: input.FileID, Name: name}, nil
}

func (r *countingHashReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		_, _ = r.hasher.Write(buffer[:n])
		r.size += int64(n)
	}
	return n, err
}

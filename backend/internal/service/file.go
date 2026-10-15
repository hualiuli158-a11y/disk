package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"disk/internal/repository"
	"disk/internal/storage"
)

const (
	DefaultMaxUploadSize       int64 = 1 << 30         // 1 GiB
	DefaultMaxEditableTextSize int64 = 5 * 1024 * 1024 // 5 MiB
)

var (
	ErrInvalidFileName      = errors.New("文件名不合法")
	ErrUploadFolderNotFound = errors.New("上传目录不存在")
	ErrFileAlreadyExists    = errors.New("同名文件已存在")
	ErrFileTooLarge         = errors.New("文件超过上传大小限制")
	ErrFileFolderNotFound   = errors.New("目录不存在")
	ErrFileNotFound         = errors.New("文件不存在")
	ErrQuotaExceeded        = errors.New("存储额度不足")
	ErrInvalidTextContent   = errors.New("文件内容不是有效的 UTF-8 文本")
	ErrFileContentTooLarge  = errors.New("文件超过在线编辑大小限制")
	ErrInvalidFileVersion   = errors.New("文件版本无效")
	ErrFileVersionConflict  = errors.New("文件已被其他操作修改")
	ErrUploadSizeMismatch   = errors.New("上传内容大小与声明不一致")
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
	UserID          uint64
	FolderID        uint64
	Name            string
	Content         io.Reader
	DeclaredSize    int64
	HasDeclaredSize bool
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
		var maxBytesError *http.MaxBytesError
		if errors.Is(err, storage.ErrTooLarge) || errors.As(err, &maxBytesError) {
			return nil, ErrFileTooLarge
		}
		return nil, fmt.Errorf("save upload object: %w", err)
	}
	if counter.size > s.maxUploadSize {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
		return nil, ErrFileTooLarge
	}
	if input.HasDeclaredSize && counter.size != input.DeclaredSize {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
		return nil, ErrUploadSizeMismatch
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
		case errors.Is(err, repository.ErrQuotaExceeded):
			return nil, ErrQuotaExceeded
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

type UpdateFileContentService struct {
	files       UpdateFileContentRepository
	objects     ObjectStorage
	maxTextSize int64
}

func NewUpdateFileContentService(files UpdateFileContentRepository, objects ObjectStorage, maxTextSize int64) *UpdateFileContentService {
	if maxTextSize <= 0 {
		maxTextSize = DefaultMaxEditableTextSize
	}
	return &UpdateFileContentService{files: files, objects: objects, maxTextSize: maxTextSize}
}

type UpdateFileContentInput struct {
	CreateVersion  bool
	UserID         uint64
	FileID         uint64
	ExpectedSHA256 string
	Content        io.Reader
}

type UpdateFileContentOutput struct {
	ID        uint64
	Name      string
	Size      int64
	SHA256    string
	UpdatedAt time.Time
}

func (s *UpdateFileContentService) UpdateFileContent(ctx context.Context, input UpdateFileContentInput) (*UpdateFileContentOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FileID == 0 {
		return nil, ErrFileNotFound
	}
	expectedSHA256, err := normalizeSHA256(input.ExpectedSHA256)
	if err != nil {
		return nil, err
	}
	if input.Content == nil {
		return nil, ErrInvalidTextContent
	}

	content, err := io.ReadAll(io.LimitReader(input.Content, s.maxTextSize+1))
	if err != nil {
		return nil, fmt.Errorf("read updated file content: %w", err)
	}
	if int64(len(content)) > s.maxTextSize {
		return nil, ErrFileContentTooLarge
	}
	if !utf8.Valid(content) {
		return nil, ErrInvalidTextContent
	}

	digest := sha256.Sum256(content)
	sha256Value := hex.EncodeToString(digest[:])
	storageKey, err := newStorageKey()
	if err != nil {
		return nil, fmt.Errorf("generate storage key: %w", err)
	}
	if err := s.objects.Save(ctx, storageKey, bytes.NewReader(content)); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return nil, ErrFileContentTooLarge
		}
		return nil, fmt.Errorf("save updated file object: %w", err)
	}

	logical, physical, physicalCreated, err := s.files.ReplaceFileContent(
		ctx,
		input.UserID,
		input.FileID,
		expectedSHA256,
		sha256Value,
		int64(len(content)),
		storageKey,
		input.CreateVersion,
	)
	if err != nil {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
		switch {
		case errors.Is(err, repository.ErrUserNotFound):
			return nil, ErrFolderUserNotFound
		case errors.Is(err, repository.ErrFileNotFound):
			return nil, ErrFileNotFound
		case errors.Is(err, repository.ErrFileVersionConflict):
			return nil, ErrFileVersionConflict
		case errors.Is(err, repository.ErrQuotaExceeded):
			return nil, ErrQuotaExceeded
		default:
			return nil, err
		}
	}
	if logical == nil || physical == nil {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
		return nil, errors.New("updated file metadata is missing")
	}
	if !physicalCreated {
		_ = s.objects.Delete(context.WithoutCancel(ctx), storageKey)
	}

	return &UpdateFileContentOutput{
		ID:        logical.ID,
		Name:      logical.Name,
		Size:      physical.Size,
		SHA256:    physical.SHA256,
		UpdatedAt: logical.UpdatedAt,
	}, nil
}

func normalizeSHA256(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) != sha256.Size*2 {
		return "", ErrInvalidFileVersion
	}
	if _, err := hex.DecodeString(normalized); err != nil {
		return "", ErrInvalidFileVersion
	}
	return normalized, nil
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
	Name      string
	Size      int64
	SHA256    string
	UpdatedAt time.Time
	Content   io.ReadCloser
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
		Name:      file.Name,
		Size:      file.PhysicalFile.Size,
		SHA256:    file.PhysicalFile.SHA256,
		UpdatedAt: file.UpdatedAt,
		Content:   content,
	}, nil
}

type DeleteFileService struct {
	files DeleteFileRepository
	now   func() time.Time
}

func NewDeleteFileService(files DeleteFileRepository) *DeleteFileService {
	return &DeleteFileService{files: files, now: time.Now}
}

func (s *DeleteFileService) DeleteFile(ctx context.Context, userID uint64, fileID uint64) error {
	if userID == 0 {
		return ErrFolderUserNotFound
	}
	if fileID == 0 {
		return ErrFileNotFound
	}

	now := s.now()
	err := s.files.TrashFile(ctx, userID, fileID, now, now.Add(DefaultTrashRetention))
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return ErrFolderUserNotFound
	case errors.Is(err, repository.ErrFileNotFound):
		return ErrFileNotFound
	case err != nil:
		return err
	default:
		return nil
	}
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

type MoveFileService struct {
	files MoveFileRepository
}

func NewMoveFileService(files MoveFileRepository) *MoveFileService {
	return &MoveFileService{files: files}
}

type MoveFileInput struct {
	UserID   uint64
	FileID   uint64
	FolderID uint64
}

type MoveFileOutput struct {
	ID       uint64
	FolderID uint64
}

func (s *MoveFileService) MoveFile(ctx context.Context, input MoveFileInput) (*MoveFileOutput, error) {
	if input.UserID == 0 {
		return nil, ErrFolderUserNotFound
	}
	if input.FileID == 0 {
		return nil, ErrFileNotFound
	}
	if input.FolderID == 0 {
		return nil, ErrFileFolderNotFound
	}

	err := s.files.MoveFile(ctx, input.UserID, input.FileID, input.FolderID)
	switch {
	case errors.Is(err, repository.ErrUserNotFound):
		return nil, ErrFolderUserNotFound
	case errors.Is(err, repository.ErrFileNotFound):
		return nil, ErrFileNotFound
	case errors.Is(err, repository.ErrFileFolderNotFound):
		return nil, ErrFileFolderNotFound
	case errors.Is(err, repository.ErrFileAlreadyExists):
		return nil, ErrFileAlreadyExists
	case err != nil:
		return nil, err
	}

	return &MoveFileOutput{ID: input.FileID, FolderID: input.FolderID}, nil
}

func (r *countingHashReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		_, _ = r.hasher.Write(buffer[:n])
		r.size += int64(n)
	}
	return n, err
}

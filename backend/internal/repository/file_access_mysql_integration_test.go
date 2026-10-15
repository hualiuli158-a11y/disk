package repository

import (
	"context"
	"errors"
	"testing"

	"disk/internal/model"
)

func TestFilesRepositoryListFilesDirectChildrenAndOwnership(t *testing.T) {
	fx := newFilesTestFixture(t)
	rootFile, rootPhysical, _ := mustUploadFilesTestFile(t, fx, "root.txt", filesTestSHA(201), 10)
	trashed, _, _ := mustUploadFilesTestFile(t, fx, "trashed.txt", filesTestSHA(202), 5)
	if err := fx.tx.Delete(&model.File{}, trashed.ID).Error; err != nil {
		t.Fatalf("soft delete fixture file: %v", err)
	}
	child := &model.Folder{UserID: fx.userID, ParentID: &fx.folderID, Name: "child"}
	if err := fx.tx.Create(child).Error; err != nil {
		t.Fatalf("create child folder: %v", err)
	}
	childFile, _, _, err := fx.repo.CreateUploadedFile(
		context.Background(), fx.userID, child.ID, "child.txt", filesTestSHA(203), 3, "objects/test/files/child",
	)
	if err != nil {
		t.Fatalf("upload child file: %v", err)
	}
	other, otherFolder := createFilesTestUser(t, fx.tx, 100)
	if _, _, _, err := fx.repo.CreateUploadedFile(
		context.Background(), other.ID, otherFolder.ID, "other.txt", filesTestSHA(204), 4, "objects/test/files/other",
	); err != nil {
		t.Fatalf("upload other user's file: %v", err)
	}

	files, err := fx.repo.ListFiles(context.Background(), fx.userID, fx.folderID)
	if err != nil || len(files) != 1 || files[0].ID != rootFile.ID ||
		files[0].PhysicalFile.ID != rootPhysical.ID || files[0].PhysicalFile.Size != rootPhysical.Size ||
		files[0].PhysicalFile.StorageKey != rootPhysical.StorageKey {
		t.Fatalf("direct files: files=%+v, err=%v", files, err)
	}
	childFiles, err := fx.repo.ListFiles(context.Background(), fx.userID, child.ID)
	if err != nil || len(childFiles) != 1 || childFiles[0].ID != childFile.ID {
		t.Fatalf("child files: files=%+v, err=%v", childFiles, err)
	}
	if _, err := fx.repo.ListFiles(context.Background(), fx.userID, otherFolder.ID); !errors.Is(err, ErrFileFolderNotFound) {
		t.Fatalf("other user's folder: got %v, want ErrFileFolderNotFound", err)
	}
}

func TestFilesRepositoryFindFileForDownloadOwnershipAndSoftDelete(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "download.txt", filesTestSHA(205), 10)
	got, err := fx.repo.FindFileForDownload(context.Background(), fx.userID, file.ID)
	if err != nil || got == nil || got.ID != file.ID || got.PhysicalFile.ID != physical.ID ||
		got.PhysicalFile.SHA256 != physical.SHA256 || got.PhysicalFile.Size != physical.Size ||
		got.PhysicalFile.StorageKey != physical.StorageKey {
		t.Fatalf("download file: file=%+v, err=%v", got, err)
	}
	other, _ := createFilesTestUser(t, fx.tx, 100)
	if _, err := fx.repo.FindFileForDownload(context.Background(), other.ID, file.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("other user's download: got %v, want ErrFileNotFound", err)
	}
	if err := fx.tx.Delete(&model.File{}, file.ID).Error; err != nil {
		t.Fatalf("soft delete fixture file: %v", err)
	}
	if _, err := fx.repo.FindFileForDownload(context.Background(), fx.userID, file.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("soft-deleted download: got %v, want ErrFileNotFound", err)
	}
}

func TestFilesRepositoryRenameFileSuccessAndValidation(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, _, _ := mustUploadFilesTestFile(t, fx, "before.txt", filesTestSHA(206), 10)
	mustUploadFilesTestFile(t, fx, "taken.txt", filesTestSHA(207), 5)
	ctx := context.Background()
	if err := fx.repo.RenameFile(ctx, fx.userID, file.ID, "after.txt"); err != nil {
		t.Fatalf("rename file: %v", err)
	}
	if got := filesTestSavedFile(t, fx.tx, file.ID).Name; got != "after.txt" {
		t.Fatalf("renamed file: got %q, want after.txt", got)
	}
	if err := fx.repo.RenameFile(ctx, fx.userID, file.ID, "after.txt"); err != nil {
		t.Fatalf("rename to current name: %v", err)
	}
	if err := fx.repo.RenameFile(ctx, fx.userID, file.ID, "taken.txt"); !errors.Is(err, ErrFileAlreadyExists) {
		t.Fatalf("rename to occupied name: got %v, want ErrFileAlreadyExists", err)
	}
	other, _ := createFilesTestUser(t, fx.tx, 100)
	if err := fx.repo.RenameFile(ctx, other.ID, file.ID, "foreign.txt"); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("rename other user's file: got %v, want ErrFileNotFound", err)
	}
	if got := filesTestSavedFile(t, fx.tx, file.ID).Name; got != "after.txt" {
		t.Fatalf("failed renames changed name to %q", got)
	}
}

func TestFilesRepositoryMoveFileSuccessAndValidation(t *testing.T) {
	fx := newFilesTestFixture(t)
	ctx := context.Background()
	target := &model.Folder{UserID: fx.userID, Name: "destination"}
	if err := fx.tx.Create(target).Error; err != nil {
		t.Fatalf("create target folder: %v", err)
	}
	file, _, _ := mustUploadFilesTestFile(t, fx, "move.txt", filesTestSHA(208), 10)
	if err := fx.repo.MoveFile(ctx, fx.userID, file.ID, target.ID); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if got := filesTestSavedFile(t, fx.tx, file.ID).FolderID; got != target.ID {
		t.Fatalf("moved folder ID: got %d, want %d", got, target.ID)
	}
	if err := fx.repo.MoveFile(ctx, fx.userID, file.ID, target.ID); err != nil {
		t.Fatalf("move to current folder: %v", err)
	}

	conflicting, _, _ := mustUploadFilesTestFile(t, fx, "same.txt", filesTestSHA(209), 5)
	if _, _, _, err := fx.repo.CreateUploadedFile(
		ctx, fx.userID, target.ID, "same.txt", filesTestSHA(210), 5, "objects/test/files/target-duplicate",
	); err != nil {
		t.Fatalf("prepare target duplicate: %v", err)
	}
	if err := fx.repo.MoveFile(ctx, fx.userID, conflicting.ID, target.ID); !errors.Is(err, ErrFileAlreadyExists) {
		t.Fatalf("move to occupied name: got %v, want ErrFileAlreadyExists", err)
	}
	other, otherFolder := createFilesTestUser(t, fx.tx, 100)
	if err := fx.repo.MoveFile(ctx, other.ID, file.ID, target.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("move other user's file: got %v, want ErrFileNotFound", err)
	}
	if err := fx.repo.MoveFile(ctx, fx.userID, conflicting.ID, otherFolder.ID); !errors.Is(err, ErrFileFolderNotFound) {
		t.Fatalf("move into other user's folder: got %v, want ErrFileFolderNotFound", err)
	}
	if got := filesTestSavedFile(t, fx.tx, conflicting.ID).FolderID; got != fx.folderID {
		t.Fatalf("failed moves changed source folder to %d", got)
	}
}

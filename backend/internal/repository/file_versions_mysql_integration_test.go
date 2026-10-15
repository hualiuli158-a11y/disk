package repository

import (
	"context"
	"errors"
	"testing"

	"disk/internal/model"
)

func TestFilesRepositoryAutosaveAndManualCheckpoints(t *testing.T) {
	fx := newFilesTestFixture(t)
	ctx := context.Background()
	a, b, c := filesTestSHA(401), filesTestSHA(402), filesTestSHA(403)
	file, _, _ := mustUploadFilesTestFile(t, fx, "autosave.md", a, 10)
	update := func(expected, next string, checkpoint bool) {
		t.Helper()
		_, _, _, err := fx.repo.ReplaceFileContent(ctx, fx.userID, file.ID, expected, next, 10, "objects/test/checkpoint/"+next, checkpoint)
		if err != nil {
			t.Fatal(err)
		}
	}
	update(a, b, false)
	update(b, c, false)
	if got := filesTestVersionCount(t, fx.tx, file.ID); got != 1 {
		t.Fatalf("autosaves created versions: %d", got)
	}
	versions, err := fx.repo.ListFileVersions(ctx, fx.userID, file.ID)
	if err != nil || len(versions) != 1 || versions[0].Current {
		t.Fatalf("old upload incorrectly current: %+v, %v", versions, err)
	}
	update(c, c, true) // Current content was already autosaved; Ctrl+S still creates a checkpoint.
	update(c, c, true) // Repeated Ctrl+S is idempotent.
	if got := filesTestVersionCount(t, fx.tx, file.ID); got != 2 {
		t.Fatalf("manual checkpoint count = %d", got)
	}
	versions, err = fx.repo.ListFileVersions(ctx, fx.userID, file.ID)
	if err != nil || !versions[0].Current || versions[0].PhysicalFile.SHA256 != c {
		t.Fatalf("checkpoint = %+v, %v", versions, err)
	}
	update(c, b, false)
	_, _, _, err = fx.repo.ReplaceFileContent(ctx, fx.userID, file.ID, c, a, 10, "objects/test/stale-checkpoint", true)
	if !errors.Is(err, ErrFileVersionConflict) {
		t.Fatalf("stale checkpoint error = %v", err)
	}
	if got := filesTestVersionCount(t, fx.tx, file.ID); got != 2 {
		t.Fatalf("conflict created version: %d", got)
	}
	_, checkpoint, err := fx.repo.FindFileVersionForDownload(ctx, fx.userID, file.ID, versions[0].ID)
	if err != nil || checkpoint.PhysicalFile.SHA256 != c {
		t.Fatalf("autosave changed checkpoint: %+v, %v", checkpoint, err)
	}
}

func TestFilesRepositoryReplaceFileContentGrowthShrinkAndReuse(t *testing.T) {
	fx := newFilesTestFixture(t)
	a, b, c := filesTestSHA(101), filesTestSHA(102), filesTestSHA(103)
	file, firstPhysical, _ := mustUploadFilesTestFile(t, fx, "editable.txt", a, 10)

	updated, larger, created := mustReplaceFilesTestContent(t, fx, file.ID, a, b, 20)
	if !created || updated.PhysicalFileID != larger.ID || filesTestUsedQuota(t, fx.tx, fx.userID) != 20 {
		t.Fatalf("growth: file=%+v, physical=%+v, created=%t", updated, larger, created)
	}
	updated, smaller, created := mustReplaceFilesTestContent(t, fx, file.ID, b, c, 5)
	if !created || updated.PhysicalFileID != smaller.ID || filesTestUsedQuota(t, fx.tx, fx.userID) != 5 {
		t.Fatalf("shrink: file=%+v, physical=%+v, created=%t", updated, smaller, created)
	}
	updated, reused, created := mustReplaceFilesTestContent(t, fx, file.ID, c, a, 10)
	if created || updated.PhysicalFileID != firstPhysical.ID || reused.ID != firstPhysical.ID ||
		reused.StorageKey != firstPhysical.StorageKey {
		t.Fatalf("reuse: file=%+v, physical=%+v, created=%t", updated, reused, created)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 3, 4, 10)
	if saved := filesTestSavedFile(t, fx.tx, file.ID); saved.PhysicalFileID != firstPhysical.ID {
		t.Fatalf("saved physical reference: got %d, want %d", saved.PhysicalFileID, firstPhysical.ID)
	}
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 4 {
		t.Fatalf("list edited versions: versions=%+v, err=%v", versions, err)
	}
	for i, want := range []uint64{firstPhysical.ID, smaller.ID, larger.ID, firstPhysical.ID} {
		if versions[i].PhysicalFileID != want {
			t.Errorf("version %d physical ID: got %d, want %d", i, versions[i].PhysicalFileID, want)
		}
		if i < 3 && versions[i].Source != "edit" || i == 3 && versions[i].Source != "upload" {
			t.Errorf("version %d source: %q", i, versions[i].Source)
		}
	}
}

func TestFilesRepositoryReplaceFileContentSameSHAIsNoOp(t *testing.T) {
	fx := newFilesTestFixture(t)
	sha := filesTestSHA(104)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "same.txt", sha, 10)
	result, reused, created, err := fx.repo.ReplaceFileContent(
		context.Background(), fx.userID, file.ID, sha, sha, 10, "objects/test/files/unused-candidate", true,
	)
	if err != nil || result == nil || reused == nil || created || result.PhysicalFileID != physical.ID ||
		reused.ID != physical.ID || reused.StorageKey != physical.StorageKey {
		t.Fatalf("same SHA replacement: file=%+v, physical=%+v, created=%t, err=%v", result, reused, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 10)
}

func TestFilesRepositoryReplaceFileContentExpectedSHAConflict(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "conflict.txt", filesTestSHA(105), 10)
	result, stored, created, err := fx.repo.ReplaceFileContent(
		context.Background(), fx.userID, file.ID, filesTestSHA(999), filesTestSHA(106), 20, "objects/test/files/conflict", true,
	)
	if !errors.Is(err, ErrFileVersionConflict) || result != nil || stored != nil || created {
		t.Fatalf("expected SHA conflict: file=%+v, physical=%+v, created=%t, err=%v", result, stored, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 10)
	if saved := filesTestSavedFile(t, fx.tx, file.ID); saved.PhysicalFileID != physical.ID {
		t.Fatalf("reference changed after conflict: %+v", saved)
	}
}

func TestFilesRepositoryReplaceFileContentOtherUsersFile(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "private.txt", filesTestSHA(107), 10)
	other, _ := createFilesTestUser(t, fx.tx, 100)
	result, stored, created, err := fx.repo.ReplaceFileContent(
		context.Background(), other.ID, file.ID, filesTestSHA(107), filesTestSHA(108), 20, "objects/test/files/foreign", true,
	)
	if !errors.Is(err, ErrFileNotFound) || result != nil || stored != nil || created {
		t.Fatalf("other user's file: file=%+v, physical=%+v, created=%t, err=%v", result, stored, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 10)
	if filesTestUsedQuota(t, fx.tx, other.ID) != 0 || filesTestSavedFile(t, fx.tx, file.ID).PhysicalFileID != physical.ID {
		t.Fatal("other-user replacement changed database state")
	}
}

func TestFilesRepositoryReplaceFileContentQuotaExceeded(t *testing.T) {
	fx := newFilesTestFixture(t)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "quota.txt", filesTestSHA(109), 10)
	result, stored, created, err := fx.repo.ReplaceFileContent(
		context.Background(), fx.userID, file.ID, filesTestSHA(109), filesTestSHA(110), 101, "objects/test/files/over-quota", true,
	)
	if !errors.Is(err, ErrQuotaExceeded) || result != nil || stored != nil || created {
		t.Fatalf("replacement over quota: file=%+v, physical=%+v, created=%t, err=%v", result, stored, created, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 10)
	if saved := filesTestSavedFile(t, fx.tx, file.ID); saved.PhysicalFileID != physical.ID {
		t.Fatalf("reference changed after quota error: %+v", saved)
	}
}

func TestFilesRepositoryListFileVersionsNewestFirstAndOwnership(t *testing.T) {
	fx := newFilesTestFixture(t)
	a, b, c := filesTestSHA(111), filesTestSHA(112), filesTestSHA(113)
	file, _, _ := mustUploadFilesTestFile(t, fx, "history.txt", a, 10)
	mustReplaceFilesTestContent(t, fx, file.ID, a, b, 11)
	mustReplaceFilesTestContent(t, fx, file.ID, b, c, 12)
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 3 {
		t.Fatalf("list versions: versions=%+v, err=%v", versions, err)
	}
	for i, wantSHA := range []string{c, b, a} {
		if versions[i].PhysicalFile.SHA256 != wantSHA || versions[i].PhysicalFile.ID != versions[i].PhysicalFileID ||
			versions[i].PhysicalFile.Size != []int64{12, 11, 10}[i] ||
			versions[i].PhysicalFile.StorageKey == "" || versions[i].FileID != file.ID {
			t.Errorf("version %d has incomplete or wrong physical data: %+v", i, versions[i])
		}
		if i > 0 && versions[i-1].ID <= versions[i].ID {
			t.Errorf("versions not newest first: %d before %d", versions[i-1].ID, versions[i].ID)
		}
	}
	other, _ := createFilesTestUser(t, fx.tx, 100)
	if _, err := fx.repo.ListFileVersions(context.Background(), other.ID, file.ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("other user's versions: got %v, want ErrFileNotFound", err)
	}
}

func TestFilesRepositoryFindFileVersionForDownloadOwnershipAndMembership(t *testing.T) {
	fx := newFilesTestFixture(t)
	a, b := filesTestSHA(114), filesTestSHA(115)
	file, _, _ := mustUploadFilesTestFile(t, fx, "download.txt", a, 10)
	mustReplaceFilesTestContent(t, fx, file.ID, a, b, 20)
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("prepare versions: versions=%+v, err=%v", versions, err)
	}
	logical, historical, err := fx.repo.FindFileVersionForDownload(context.Background(), fx.userID, file.ID, versions[1].ID)
	if err != nil || logical == nil || historical == nil || logical.ID != file.ID ||
		logical.Name != file.Name || historical.PhysicalFile.SHA256 != a ||
		historical.PhysicalFile.Size != 10 || historical.PhysicalFile.StorageKey == "" {
		t.Fatalf("historical download: file=%+v, version=%+v, err=%v", logical, historical, err)
	}
	other, _ := createFilesTestUser(t, fx.tx, 100)
	if _, _, err := fx.repo.FindFileVersionForDownload(context.Background(), other.ID, file.ID, versions[1].ID); !errors.Is(err, ErrFileNotFound) {
		t.Fatalf("other user's file: got %v, want ErrFileNotFound", err)
	}
	second, _, _ := mustUploadFilesTestFile(t, fx, "another.txt", filesTestSHA(116), 5)
	otherVersions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, second.ID)
	if err != nil || len(otherVersions) != 1 {
		t.Fatalf("prepare other file version: versions=%+v, err=%v", otherVersions, err)
	}
	if _, _, err := fx.repo.FindFileVersionForDownload(context.Background(), fx.userID, file.ID, otherVersions[0].ID); !errors.Is(err, ErrFileVersionNotFound) {
		t.Fatalf("version from another file: got %v, want ErrFileVersionNotFound", err)
	}
}

func TestFilesRepositoryRestoreFileVersionSuccess(t *testing.T) {
	fx := newFilesTestFixture(t)
	a, b := filesTestSHA(117), filesTestSHA(118)
	file, original, _ := mustUploadFilesTestFile(t, fx, "restore.txt", a, 10)
	mustReplaceFilesTestContent(t, fx, file.ID, a, b, 20)
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("prepare restore versions: versions=%+v, err=%v", versions, err)
	}
	target := versions[1]
	restoredFile, restored, err := fx.repo.RestoreFileVersion(context.Background(), fx.userID, file.ID, target.ID, b)
	if err != nil || restoredFile == nil || restored == nil || restoredFile.PhysicalFileID != original.ID ||
		restored.PhysicalFileID != original.ID || restored.Source != "restore" ||
		restored.RestoredFromVersionID == nil || *restored.RestoredFromVersionID != target.ID {
		t.Fatalf("restore result: file=%+v, version=%+v, err=%v", restoredFile, restored, err)
	}
	if filesTestSavedFile(t, fx.tx, file.ID).PhysicalFileID != original.ID ||
		filesTestVersionCount(t, fx.tx, file.ID) != 3 || filesTestUsedQuota(t, fx.tx, fx.userID) != 10 {
		t.Fatal("restore did not persist reference, version and quota together")
	}
	var saved model.FileVersion
	if err := fx.tx.First(&saved, restored.ID).Error; err != nil || saved.RestoredFromVersionID == nil ||
		*saved.RestoredFromVersionID != target.ID || saved.Source != "restore" {
		t.Fatalf("saved restore version: version=%+v, err=%v", saved, err)
	}
}

func TestFilesRepositoryRestoreFileVersionExpectedSHAConflict(t *testing.T) {
	fx := newFilesTestFixture(t)
	a, b := filesTestSHA(119), filesTestSHA(120)
	file, original, _ := mustUploadFilesTestFile(t, fx, "conflict.txt", a, 10)
	_, current, _ := mustReplaceFilesTestContent(t, fx, file.ID, a, b, 20)
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("prepare restore versions: versions=%+v, err=%v", versions, err)
	}
	_, _, err = fx.repo.RestoreFileVersion(context.Background(), fx.userID, file.ID, versions[1].ID, original.SHA256)
	if !errors.Is(err, ErrFileVersionConflict) {
		t.Fatalf("restore conflict: got %v, want ErrFileVersionConflict", err)
	}
	if filesTestSavedFile(t, fx.tx, file.ID).PhysicalFileID != current.ID ||
		filesTestVersionCount(t, fx.tx, file.ID) != 2 || filesTestUsedQuota(t, fx.tx, fx.userID) != 20 {
		t.Fatal("conflicting restore changed database state")
	}
}

func TestFilesRepositoryRestoreFileVersionMissingVersion(t *testing.T) {
	fx := newFilesTestFixture(t)
	sha := filesTestSHA(121)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "missing.txt", sha, 10)
	_, _, err := fx.repo.RestoreFileVersion(context.Background(), fx.userID, file.ID, 0, sha)
	if !errors.Is(err, ErrFileVersionNotFound) {
		t.Fatalf("missing restore target: got %v, want ErrFileVersionNotFound", err)
	}
	if filesTestSavedFile(t, fx.tx, file.ID).PhysicalFileID != physical.ID ||
		filesTestVersionCount(t, fx.tx, file.ID) != 1 || filesTestUsedQuota(t, fx.tx, fx.userID) != 10 {
		t.Fatal("missing-target restore changed database state")
	}
}

func TestFilesRepositoryRestoreFileVersionQuotaExceeded(t *testing.T) {
	fx := newFilesTestFixture(t)
	a, b := filesTestSHA(122), filesTestSHA(123)
	file, _, _ := mustUploadFilesTestFile(t, fx, "quota.txt", a, 90)
	_, current, _ := mustReplaceFilesTestContent(t, fx, file.ID, a, b, 5)
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("prepare restore versions: versions=%+v, err=%v", versions, err)
	}
	if err := fx.tx.Model(&model.User{}).Where("id = ?", fx.userID).Update("quota_limit", 80).Error; err != nil {
		t.Fatalf("lower fixture quota: %v", err)
	}
	_, _, err = fx.repo.RestoreFileVersion(context.Background(), fx.userID, file.ID, versions[1].ID, b)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("restore over quota: got %v, want ErrQuotaExceeded", err)
	}
	if filesTestSavedFile(t, fx.tx, file.ID).PhysicalFileID != current.ID ||
		filesTestVersionCount(t, fx.tx, file.ID) != 2 || filesTestUsedQuota(t, fx.tx, fx.userID) != 5 {
		t.Fatal("over-quota restore changed database state")
	}
}

func TestFilesRepositoryRestoreFileVersionCurrentContentIsNoOp(t *testing.T) {
	fx := newFilesTestFixture(t)
	sha := filesTestSHA(124)
	file, physical, _ := mustUploadFilesTestFile(t, fx, "same.txt", sha, 10)
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("prepare current version: versions=%+v, err=%v", versions, err)
	}
	result, restored, err := fx.repo.RestoreFileVersion(context.Background(), fx.userID, file.ID, versions[0].ID, sha)
	if err != nil || result == nil || restored == nil || result.PhysicalFileID != physical.ID || restored.ID != versions[0].ID {
		t.Fatalf("restore current content: file=%+v, version=%+v, err=%v", result, restored, err)
	}
	assertUploadState(t, fx.tx, fx.userID, 1, 1, 1, 10)
}

func TestFilesRepositoryReplaceFileContentRetainsLatest50Versions(t *testing.T) {
	fx := newFilesTestFixture(t)
	firstSHA := filesTestSHA(1000)
	file, _, _ := mustUploadFilesTestFile(t, fx, "many-versions.txt", firstSHA, 1)
	initial, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(initial) != 1 {
		t.Fatalf("prepare initial version: versions=%+v, err=%v", initial, err)
	}
	oldestID := initial[0].ID
	previousSHA := firstSHA
	for i := 1; i <= retainedVersionsPerFile; i++ {
		nextSHA := filesTestSHA(1000 + i)
		mustReplaceFilesTestContent(t, fx, file.ID, previousSHA, nextSHA, 1)
		previousSHA = nextSHA
	}
	versions, err := fx.repo.ListFileVersions(context.Background(), fx.userID, file.ID)
	if err != nil || len(versions) != retainedVersionsPerFile {
		t.Fatalf("retained versions: count=%d, err=%v", len(versions), err)
	}
	if versions[0].PhysicalFile.SHA256 != previousSHA || versions[len(versions)-1].PhysicalFile.SHA256 != filesTestSHA(1001) {
		t.Fatalf("retention boundaries: newest=%s, oldest=%s", versions[0].PhysicalFile.SHA256, versions[len(versions)-1].PhysicalFile.SHA256)
	}
	for i := 1; i < len(versions); i++ {
		if versions[i-1].ID <= versions[i].ID {
			t.Fatalf("retained versions out of order at %d: %d <= %d", i, versions[i-1].ID, versions[i].ID)
		}
	}
	if _, _, err := fx.repo.FindFileVersionForDownload(context.Background(), fx.userID, file.ID, oldestID); !errors.Is(err, ErrFileVersionNotFound) {
		t.Fatalf("pruned version download: got %v, want ErrFileVersionNotFound", err)
	}
	if filesTestUsedQuota(t, fx.tx, fx.userID) != 1 {
		t.Fatalf("quota changed across equal-size replacements: %d", filesTestUsedQuota(t, fx.tx, fx.userID))
	}
}

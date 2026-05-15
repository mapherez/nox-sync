package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"
)

const (
	testUserID  = "user_test"
	testVaultID = "vault_test"
)

func TestBeginSyncRejectsSecondActiveSession(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	}); err != nil {
		t.Fatalf("begin first sync: %v", err)
	}

	_, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_b",
		ClientName: "B",
		VaultID:    "vault_test",
	})
	if !errors.Is(err, ErrSyncLocked) {
		t.Fatalf("expected ErrSyncLocked, got %v", err)
	}
}

func TestDifferentVaultsCanSyncConcurrently(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	secondVault, err := store.CreateVault(ctx, testUserID, "Second Vault")
	if err != nil {
		t.Fatalf("create second vault: %v", err)
	}

	if _, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    testVaultID,
	}); err != nil {
		t.Fatalf("begin first vault sync: %v", err)
	}

	if _, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_b",
		ClientName: "B",
		VaultID:    secondVault.ID,
	}); err != nil {
		t.Fatalf("begin second vault sync: %v", err)
	}
}

func TestUserCannotSyncAnotherUsersVault(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	otherUser, err := store.UpsertAllowedUser(ctx, "other@example.com", UserRoleUser)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	otherVault, err := store.CreateVault(ctx, otherUser.ID, "Other Vault")
	if err != nil {
		t.Fatalf("create other vault: %v", err)
	}

	_, err = store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    otherVault.ID,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for another user's vault, got %v", err)
	}

	if _, err := store.DownloadFile(ctx, testUserID, otherVault.ID, "note.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for another user's download, got %v", err)
	}
}

func TestPurgeDeletedVaultRemovesUnreferencedBlob(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("purge me")
	hash := sha256Hex(content)

	commitFileForTest(t, store, "client_a", 0, "purge.md", content)
	if _, err := os.Stat(store.BlobPath(hash)); err != nil {
		t.Fatalf("expected blob to exist before purge: %v", err)
	}

	if err := store.SoftDeleteVault(ctx, testUserID, testVaultID); err != nil {
		t.Fatalf("soft delete vault: %v", err)
	}
	if err := store.PurgeDeletedVault(ctx, testUserID, testVaultID); err != nil {
		t.Fatalf("purge deleted vault: %v", err)
	}
	if _, err := os.Stat(store.BlobPath(hash)); !os.IsNotExist(err) {
		t.Fatalf("expected unreferenced blob to be removed, got %v", err)
	}
}

func TestFirstUploadCommitCreatesDownloadableBlob(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("# Hello\n")
	hash := sha256Hex(content)

	begin, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	})
	if err != nil {
		t.Fatalf("begin sync: %v", err)
	}

	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: begin.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "Notes/Hello.md",
			Hash: hash,
			Size: int64(len(content)),
		}},
	})
	if err != nil {
		t.Fatalf("plan sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionUpload {
		t.Fatalf("expected one upload action, got %#v", plan.Actions)
	}

	if err := store.StageUpload(ctx, testUserID, begin.SessionID, "client_a", "Notes/Hello.md", hash, int64(len(content)), bytes.NewReader(content)); err != nil {
		t.Fatalf("stage upload: %v", err)
	}

	commit, err := store.CommitSync(ctx, testUserID, CommitRequest{
		SessionID: begin.SessionID,
		ClientID:  "client_a",
	})
	if err != nil {
		t.Fatalf("commit sync: %v", err)
	}
	if commit.ServerRevision != 1 {
		t.Fatalf("expected server revision 1, got %d", commit.ServerRevision)
	}

	download, err := store.DownloadFile(ctx, testUserID, testVaultID, "Notes/Hello.md")
	if err != nil {
		t.Fatalf("download metadata: %v", err)
	}
	if download.Hash != hash {
		t.Fatalf("expected hash %s, got %s", hash, download.Hash)
	}
	if _, err := os.Stat(store.BlobPath(hash)); err != nil {
		t.Fatalf("expected blob to exist: %v", err)
	}
}

func TestStageUploadRejectsHashMismatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("correct")
	expectedHash := sha256Hex(content)

	begin, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	})
	if err != nil {
		t.Fatalf("begin sync: %v", err)
	}

	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: begin.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: expectedHash,
			Size: int64(len(content)),
		}},
	}); err != nil {
		t.Fatalf("plan sync: %v", err)
	}

	err = store.StageUpload(ctx, testUserID, begin.SessionID, "client_a", "note.md", expectedHash, int64(len(content)), bytes.NewReader([]byte("wrong!!")))
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
}

func TestPlanSyncReportsConflictForDivergedFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("remote")
	baseHash := sha256Hex(baseContent)

	first := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: first.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: baseHash,
			Size: int64(len(baseContent)),
		}},
	}); err != nil {
		t.Fatalf("plan first sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	localContent := []byte("local")
	localHash := sha256Hex(localContent)
	second := beginSyncForTest(t, store, "client_b")
	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 0,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              localHash,
			Size:              int64(len(localContent)),
			LastKnownRevision: 0,
		}},
	})
	if err != nil {
		t.Fatalf("plan second sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionConflict {
		t.Fatalf("expected conflict action, got %#v", plan.Actions)
	}
}

func TestPlanSyncDownloadsRemoteOnlyChangeForUnchangedLocalFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("base")
	baseHash := sha256Hex(baseContent)
	remoteContent := []byte("remote")
	remoteHash := sha256Hex(remoteContent)

	first := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: first.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: baseHash,
			Size: int64(len(baseContent)),
		}},
	}); err != nil {
		t.Fatalf("plan first sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              remoteHash,
			Size:              int64(len(remoteContent)),
			LastKnownRevision: 1,
		}},
	}); err != nil {
		t.Fatalf("plan second sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, second.SessionID, "client_b", "note.md", remoteHash, int64(len(remoteContent)), bytes.NewReader(remoteContent)); err != nil {
		t.Fatalf("stage second upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit second sync: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               third.SessionID,
		ClientID:                "client_a",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              baseHash,
			Size:              int64(len(baseContent)),
			LastKnownRevision: 1,
		}},
	})
	if err != nil {
		t.Fatalf("plan third sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionDownload {
		t.Fatalf("expected download action, got %#v", plan.Actions)
	}
}

func TestPlanSyncRemoteDeleteConflictsWhenLocalChanged(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("base")
	baseHash := sha256Hex(baseContent)
	localContent := []byte("local")
	localHash := sha256Hex(localContent)

	first := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: first.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: baseHash,
			Size: int64(len(baseContent)),
		}},
	}); err != nil {
		t.Fatalf("plan first sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		DeletedPaths:            []string{"note.md"},
	}); err != nil {
		t.Fatalf("plan second sync: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit second sync: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               third.SessionID,
		ClientID:                "client_a",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              localHash,
			Size:              int64(len(localContent)),
			LastKnownRevision: 1,
		}},
	})
	if err != nil {
		t.Fatalf("plan third sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionConflict {
		t.Fatalf("expected conflict action, got %#v", plan.Actions)
	}
}

func TestPlanSyncUploadsAfterExplicitRemoteDeleteResolution(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("base")
	baseHash := sha256Hex(baseContent)

	first := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: first.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: baseHash,
			Size: int64(len(baseContent)),
		}},
	}); err != nil {
		t.Fatalf("plan first sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		DeletedPaths:            []string{"note.md"},
	}); err != nil {
		t.Fatalf("plan delete sync: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit delete sync: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               third.SessionID,
		ClientID:                "client_a",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              baseHash,
			Size:              int64(len(baseContent)),
			LastKnownRevision: 2,
		}},
	})
	if err != nil {
		t.Fatalf("plan resolved upload sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionUpload {
		t.Fatalf("expected upload action, got %#v", plan.Actions)
	}
}

func TestPlanSyncDeletesRemoteAfterExplicitLocalDeleteResolution(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("base")
	baseHash := sha256Hex(baseContent)
	remoteContent := []byte("remote")
	remoteHash := sha256Hex(remoteContent)

	first := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: first.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: baseHash,
			Size: int64(len(baseContent)),
		}},
	}); err != nil {
		t.Fatalf("plan first sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              remoteHash,
			Size:              int64(len(remoteContent)),
			LastKnownRevision: 1,
		}},
	}); err != nil {
		t.Fatalf("plan remote update sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, second.SessionID, "client_b", "note.md", remoteHash, int64(len(remoteContent)), bytes.NewReader(remoteContent)); err != nil {
		t.Fatalf("stage remote update: %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit remote update: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               third.SessionID,
		ClientID:                "client_a",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			LastKnownRevision: 2,
			Deleted:           true,
		}},
	})
	if err != nil {
		t.Fatalf("plan resolved delete sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionDeleteRemote {
		t.Fatalf("expected delete_remote action, got %#v", plan.Actions)
	}
}

func TestExpiredLockCleanupRemovesAbandonedStaging(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("abandoned")
	hash := sha256Hex(content)

	begin := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: begin.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: hash,
			Size: int64(len(content)),
		}},
	}); err != nil {
		t.Fatalf("plan sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, begin.SessionID, "client_a", "note.md", hash, int64(len(content)), bytes.NewReader(content)); err != nil {
		t.Fatalf("stage upload: %v", err)
	}
	if _, err := os.Stat(store.stagingSessionDir(begin.SessionID)); err != nil {
		t.Fatalf("expected staging directory before stale cleanup: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		UPDATE sync_locks
		SET expires_at = ?
		WHERE vault_id = ?
	`, timestamp(time.Now().Add(-time.Second)), testVaultID); err != nil {
		t.Fatalf("expire lock: %v", err)
	}

	status, err := store.SyncStatus(ctx, testUserID, testVaultID)
	if err != nil {
		t.Fatalf("load sync status: %v", err)
	}
	if status.State != SyncStateStaleLock {
		t.Fatalf("expected stale lock status, got %q", status.State)
	}
	if _, err := os.Stat(store.stagingSessionDir(begin.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("expected abandoned staging directory to be removed, got %v", err)
	}

	if _, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_b",
		ClientName: "B",
		VaultID:    "vault_test",
	}); err != nil {
		t.Fatalf("expected new sync after stale cleanup: %v", err)
	}
}

func TestReapExpiredLockReportsTransitionOnce(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	begin := beginSyncForTest(t, store, "client_a")
	if _, err := store.db.ExecContext(ctx, `
		UPDATE sync_locks
		SET expires_at = ?
		WHERE vault_id = ?
	`, timestamp(time.Now().Add(-time.Second)), testVaultID); err != nil {
		t.Fatalf("expire lock: %v", err)
	}

	reaped, err := store.ReapExpiredLocks(ctx)
	if err != nil {
		t.Fatalf("reap expired lock: %v", err)
	}
	if len(reaped) != 1 || reaped[0] != testVaultID {
		t.Fatalf("expected first stale-lock reap to report a transition")
	}

	status, err := store.SyncStatus(ctx, testUserID, testVaultID)
	if err != nil {
		t.Fatalf("load sync status: %v", err)
	}
	if status.State != SyncStateStaleLock {
		t.Fatalf("expected stale lock status, got %q", status.State)
	}

	reaped, err = store.ReapExpiredLocks(ctx)
	if err != nil {
		t.Fatalf("reap expired lock again: %v", err)
	}
	if len(reaped) != 0 {
		t.Fatalf("expected second stale-lock reap to report no transition")
	}

	var sessionStatus string
	if err := store.db.QueryRowContext(ctx, `
		SELECT status
		FROM sync_sessions
		WHERE session_id = ?
	`, begin.SessionID).Scan(&sessionStatus); err != nil {
		t.Fatalf("load stale session: %v", err)
	}
	if sessionStatus != SessionStatusFailed {
		t.Fatalf("expected failed session status, got %q", sessionStatus)
	}
}

func TestCommitRejectsMissingStagedUpload(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("pending")
	hash := sha256Hex(content)

	begin := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID: begin.SessionID,
		ClientID:  "client_a",
		VaultID:   "vault_test",
		Files: []ManifestFile{{
			Path: "note.md",
			Hash: hash,
			Size: int64(len(content)),
		}},
	}); err != nil {
		t.Fatalf("plan sync: %v", err)
	}

	_, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: begin.SessionID, ClientID: "client_a"})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for missing staged upload, got %v", err)
	}

	revision, err := store.ServerRevision(ctx, testUserID, testVaultID)
	if err != nil {
		t.Fatalf("load server revision: %v", err)
	}
	if revision != 0 {
		t.Fatalf("expected server revision to remain 0, got %d", revision)
	}
	if _, err := store.DownloadFile(ctx, testUserID, testVaultID, "note.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing remote file after rejected commit, got %v", err)
	}
	if err := store.AbortSync(ctx, testUserID, AbortRequest{SessionID: begin.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("abort failed sync: %v", err)
	}
}

func TestCommitRejectsConflictPlan(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("base")
	baseHash := sha256Hex(baseContent)
	localContent := []byte("local")
	localHash := sha256Hex(localContent)

	commitFileForTest(t, store, "client_a", 0, "note.md", baseContent)

	second := beginSyncForTest(t, store, "client_b")
	plan, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 0,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              localHash,
			Size:              int64(len(localContent)),
			LastKnownRevision: 0,
		}},
	})
	if err != nil {
		t.Fatalf("plan conflict sync: %v", err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != PlanActionConflict {
		t.Fatalf("expected conflict plan, got %#v", plan.Actions)
	}

	_, err = store.CommitSync(ctx, testUserID, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"})
	if !errors.Is(err, ErrConflictDetected) {
		t.Fatalf("expected ErrConflictDetected, got %v", err)
	}

	download, err := store.DownloadFile(ctx, testUserID, testVaultID, "note.md")
	if err != nil {
		t.Fatalf("download original file: %v", err)
	}
	if download.Hash != baseHash {
		t.Fatalf("expected remote hash to remain %s, got %s", baseHash, download.Hash)
	}
	if err := store.AbortSync(ctx, testUserID, AbortRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("abort conflict sync: %v", err)
	}
}

func TestSyncSessionRejectsWrongClientOwnership(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("owned")
	hash := sha256Hex(content)

	begin := beginSyncForTest(t, store, "client_a")
	if err := store.HeartbeatSync(ctx, testUserID, HeartbeatRequest{SessionID: begin.SessionID, ClientID: "client_b"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected heartbeat ownership rejection, got %v", err)
	}
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{SessionID: begin.SessionID, ClientID: "client_b", VaultID: "vault_test"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected plan ownership rejection, got %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, begin.SessionID, "client_b", "note.md", hash, int64(len(content)), bytes.NewReader(content)); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected upload ownership rejection, got %v", err)
	}
	if _, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: begin.SessionID, ClientID: "client_b"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected commit ownership rejection, got %v", err)
	}
	if err := store.AbortSync(ctx, testUserID, AbortRequest{SessionID: begin.SessionID, ClientID: "client_b"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected abort ownership rejection, got %v", err)
	}

	if err := store.AbortSync(ctx, testUserID, AbortRequest{SessionID: begin.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("owner should still be able to abort: %v", err)
	}
}

func TestHashMismatchLeavesRemoteStateUnchanged(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseContent := []byte("base")
	baseHash := sha256Hex(baseContent)
	newContent := []byte("new")
	newHash := sha256Hex(newContent)

	commitFileForTest(t, store, "client_a", 0, "note.md", baseContent)

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		Files: []ManifestFile{{
			Path:              "note.md",
			Hash:              newHash,
			Size:              int64(len(newContent)),
			LastKnownRevision: 1,
		}},
	}); err != nil {
		t.Fatalf("plan update sync: %v", err)
	}

	err := store.StageUpload(ctx, testUserID, second.SessionID, "client_b", "note.md", newHash, int64(len(newContent)), bytes.NewReader([]byte("bad")))
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
	_, err = store.CommitSync(ctx, testUserID, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected commit to reject missing validated upload, got %v", err)
	}

	revision, err := store.ServerRevision(ctx, testUserID, testVaultID)
	if err != nil {
		t.Fatalf("load server revision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("expected server revision to remain 1, got %d", revision)
	}
	download, err := store.DownloadFile(ctx, testUserID, testVaultID, "note.md")
	if err != nil {
		t.Fatalf("download original file: %v", err)
	}
	if download.Hash != baseHash {
		t.Fatalf("expected remote hash to remain %s, got %s", baseHash, download.Hash)
	}
	if _, err := os.Stat(store.BlobPath(newHash)); !os.IsNotExist(err) {
		t.Fatalf("expected rejected blob to be absent, got %v", err)
	}
	if err := store.AbortSync(ctx, testUserID, AbortRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("abort failed update sync: %v", err)
	}
}

func TestRestartWithExpiredActiveLockRecovers(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	store, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	seedTestUserAndVault(t, store)

	begin, err := store.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	})
	if err != nil {
		t.Fatalf("begin sync: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE sync_locks
		SET expires_at = ?
		WHERE vault_id = ?
	`, timestamp(time.Now().Add(-time.Second)), testVaultID); err != nil {
		t.Fatalf("expire lock: %v", err)
	}
	if err := os.MkdirAll(store.stagingSessionDir(begin.SessionID), 0o755); err != nil {
		t.Fatalf("create abandoned staging directory: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	reopened, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close reopened store: %v", err)
		}
	}()

	status, err := reopened.SyncStatus(ctx, testUserID, testVaultID)
	if err != nil {
		t.Fatalf("load status after restart: %v", err)
	}
	if status.State != SyncStateStaleLock {
		t.Fatalf("expected stale lock after restart, got %q", status.State)
	}
	if _, err := os.Stat(reopened.stagingSessionDir(begin.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("expected stale staging to be removed after restart recovery, got %v", err)
	}
	if _, err := reopened.BeginSync(ctx, testUserID, BeginSyncRequest{
		ClientID:   "client_b",
		ClientName: "B",
		VaultID:    "vault_test",
	}); err != nil {
		t.Fatalf("expected new sync after restart recovery: %v", err)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	seedTestUserAndVault(t, store)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	return store
}

func seedTestUserAndVault(t *testing.T, store *Store) {
	t.Helper()

	now := timestamp(time.Now())
	if _, err := store.db.ExecContext(context.Background(), `
		INSERT OR IGNORE INTO users (id, email, first_name, display_name, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, testUserID, "test@example.com", "Test", "Test User", UserRoleAdmin, UserStatusActive, now, now); err != nil {
		t.Fatalf("seed test user: %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
		INSERT OR IGNORE INTO vaults (id, user_id, name, revision, status, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?, ?)
	`, testVaultID, testUserID, "Test Vault", VaultStatusActive, now, now); err != nil {
		t.Fatalf("seed test vault: %v", err)
	}
	if _, err := store.CurrentAPIKey(context.Background(), testUserID); err != nil {
		t.Fatalf("seed test api key: %v", err)
	}
}

func beginSyncForTest(t *testing.T, store *Store, clientID string) BeginSyncResult {
	t.Helper()

	begin, err := store.BeginSync(context.Background(), testUserID, BeginSyncRequest{
		ClientID:   clientID,
		ClientName: clientID,
		VaultID:    "vault_test",
	})
	if err != nil {
		t.Fatalf("begin sync for %s: %v", clientID, err)
	}

	return begin
}

func commitFileForTest(t *testing.T, store *Store, clientID string, lastKnownServerRevision int64, path string, content []byte) CommitResult {
	t.Helper()

	ctx := context.Background()
	hash := sha256Hex(content)
	begin := beginSyncForTest(t, store, clientID)
	if _, err := store.PlanSync(ctx, testUserID, ManifestRequest{
		SessionID:               begin.SessionID,
		ClientID:                clientID,
		VaultID:                 "vault_test",
		LastKnownServerRevision: lastKnownServerRevision,
		Files: []ManifestFile{{
			Path:              path,
			Hash:              hash,
			Size:              int64(len(content)),
			LastKnownRevision: lastKnownServerRevision,
		}},
	}); err != nil {
		t.Fatalf("plan test file sync: %v", err)
	}
	if err := store.StageUpload(ctx, testUserID, begin.SessionID, clientID, path, hash, int64(len(content)), bytes.NewReader(content)); err != nil {
		t.Fatalf("stage test file upload: %v", err)
	}
	commit, err := store.CommitSync(ctx, testUserID, CommitRequest{SessionID: begin.SessionID, ClientID: clientID})
	if err != nil {
		t.Fatalf("commit test file sync: %v", err)
	}

	return commit
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

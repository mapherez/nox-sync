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

func TestBeginSyncRejectsSecondActiveSession(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.BeginSync(ctx, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	}); err != nil {
		t.Fatalf("begin first sync: %v", err)
	}

	_, err := store.BeginSync(ctx, BeginSyncRequest{
		ClientID:   "client_b",
		ClientName: "B",
		VaultID:    "vault_test",
	})
	if !errors.Is(err, ErrSyncLocked) {
		t.Fatalf("expected ErrSyncLocked, got %v", err)
	}
}

func TestFirstUploadCommitCreatesDownloadableBlob(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("# Hello\n")
	hash := sha256Hex(content)

	begin, err := store.BeginSync(ctx, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	})
	if err != nil {
		t.Fatalf("begin sync: %v", err)
	}

	plan, err := store.PlanSync(ctx, ManifestRequest{
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

	if err := store.StageUpload(ctx, begin.SessionID, "client_a", "Notes/Hello.md", hash, int64(len(content)), bytes.NewReader(content)); err != nil {
		t.Fatalf("stage upload: %v", err)
	}

	commit, err := store.CommitSync(ctx, CommitRequest{
		SessionID: begin.SessionID,
		ClientID:  "client_a",
	})
	if err != nil {
		t.Fatalf("commit sync: %v", err)
	}
	if commit.ServerRevision != 1 {
		t.Fatalf("expected server revision 1, got %d", commit.ServerRevision)
	}

	download, err := store.DownloadFile(ctx, "Notes/Hello.md")
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

	begin, err := store.BeginSync(ctx, BeginSyncRequest{
		ClientID:   "client_a",
		ClientName: "A",
		VaultID:    "vault_test",
	})
	if err != nil {
		t.Fatalf("begin sync: %v", err)
	}

	if _, err := store.PlanSync(ctx, ManifestRequest{
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

	err = store.StageUpload(ctx, begin.SessionID, "client_a", "note.md", expectedHash, int64(len(content)), bytes.NewReader([]byte("wrong!!")))
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	localContent := []byte("local")
	localHash := sha256Hex(localContent)
	second := beginSyncForTest(t, store, "client_b")
	plan, err := store.PlanSync(ctx, ManifestRequest{
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, second.SessionID, "client_b", "note.md", remoteHash, int64(len(remoteContent)), bytes.NewReader(remoteContent)); err != nil {
		t.Fatalf("stage second upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit second sync: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, ManifestRequest{
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		DeletedPaths:            []string{"note.md"},
	}); err != nil {
		t.Fatalf("plan second sync: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit second sync: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, ManifestRequest{
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, ManifestRequest{
		SessionID:               second.SessionID,
		ClientID:                "client_b",
		VaultID:                 "vault_test",
		LastKnownServerRevision: 1,
		DeletedPaths:            []string{"note.md"},
	}); err != nil {
		t.Fatalf("plan delete sync: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit delete sync: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, ManifestRequest{
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, first.SessionID, "client_a", "note.md", baseHash, int64(len(baseContent)), bytes.NewReader(baseContent)); err != nil {
		t.Fatalf("stage first upload: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: first.SessionID, ClientID: "client_a"}); err != nil {
		t.Fatalf("commit first sync: %v", err)
	}

	second := beginSyncForTest(t, store, "client_b")
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, second.SessionID, "client_b", "note.md", remoteHash, int64(len(remoteContent)), bytes.NewReader(remoteContent)); err != nil {
		t.Fatalf("stage remote update: %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("commit remote update: %v", err)
	}

	third := beginSyncForTest(t, store, "client_a")
	plan, err := store.PlanSync(ctx, ManifestRequest{
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, begin.SessionID, "client_a", "note.md", hash, int64(len(content)), bytes.NewReader(content)); err != nil {
		t.Fatalf("stage upload: %v", err)
	}
	if _, err := os.Stat(store.stagingSessionDir(begin.SessionID)); err != nil {
		t.Fatalf("expected staging directory before stale cleanup: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		UPDATE sync_locks
		SET expires_at = ?
		WHERE id = 1
	`, timestamp(time.Now().Add(-time.Second))); err != nil {
		t.Fatalf("expire lock: %v", err)
	}

	status, err := store.SyncStatus(ctx)
	if err != nil {
		t.Fatalf("load sync status: %v", err)
	}
	if status.State != SyncStateStaleLock {
		t.Fatalf("expected stale lock status, got %q", status.State)
	}
	if _, err := os.Stat(store.stagingSessionDir(begin.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("expected abandoned staging directory to be removed, got %v", err)
	}

	if _, err := store.BeginSync(ctx, BeginSyncRequest{
		ClientID:   "client_b",
		ClientName: "B",
		VaultID:    "vault_test",
	}); err != nil {
		t.Fatalf("expected new sync after stale cleanup: %v", err)
	}
}

func TestCommitRejectsMissingStagedUpload(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("pending")
	hash := sha256Hex(content)

	begin := beginSyncForTest(t, store, "client_a")
	if _, err := store.PlanSync(ctx, ManifestRequest{
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

	_, err := store.CommitSync(ctx, CommitRequest{SessionID: begin.SessionID, ClientID: "client_a"})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for missing staged upload, got %v", err)
	}

	revision, err := store.ServerRevision(ctx)
	if err != nil {
		t.Fatalf("load server revision: %v", err)
	}
	if revision != 0 {
		t.Fatalf("expected server revision to remain 0, got %d", revision)
	}
	if _, err := store.DownloadFile(ctx, "note.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing remote file after rejected commit, got %v", err)
	}
	if err := store.AbortSync(ctx, AbortRequest{SessionID: begin.SessionID, ClientID: "client_a"}); err != nil {
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
	plan, err := store.PlanSync(ctx, ManifestRequest{
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

	_, err = store.CommitSync(ctx, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"})
	if !errors.Is(err, ErrConflictDetected) {
		t.Fatalf("expected ErrConflictDetected, got %v", err)
	}

	download, err := store.DownloadFile(ctx, "note.md")
	if err != nil {
		t.Fatalf("download original file: %v", err)
	}
	if download.Hash != baseHash {
		t.Fatalf("expected remote hash to remain %s, got %s", baseHash, download.Hash)
	}
	if err := store.AbortSync(ctx, AbortRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
		t.Fatalf("abort conflict sync: %v", err)
	}
}

func TestSyncSessionRejectsWrongClientOwnership(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("owned")
	hash := sha256Hex(content)

	begin := beginSyncForTest(t, store, "client_a")
	if err := store.HeartbeatSync(ctx, HeartbeatRequest{SessionID: begin.SessionID, ClientID: "client_b"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected heartbeat ownership rejection, got %v", err)
	}
	if _, err := store.PlanSync(ctx, ManifestRequest{SessionID: begin.SessionID, ClientID: "client_b", VaultID: "vault_test"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected plan ownership rejection, got %v", err)
	}
	if err := store.StageUpload(ctx, begin.SessionID, "client_b", "note.md", hash, int64(len(content)), bytes.NewReader(content)); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected upload ownership rejection, got %v", err)
	}
	if _, err := store.CommitSync(ctx, CommitRequest{SessionID: begin.SessionID, ClientID: "client_b"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected commit ownership rejection, got %v", err)
	}
	if err := store.AbortSync(ctx, AbortRequest{SessionID: begin.SessionID, ClientID: "client_b"}); !errors.Is(err, ErrSyncSessionNotFound) {
		t.Fatalf("expected abort ownership rejection, got %v", err)
	}

	if err := store.AbortSync(ctx, AbortRequest{SessionID: begin.SessionID, ClientID: "client_a"}); err != nil {
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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

	err := store.StageUpload(ctx, second.SessionID, "client_b", "note.md", newHash, int64(len(newContent)), bytes.NewReader([]byte("bad")))
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got %v", err)
	}
	_, err = store.CommitSync(ctx, CommitRequest{SessionID: second.SessionID, ClientID: "client_b"})
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected commit to reject missing validated upload, got %v", err)
	}

	revision, err := store.ServerRevision(ctx)
	if err != nil {
		t.Fatalf("load server revision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("expected server revision to remain 1, got %d", revision)
	}
	download, err := store.DownloadFile(ctx, "note.md")
	if err != nil {
		t.Fatalf("download original file: %v", err)
	}
	if download.Hash != baseHash {
		t.Fatalf("expected remote hash to remain %s, got %s", baseHash, download.Hash)
	}
	if _, err := os.Stat(store.BlobPath(newHash)); !os.IsNotExist(err) {
		t.Fatalf("expected rejected blob to be absent, got %v", err)
	}
	if err := store.AbortSync(ctx, AbortRequest{SessionID: second.SessionID, ClientID: "client_b"}); err != nil {
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

	begin, err := store.BeginSync(ctx, BeginSyncRequest{
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
		WHERE id = 1
	`, timestamp(time.Now().Add(-time.Second))); err != nil {
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

	status, err := reopened.SyncStatus(ctx)
	if err != nil {
		t.Fatalf("load status after restart: %v", err)
	}
	if status.State != SyncStateStaleLock {
		t.Fatalf("expected stale lock after restart, got %q", status.State)
	}
	if _, err := os.Stat(reopened.stagingSessionDir(begin.SessionID)); !os.IsNotExist(err) {
		t.Fatalf("expected stale staging to be removed after restart recovery, got %v", err)
	}
	if _, err := reopened.BeginSync(ctx, BeginSyncRequest{
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
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	return store
}

func beginSyncForTest(t *testing.T, store *Store, clientID string) BeginSyncResult {
	t.Helper()

	begin, err := store.BeginSync(context.Background(), BeginSyncRequest{
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
	if _, err := store.PlanSync(ctx, ManifestRequest{
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
	if err := store.StageUpload(ctx, begin.SessionID, clientID, path, hash, int64(len(content)), bytes.NewReader(content)); err != nil {
		t.Fatalf("stage test file upload: %v", err)
	}
	commit, err := store.CommitSync(ctx, CommitRequest{SessionID: begin.SessionID, ClientID: clientID})
	if err != nil {
		t.Fatalf("commit test file sync: %v", err)
	}

	return commit
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

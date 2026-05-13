package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
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

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

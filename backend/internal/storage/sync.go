package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	lockTTL               = 2 * time.Minute
	heartbeatAfter        = 10 * time.Second
	defaultClientName     = "Unknown client"
	syncSessionIDPrefix   = "sync_"
	conflictIDPrefix      = "conflict_"
	fileRevisionKindWrite = "write"
	fileRevisionKindDel   = "delete"
)

type remoteFile struct {
	Path         string
	CurrentHash  string
	PreviousHash string
	Size         int64
	Revision     int64
	Deleted      bool
}

type stagedUpload struct {
	Path        string
	ActualHash  string
	Size        int64
	StagingPath string
}

type syncSession struct {
	SessionID string
	UserID    string
	VaultID   string
	ClientID  string
	Status    string
}

// BeginSync acquires the selected vault's sync lock and opens a sync session.
func (s *Store) BeginSync(ctx context.Context, userID string, req BeginSyncRequest) (BeginSyncResult, error) {
	userID = strings.TrimSpace(userID)
	clientID := strings.TrimSpace(req.ClientID)
	clientName := strings.TrimSpace(req.ClientName)
	vaultID := strings.TrimSpace(req.VaultID)
	if userID == "" || clientID == "" || vaultID == "" {
		return BeginSyncResult{}, fmt.Errorf("%w: userId, clientId, and vaultId are required", ErrBadRequest)
	}
	if clientName == "" {
		clientName = defaultClientName
	}

	sessionID, err := randomID(syncSessionIDPrefix)
	if err != nil {
		return BeginSyncResult{}, err
	}

	now := time.Now().UTC()
	nowText := timestamp(now)
	expiresText := timestamp(now.Add(lockTTL))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BeginSyncResult{}, fmt.Errorf("begin sync transaction: %w", err)
	}
	defer rollback(tx)

	revision, err := ensureActiveVaultTx(ctx, tx, userID, vaultID)
	if err != nil {
		return BeginSyncResult{}, err
	}

	lockStatus, lockSessionID, expiresAt, err := currentLock(ctx, tx, vaultID)
	if err != nil {
		return BeginSyncResult{}, err
	}

	staleSessionID := ""
	if lockStatus == SyncStateSyncing {
		expired, err := isExpired(expiresAt, now)
		if err != nil {
			return BeginSyncResult{}, err
		}
		if !expired {
			return BeginSyncResult{}, ErrSyncLocked
		}
		if err := markSessionFailedTx(ctx, tx, lockSessionID, "SYNC_SESSION_STALE", "Sync lock expired."); err != nil {
			return BeginSyncResult{}, err
		}
		staleSessionID = lockSessionID
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_sessions (
			session_id,
			user_id,
			vault_id,
			client_id,
			client_name,
			status,
			started_at,
			base_server_revision
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, userID, vaultID, clientID, clientName, SessionStatusActive, nowText, revision); err != nil {
		return BeginSyncResult{}, fmt.Errorf("insert sync session: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sync_locks (
			vault_id,
			session_id,
			client_id,
			client_name,
			status,
			acquired_at,
			heartbeat_at,
			expires_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(vault_id) DO UPDATE SET
			session_id = excluded.session_id,
			client_id = excluded.client_id,
			client_name = excluded.client_name,
			status = excluded.status,
			acquired_at = excluded.acquired_at,
			heartbeat_at = excluded.heartbeat_at,
			expires_at = excluded.expires_at
	`, vaultID, sessionID, clientID, clientName, SyncStateSyncing, nowText, nowText, expiresText); err != nil {
		return BeginSyncResult{}, fmt.Errorf("acquire sync lock: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return BeginSyncResult{}, fmt.Errorf("commit sync begin transaction: %w", err)
	}

	if staleSessionID != "" {
		_ = os.RemoveAll(s.stagingSessionDir(staleSessionID))
	}

	return BeginSyncResult{
		SessionID:             sessionID,
		ServerRevision:        revision,
		HeartbeatAfterSeconds: int(heartbeatAfter.Seconds()),
	}, nil
}

// HeartbeatSync refreshes the active sync lock.
func (s *Store) HeartbeatSync(ctx context.Context, userID string, req HeartbeatRequest) error {
	session, err := s.ensureActiveSession(ctx, userID, req.SessionID, req.ClientID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE sync_locks
		SET heartbeat_at = ?, expires_at = ?
		WHERE vault_id = ? AND session_id = ? AND client_id = ? AND status = ?
	`, timestamp(now), timestamp(now.Add(lockTTL)), session.VaultID, req.SessionID, req.ClientID, SyncStateSyncing)
	if err != nil {
		return fmt.Errorf("refresh sync heartbeat: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check heartbeat update: %w", err)
	}
	if rows == 0 {
		return ErrSyncSessionNotFound
	}

	return nil
}

// PlanSync compares a local manifest against backend metadata and persists a sync plan.
func (s *Store) PlanSync(ctx context.Context, userID string, req ManifestRequest) (SyncPlan, error) {
	session, err := s.ensureActiveSession(ctx, userID, req.SessionID, req.ClientID)
	if err != nil {
		return SyncPlan{}, err
	}
	if strings.TrimSpace(req.VaultID) != "" && strings.TrimSpace(req.VaultID) != session.VaultID {
		return SyncPlan{}, fmt.Errorf("%w: manifest vaultId does not match session", ErrBadRequest)
	}

	localFiles, deletedPaths, err := normalizeManifest(req)
	if err != nil {
		return SyncPlan{}, err
	}

	remoteFiles, err := s.loadRemoteFiles(ctx, session.VaultID)
	if err != nil {
		return SyncPlan{}, err
	}

	actions := make([]PlanAction, 0)
	seen := map[string]bool{}

	for path, local := range localFiles {
		seen[path] = true
		remote, exists := remoteFiles[path]
		switch {
		case !exists:
			actions = append(actions, uploadAction(local))
		case remote.Deleted:
			if local.LastKnownRevision == remote.Revision {
				actions = append(actions, uploadAction(local))
			} else if remote.Revision > req.LastKnownServerRevision {
				if local.Hash == remote.PreviousHash {
					actions = append(actions, PlanAction{
						Type:          PlanActionDeleteLocal,
						Path:          path,
						RemoteHash:    remote.PreviousHash,
						Revision:      remote.Revision,
						RemoteDeleted: true,
					})
				} else {
					actions = append(actions, PlanAction{
						Type:          PlanActionConflict,
						Path:          path,
						ExpectedHash:  local.Hash,
						RemoteHash:    remote.PreviousHash,
						Size:          local.Size,
						Revision:      remote.Revision,
						RemoteDeleted: true,
					})
				}
			} else {
				actions = append(actions, uploadAction(local))
			}
		case local.Hash == remote.CurrentHash:
			actions = append(actions, PlanAction{Type: PlanActionNone, Path: path, ExpectedHash: local.Hash, Revision: remote.Revision})
		case local.LastKnownRevision == remote.Revision:
			actions = append(actions, uploadAction(local))
		case local.LastKnownRevision < remote.Revision:
			if local.Hash == remote.PreviousHash {
				actions = append(actions, PlanAction{
					Type:       PlanActionDownload,
					Path:       path,
					RemoteHash: remote.CurrentHash,
					Size:       remote.Size,
					Revision:   remote.Revision,
				})
			} else {
				actions = append(actions, PlanAction{
					Type:         PlanActionConflict,
					Path:         path,
					ExpectedHash: local.Hash,
					RemoteHash:   remote.CurrentHash,
					Size:         local.Size,
					Revision:     remote.Revision,
				})
			}
		default:
			actions = append(actions, PlanAction{
				Type:         PlanActionConflict,
				Path:         path,
				ExpectedHash: local.Hash,
				RemoteHash:   remote.CurrentHash,
				Size:         local.Size,
				Revision:     remote.Revision,
			})
		}
	}

	for path, deleted := range deletedPaths {
		seen[path] = true
		remote, exists := remoteFiles[path]
		if !exists || remote.Deleted {
			actions = append(actions, PlanAction{Type: PlanActionNone, Path: path})
			continue
		}
		if remote.Revision > req.LastKnownServerRevision && deleted.LastKnownRevision != remote.Revision {
			actions = append(actions, PlanAction{
				Type:       PlanActionConflict,
				Path:       path,
				RemoteHash: remote.CurrentHash,
				Revision:   remote.Revision,
			})
			continue
		}

		actions = append(actions, PlanAction{
			Type:       PlanActionDeleteRemote,
			Path:       path,
			RemoteHash: remote.CurrentHash,
			Revision:   remote.Revision,
		})
	}

	for path, remote := range remoteFiles {
		if seen[path] || remote.Deleted {
			continue
		}
		if remote.Revision > req.LastKnownServerRevision {
			actions = append(actions, PlanAction{
				Type:       PlanActionDownload,
				Path:       path,
				RemoteHash: remote.CurrentHash,
				Size:       remote.Size,
				Revision:   remote.Revision,
			})
		} else {
			actions = append(actions, PlanAction{
				Type:       PlanActionDeleteRemote,
				Path:       path,
				RemoteHash: remote.CurrentHash,
				Revision:   remote.Revision,
			})
		}
	}

	if err := s.replacePlan(ctx, session.VaultID, req.SessionID, actions); err != nil {
		return SyncPlan{}, err
	}

	revision, err := s.ServerRevision(ctx, userID, session.VaultID)
	if err != nil {
		return SyncPlan{}, err
	}

	return SyncPlan{
		SessionID:      req.SessionID,
		ServerRevision: revision,
		Actions:        actions,
	}, nil
}

// CommitSync validates staged uploads, updates remote metadata, and releases the lock.
func (s *Store) CommitSync(ctx context.Context, userID string, req CommitRequest) (CommitResult, error) {
	session, err := s.ensureActiveSession(ctx, userID, req.SessionID, req.ClientID)
	if err != nil {
		return CommitResult{}, err
	}

	actions, err := s.loadPlanActions(ctx, req.SessionID)
	if err != nil {
		return CommitResult{}, err
	}
	for _, action := range actions {
		if action.Type == PlanActionConflict {
			return CommitResult{}, ErrConflictDetected
		}
	}

	uploads, err := s.loadValidatedUploads(ctx, req.SessionID)
	if err != nil {
		return CommitResult{}, err
	}
	for _, action := range actions {
		if action.Type != PlanActionUpload {
			continue
		}
		upload, ok := uploads[action.Path]
		if !ok || upload.ActualHash != action.ExpectedHash {
			return CommitResult{}, fmt.Errorf("%w: missing validated upload for %s", ErrBadRequest, action.Path)
		}
		if err := s.ensureBlob(ctx, upload.ActualHash, upload.StagingPath); err != nil {
			return CommitResult{}, err
		}
	}

	mutatesRemote := false
	for _, action := range actions {
		if action.Type == PlanActionUpload || action.Type == PlanActionDeleteRemote {
			mutatesRemote = true
			break
		}
	}

	now := time.Now().UTC()
	nowText := timestamp(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommitResult{}, fmt.Errorf("begin commit transaction: %w", err)
	}
	defer rollback(tx)

	serverRevision, err := ensureActiveVaultTx(ctx, tx, userID, session.VaultID)
	if err != nil {
		return CommitResult{}, err
	}

	commitRevision := serverRevision
	if mutatesRemote {
		commitRevision++
	}

	for _, action := range actions {
		switch action.Type {
		case PlanActionUpload:
			upload := uploads[action.Path]
			if err := applyUploadTx(ctx, tx, session.VaultID, action, upload, commitRevision, nowText); err != nil {
				return CommitResult{}, err
			}
		case PlanActionDeleteRemote:
			if err := applyRemoteDeleteTx(ctx, tx, session.VaultID, action, req.SessionID, req.ClientID, commitRevision, nowText); err != nil {
				return CommitResult{}, err
			}
		}
	}

	if mutatesRemote {
		if _, err := tx.ExecContext(ctx, `
			UPDATE vaults
			SET revision = ?, updated_at = ?
			WHERE id = ? AND user_id = ? AND status = ?
		`, commitRevision, nowText, session.VaultID, userID, VaultStatusActive); err != nil {
			return CommitResult{}, fmt.Errorf("update vault revision: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_plan_actions
		SET status = ?, completed_at = ?
		WHERE session_id = ?
	`, PlanStatusCompleted, nowText, req.SessionID); err != nil {
		return CommitResult{}, fmt.Errorf("complete sync plan: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_sessions
		SET status = ?, completed_at = ?, commit_server_revision = ?
		WHERE session_id = ? AND user_id = ? AND client_id = ? AND status = ?
	`, SessionStatusCommitted, nowText, commitRevision, req.SessionID, userID, req.ClientID, SessionStatusActive); err != nil {
		return CommitResult{}, fmt.Errorf("complete sync session: %w", err)
	}

	if err := releaseLockTx(ctx, tx, session.VaultID, req.SessionID, req.ClientID, SyncStateIdle); err != nil {
		return CommitResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return CommitResult{}, fmt.Errorf("commit sync transaction: %w", err)
	}

	_ = os.RemoveAll(s.stagingSessionDir(req.SessionID))
	return CommitResult{ServerRevision: commitRevision}, nil
}

// AbortSync aborts an active sync session and releases the lock.
func (s *Store) AbortSync(ctx context.Context, userID string, req AbortRequest) error {
	session, err := s.ensureSessionOwnership(ctx, userID, req.SessionID, req.ClientID)
	if err != nil {
		return err
	}

	nowText := timestamp(time.Now())
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "Sync aborted."
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin abort transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_sessions
		SET status = ?, completed_at = ?, error_code = ?, error_message = ?
		WHERE session_id = ? AND user_id = ? AND client_id = ? AND status = ?
	`, SessionStatusAborted, nowText, "SYNC_ABORTED", reason, req.SessionID, userID, req.ClientID, SessionStatusActive); err != nil {
		return fmt.Errorf("abort sync session: %w", err)
	}

	if err := releaseLockTx(ctx, tx, session.VaultID, req.SessionID, req.ClientID, SyncStateIdle); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit abort transaction: %w", err)
	}

	_ = os.RemoveAll(s.stagingSessionDir(req.SessionID))
	return nil
}

// DownloadFile returns metadata for a finalized remote file.
func (s *Store) DownloadFile(ctx context.Context, userID string, vaultID string, vaultPath string) (DownloadResult, error) {
	if _, err := s.VaultByID(ctx, userID, vaultID); err != nil {
		return DownloadResult{}, err
	}

	normalizedPath, err := NormalizeVaultPath(vaultPath)
	if err != nil {
		return DownloadResult{}, err
	}

	var result DownloadResult
	if err := s.db.QueryRowContext(ctx, `
		SELECT path, current_hash, size, revision
		FROM files
		WHERE vault_id = ? AND path = ? AND deleted = 0
	`, vaultID, normalizedPath).Scan(&result.Path, &result.Hash, &result.Size, &result.Revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DownloadResult{}, ErrNotFound
		}
		return DownloadResult{}, fmt.Errorf("load file metadata: %w", err)
	}

	return result, nil
}

func normalizeManifest(req ManifestRequest) (map[string]ManifestFile, map[string]ManifestFile, error) {
	deletedPaths := make(map[string]ManifestFile, len(req.DeletedPaths))
	for _, rawPath := range req.DeletedPaths {
		normalizedPath, err := NormalizeVaultPath(rawPath)
		if err != nil {
			return nil, nil, err
		}
		deletedPaths[normalizedPath] = ManifestFile{
			Path:              normalizedPath,
			LastKnownRevision: req.LastKnownServerRevision,
			Deleted:           true,
		}
	}

	localFiles := make(map[string]ManifestFile, len(req.Files))
	for _, file := range req.Files {
		normalizedPath, err := NormalizeVaultPath(file.Path)
		if err != nil {
			return nil, nil, err
		}
		if file.Deleted {
			file.Path = normalizedPath
			deletedPaths[normalizedPath] = file
			continue
		}
		if err := validateSHA256(file.Hash); err != nil {
			return nil, nil, err
		}
		if file.Size < 0 {
			return nil, nil, fmt.Errorf("%w: file size cannot be negative", ErrBadRequest)
		}
		if _, exists := localFiles[normalizedPath]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate manifest path", ErrBadRequest)
		}
		file.Path = normalizedPath
		localFiles[normalizedPath] = file
	}

	return localFiles, deletedPaths, nil
}

func uploadAction(file ManifestFile) PlanAction {
	return PlanAction{
		Type:         PlanActionUpload,
		Path:         file.Path,
		ExpectedHash: file.Hash,
		Size:         file.Size,
		Revision:     file.LastKnownRevision,
	}
}

func (s *Store) loadRemoteFiles(ctx context.Context, vaultID string) (map[string]remoteFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT path, COALESCE(current_hash, ''), COALESCE(previous_hash, ''), size, revision, deleted
		FROM files
		WHERE vault_id = ?
	`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("load remote files: %w", err)
	}
	defer rows.Close()

	files := map[string]remoteFile{}
	for rows.Next() {
		var file remoteFile
		var deleted int
		if err := rows.Scan(&file.Path, &file.CurrentHash, &file.PreviousHash, &file.Size, &file.Revision, &deleted); err != nil {
			return nil, fmt.Errorf("scan remote file: %w", err)
		}
		file.Deleted = deleted == 1
		files[file.Path] = file
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remote files: %w", err)
	}

	return files, nil
}

func (s *Store) replacePlan(ctx context.Context, vaultID string, sessionID string, actions []PlanAction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin plan transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM sync_plan_actions WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("clear old sync plan: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM conflicts WHERE session_id = ? AND status = 'PENDING'`, sessionID); err != nil {
		return fmt.Errorf("clear old conflicts: %w", err)
	}

	nowText := timestamp(time.Now())
	for _, action := range actions {
		status := PlanStatusPending
		if action.Type == PlanActionConflict {
			status = PlanStatusConflict
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sync_plan_actions (
				session_id,
				action_type,
				path,
				expected_hash,
				remote_hash,
				base_hash,
				size,
				revision,
				status,
				created_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sessionID, action.Type, action.Path, action.ExpectedHash, action.RemoteHash, action.BaseHash, action.Size, action.Revision, status, nowText); err != nil {
			return fmt.Errorf("insert sync plan action: %w", err)
		}

		if action.Type == PlanActionConflict {
			conflictID, err := randomID(conflictIDPrefix)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO conflicts (
					id,
					vault_id,
					path,
					session_id,
					local_hash,
					remote_hash,
					base_hash,
					kind,
					status,
					created_at
				)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, conflictID, vaultID, action.Path, sessionID, action.ExpectedHash, action.RemoteHash, action.BaseHash, "file", "PENDING", nowText); err != nil {
				return fmt.Errorf("insert conflict: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit plan transaction: %w", err)
	}

	return nil
}

func (s *Store) loadPlanActions(ctx context.Context, sessionID string) ([]PlanAction, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT action_type, path, COALESCE(expected_hash, ''), COALESCE(remote_hash, ''), COALESCE(base_hash, ''), size, revision
		FROM sync_plan_actions
		WHERE session_id = ?
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load sync plan actions: %w", err)
	}
	defer rows.Close()

	actions := []PlanAction{}
	for rows.Next() {
		var action PlanAction
		if err := rows.Scan(&action.Type, &action.Path, &action.ExpectedHash, &action.RemoteHash, &action.BaseHash, &action.Size, &action.Revision); err != nil {
			return nil, fmt.Errorf("scan sync plan action: %w", err)
		}
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync plan actions: %w", err)
	}

	return actions, nil
}

func (s *Store) loadValidatedUploads(ctx context.Context, sessionID string) (map[string]stagedUpload, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT path, actual_hash, size, staging_path
		FROM staged_uploads
		WHERE session_id = ? AND status = ?
	`, sessionID, UploadStatusValidated)
	if err != nil {
		return nil, fmt.Errorf("load staged uploads: %w", err)
	}
	defer rows.Close()

	uploads := map[string]stagedUpload{}
	for rows.Next() {
		var upload stagedUpload
		if err := rows.Scan(&upload.Path, &upload.ActualHash, &upload.Size, &upload.StagingPath); err != nil {
			return nil, fmt.Errorf("scan staged upload: %w", err)
		}
		uploads[upload.Path] = upload
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate staged uploads: %w", err)
	}

	return uploads, nil
}

func applyUploadTx(ctx context.Context, tx *sql.Tx, vaultID string, action PlanAction, upload stagedUpload, revision int64, nowText string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO file_revisions (vault_id, path, hash, size, revision, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, vaultID, action.Path, upload.ActualHash, upload.Size, revision, fileRevisionKindWrite, nowText); err != nil {
		return fmt.Errorf("insert file revision: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO files (vault_id, path, current_hash, previous_hash, size, revision, deleted, updated_at)
		VALUES (?, ?, ?, NULL, ?, ?, 0, ?)
		ON CONFLICT(vault_id, path) DO UPDATE SET
			previous_hash = COALESCE(files.current_hash, files.previous_hash),
			current_hash = excluded.current_hash,
			size = excluded.size,
			revision = excluded.revision,
			deleted = 0,
			updated_at = excluded.updated_at
	`, vaultID, action.Path, upload.ActualHash, upload.Size, revision, nowText); err != nil {
		return fmt.Errorf("upsert file metadata: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tombstones WHERE vault_id = ? AND path = ?`, vaultID, action.Path); err != nil {
		return fmt.Errorf("clear tombstone: %w", err)
	}

	return nil
}

func applyRemoteDeleteTx(ctx context.Context, tx *sql.Tx, vaultID string, action PlanAction, sessionID string, clientID string, revision int64, nowText string) error {
	var previousHash sql.NullString
	var size int64
	if err := tx.QueryRowContext(ctx, `
		SELECT current_hash, size
		FROM files
		WHERE vault_id = ? AND path = ?
	`, vaultID, action.Path).Scan(&previousHash, &size); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load deleted file metadata: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO file_revisions (vault_id, path, hash, size, revision, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, vaultID, action.Path, previousHash.String, size, revision, fileRevisionKindDel, nowText); err != nil {
		return fmt.Errorf("insert delete revision: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO files (vault_id, path, current_hash, previous_hash, size, revision, deleted, updated_at)
		VALUES (?, ?, NULL, ?, 0, ?, 1, ?)
		ON CONFLICT(vault_id, path) DO UPDATE SET
			previous_hash = files.current_hash,
			current_hash = NULL,
			size = 0,
			revision = excluded.revision,
			deleted = 1,
			updated_at = excluded.updated_at
	`, vaultID, action.Path, previousHash.String, revision, nowText); err != nil {
		return fmt.Errorf("mark file deleted: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tombstones (vault_id, path, revision, deleted_at, client_id, session_id)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(vault_id, path) DO UPDATE SET
			revision = excluded.revision,
			deleted_at = excluded.deleted_at,
			client_id = excluded.client_id,
			session_id = excluded.session_id
	`, vaultID, action.Path, revision, nowText, clientID, sessionID); err != nil {
		return fmt.Errorf("upsert tombstone: %w", err)
	}

	return nil
}

func (s *Store) uploadPlanned(ctx context.Context, sessionID string, vaultPath string, expectedHash string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM sync_plan_actions
		WHERE session_id = ? AND path = ? AND expected_hash = ? AND action_type = ?
	`, sessionID, vaultPath, expectedHash, PlanActionUpload).Scan(&count); err != nil {
		return false, fmt.Errorf("check upload plan: %w", err)
	}

	return count > 0, nil
}

func (s *Store) ensureActiveSession(ctx context.Context, userID string, sessionID string, clientID string) (syncSession, error) {
	session, err := s.ensureSessionOwnership(ctx, userID, sessionID, clientID)
	if err != nil {
		return syncSession{}, err
	}

	var lockSessionID string
	var lockClientID string
	var lockStatus string
	var expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(session_id, ''),
			COALESCE(client_id, ''),
			status,
			COALESCE(expires_at, '')
		FROM sync_locks
		WHERE vault_id = ?
	`, session.VaultID).Scan(&lockSessionID, &lockClientID, &lockStatus, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return syncSession{}, ErrSyncSessionNotFound
		}
		return syncSession{}, fmt.Errorf("load sync lock: %w", err)
	}

	if lockSessionID != strings.TrimSpace(sessionID) || lockClientID != strings.TrimSpace(clientID) || lockStatus != SyncStateSyncing {
		return syncSession{}, ErrSyncSessionNotFound
	}

	expired, err := isExpired(expiresAt, time.Now().UTC())
	if err != nil {
		return syncSession{}, err
	}
	if expired {
		if _, err := s.markSessionStale(ctx, session.VaultID, sessionID); err != nil {
			return syncSession{}, err
		}
		return syncSession{}, ErrSyncSessionStale
	}

	return session, nil
}

func (s *Store) ensureSessionOwnership(ctx context.Context, userID string, sessionID string, clientID string) (syncSession, error) {
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	clientID = strings.TrimSpace(clientID)
	if userID == "" || sessionID == "" || clientID == "" {
		return syncSession{}, fmt.Errorf("%w: userId, sessionId, and clientId are required", ErrBadRequest)
	}

	var session syncSession
	if err := s.db.QueryRowContext(ctx, `
		SELECT session_id, user_id, vault_id, client_id, status
		FROM sync_sessions
		WHERE session_id = ? AND user_id = ?
	`, sessionID, userID).Scan(&session.SessionID, &session.UserID, &session.VaultID, &session.ClientID, &session.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return syncSession{}, ErrSyncSessionNotFound
		}
		return syncSession{}, fmt.Errorf("load sync session: %w", err)
	}

	if session.ClientID != clientID {
		return syncSession{}, ErrSyncSessionNotFound
	}
	if session.Status != SessionStatusActive {
		return syncSession{}, ErrSyncSessionStale
	}

	return session, nil
}

// SessionVaultID returns the vault linked to a user's sync session.
func (s *Store) SessionVaultID(ctx context.Context, userID string, sessionID string) (string, error) {
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	if userID == "" || sessionID == "" {
		return "", fmt.Errorf("%w: userId and sessionId are required", ErrBadRequest)
	}
	var vaultID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT vault_id
		FROM sync_sessions
		WHERE session_id = ? AND user_id = ?
	`, sessionID, userID).Scan(&vaultID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSyncSessionNotFound
		}
		return "", fmt.Errorf("load session vault: %w", err)
	}
	return vaultID, nil
}

func currentLock(ctx context.Context, tx *sql.Tx, vaultID string) (string, string, string, error) {
	var status string
	var sessionID string
	var expiresAt string
	if err := tx.QueryRowContext(ctx, `
		SELECT status, COALESCE(session_id, ''), COALESCE(expires_at, '')
		FROM sync_locks
		WHERE vault_id = ?
	`, vaultID).Scan(&status, &sessionID, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncStateIdle, "", "", nil
		}
		return "", "", "", fmt.Errorf("load current lock: %w", err)
	}

	return status, sessionID, expiresAt, nil
}

func isExpired(value string, now time.Time) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return true, nil
	}

	return !expiresAt.After(now), nil
}

func markSessionFailedTx(ctx context.Context, tx *sql.Tx, sessionID string, code string, message string) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_sessions
		SET status = ?, completed_at = ?, error_code = ?, error_message = ?
		WHERE session_id = ? AND status = ?
	`, SessionStatusFailed, timestamp(time.Now()), code, message, sessionID, SessionStatusActive); err != nil {
		return fmt.Errorf("mark session failed: %w", err)
	}

	return nil
}

func (s *Store) markSessionStale(ctx context.Context, vaultID string, sessionID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin stale session transaction: %w", err)
	}
	defer rollback(tx)

	if err := markSessionFailedTx(ctx, tx, sessionID, "SYNC_SESSION_STALE", "Sync lock expired."); err != nil {
		return false, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE sync_locks
		SET status = ?, session_id = NULL, client_id = NULL, client_name = NULL, acquired_at = NULL, heartbeat_at = NULL, expires_at = NULL
		WHERE vault_id = ? AND session_id = ?
	`, SyncStateStaleLock, vaultID, sessionID)
	if err != nil {
		return false, fmt.Errorf("mark lock stale: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check stale lock update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit stale session transaction: %w", err)
	}

	_ = os.RemoveAll(s.stagingSessionDir(sessionID))
	return rowsAffected > 0, nil
}

func releaseLockTx(ctx context.Context, tx *sql.Tx, vaultID string, sessionID string, clientID string, status string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE sync_locks
		SET status = ?, session_id = NULL, client_id = NULL, client_name = NULL, acquired_at = NULL, heartbeat_at = NULL, expires_at = NULL
		WHERE vault_id = ? AND session_id = ? AND client_id = ?
	`, status, vaultID, sessionID, clientID); err != nil {
		return fmt.Errorf("release sync lock: %w", err)
	}
	return nil
}

func (s *Store) stagingSessionDir(sessionID string) string {
	return filepath.Join(s.dataDir, "staging", sessionID)
}

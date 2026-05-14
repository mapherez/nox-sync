package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ServerRevision returns the current backend revision for one active vault.
func (s *Store) ServerRevision(ctx context.Context, userID string, vaultID string) (int64, error) {
	vault, err := s.VaultByID(ctx, userID, vaultID)
	if err != nil {
		return 0, err
	}
	return vault.Revision, nil
}

// SyncStatus returns the current sync lock state for one vault.
func (s *Store) SyncStatus(ctx context.Context, userID string, vaultID string) (SyncStatus, error) {
	status, _, err := s.RefreshSyncStatus(ctx, userID, vaultID)
	return status, err
}

// RefreshSyncStatus returns one vault's sync lock state and reports whether it was reaped.
func (s *Store) RefreshSyncStatus(ctx context.Context, userID string, vaultID string) (SyncStatus, bool, error) {
	if _, err := s.VaultByID(ctx, userID, vaultID); err != nil {
		return SyncStatus{}, false, err
	}

	reapedVaultIDs, err := s.ReapExpiredLocks(ctx)
	if err != nil {
		return SyncStatus{}, false, err
	}
	reaped := false
	for _, reapedVaultID := range reapedVaultIDs {
		if reapedVaultID == vaultID {
			reaped = true
			break
		}
	}

	var status SyncStatus
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			status,
			COALESCE(session_id, ''),
			COALESCE(client_id, ''),
			COALESCE(client_name, ''),
			COALESCE(acquired_at, '')
		FROM sync_locks
		WHERE vault_id = ?
	`, vaultID).Scan(
		&status.State,
		&status.SessionID,
		&status.ClientID,
		&status.ClientName,
		&status.StartedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncStatus{State: SyncStateIdle}, reaped, nil
		}
		return SyncStatus{}, false, fmt.Errorf("load sync status: %w", err)
	}

	return status, reaped, nil
}

// ReapExpiredLocks marks all expired active sync locks stale and removes their abandoned staging data.
func (s *Store) ReapExpiredLocks(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vault_id, COALESCE(session_id, ''), COALESCE(expires_at, '')
		FROM sync_locks
		WHERE status = ?
	`, SyncStateSyncing)
	if err != nil {
		return nil, fmt.Errorf("load lock expiries: %w", err)
	}
	defer rows.Close()

	type expiredLock struct {
		vaultID   string
		sessionID string
	}
	expiredLocks := []expiredLock{}
	now := time.Now().UTC()
	for rows.Next() {
		var vaultID string
		var sessionID string
		var expiresAt string
		if err := rows.Scan(&vaultID, &sessionID, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan lock expiry: %w", err)
		}
		expired, err := isExpired(expiresAt, now)
		if err != nil {
			return nil, err
		}
		if expired {
			expiredLocks = append(expiredLocks, expiredLock{vaultID: vaultID, sessionID: sessionID})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lock expiries: %w", err)
	}

	reaped := []string{}
	for _, lock := range expiredLocks {
		ok, err := s.markSessionStale(ctx, lock.vaultID, lock.sessionID)
		if err != nil {
			return nil, err
		}
		if ok {
			reaped = append(reaped, lock.vaultID)
		}
	}
	return reaped, nil
}

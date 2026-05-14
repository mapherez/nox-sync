package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ServerRevision returns the current backend vault revision.
func (s *Store) ServerRevision(ctx context.Context) (int64, error) {
	var revision int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT revision
		FROM server_state
		WHERE id = 1
	`).Scan(&revision); err != nil {
		return 0, fmt.Errorf("load server revision: %w", err)
	}

	return revision, nil
}

// SyncStatus returns the current sync lock state.
func (s *Store) SyncStatus(ctx context.Context) (SyncStatus, error) {
	status, _, err := s.RefreshSyncStatus(ctx)
	return status, err
}

// RefreshSyncStatus returns the current sync lock state and reports whether an expired lock was reaped.
func (s *Store) RefreshSyncStatus(ctx context.Context) (SyncStatus, bool, error) {
	reaped, err := s.ReapExpiredLock(ctx)
	if err != nil {
		return SyncStatus{}, false, err
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
		WHERE id = 1
	`).Scan(
		&status.State,
		&status.SessionID,
		&status.ClientID,
		&status.ClientName,
		&status.StartedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncStatus{State: "IDLE"}, reaped, nil
		}
		return SyncStatus{}, false, fmt.Errorf("load sync status: %w", err)
	}

	return status, reaped, nil
}

// ReapExpiredLock marks an expired active sync lock stale and removes its abandoned staging data.
func (s *Store) ReapExpiredLock(ctx context.Context) (bool, error) {
	var sessionID string
	var status string
	var expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(session_id, ''), status, COALESCE(expires_at, '')
		FROM sync_locks
		WHERE id = 1
	`).Scan(&sessionID, &status, &expiresAt); err != nil {
		return false, fmt.Errorf("load lock expiry: %w", err)
	}

	if status != SyncStateSyncing {
		return false, nil
	}

	expired, err := isExpired(expiresAt, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if !expired {
		return false, nil
	}

	return s.markSessionStale(ctx, sessionID)
}

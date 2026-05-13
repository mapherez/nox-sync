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
	if err := s.reapExpiredLock(ctx); err != nil {
		return SyncStatus{}, err
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
			return SyncStatus{State: "IDLE"}, nil
		}
		return SyncStatus{}, fmt.Errorf("load sync status: %w", err)
	}

	return status, nil
}

func (s *Store) reapExpiredLock(ctx context.Context) error {
	var sessionID string
	var status string
	var expiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(session_id, ''), status, COALESCE(expires_at, '')
		FROM sync_locks
		WHERE id = 1
	`).Scan(&sessionID, &status, &expiresAt); err != nil {
		return fmt.Errorf("load lock expiry: %w", err)
	}

	if status != SyncStateSyncing {
		return nil
	}

	expired, err := isExpired(expiresAt, time.Now().UTC())
	if err != nil {
		return err
	}
	if !expired {
		return nil
	}

	return s.markSessionStale(ctx, sessionID)
}

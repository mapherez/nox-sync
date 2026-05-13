package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

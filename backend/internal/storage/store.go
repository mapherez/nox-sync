package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DatabaseFileName = "nox-sync.db"
)

// Store owns access to backend metadata stored in SQLite.
type Store struct {
	db      *sql.DB
	dataDir string
}

// SyncStatus is the lightweight sync status shape used by the status API.
type SyncStatus struct {
	State      string
	SessionID  string
	ClientID   string
	ClientName string
	StartedAt  string
}

// Open opens the SQLite database, applies migrations, and ensures required
// singleton rows exist.
func Open(ctx context.Context, dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data directory is required")
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, DatabaseFileName)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &Store{db: db, dataDir: dataDir}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.ensureSingletonRows(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// Close closes the SQLite connection pool.
func (s *Store) Close() error {
	return s.db.Close()
}

// DBPath returns the absolute database path used by this store.
func (s *Store) DBPath() string {
	return filepath.Join(s.dataDir, DatabaseFileName)
}

func (s *Store) configure(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	}

	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply sqlite pragma %q: %w", pragma, err)
		}
	}

	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}

	return nil
}

func (s *Store) ensureSingletonRows(ctx context.Context) error {
	now := timestamp(time.Now())

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin singleton setup transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO server_state (id, revision, updated_at)
		VALUES (1, 0, ?)
	`, now); err != nil {
		return fmt.Errorf("ensure server_state row: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO sync_locks (id, status)
		VALUES (1, 'IDLE')
	`); err != nil {
		return fmt.Errorf("ensure sync_locks row: %w", err)
	}

	if err := ensureAPIKeyTx(ctx, tx, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit singleton setup transaction: %w", err)
	}

	return nil
}

func timestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

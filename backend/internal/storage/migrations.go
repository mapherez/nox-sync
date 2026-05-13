package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var migrationFilePattern = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

type migration struct {
	version int
	name    string
	path    string
}

// Migrate applies all embedded SQLite migrations in numeric order.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		applied, err := s.isMigrationApplied(ctx, migration.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		body, err := migrationFiles.ReadFile(migration.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.path, err)
		}

		if err := s.applyMigration(ctx, migration, string(body)); err != nil {
			return err
		}
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    matches[2],
			path:    "migrations/" + entry.Name(),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func (s *Store) isMigrationApplied(ctx context.Context, version int) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM schema_migrations
		WHERE version = ?
	`, version).Scan(&count); err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}

	return count > 0, nil
}

func (s *Store) applyMigration(ctx context.Context, migration migration, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.version, err)
	}
	defer rollback(tx)

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("apply migration %d: %w", migration.version, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, applied_at)
		VALUES (?, ?, ?)
	`, migration.version, migration.name, timestamp(time.Now())); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.version, err)
	}

	return nil
}

type txAPI interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

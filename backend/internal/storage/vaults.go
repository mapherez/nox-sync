package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const vaultIDPrefix = "vault_"

// ListVaults returns active vaults owned by one user.
func (s *Store) ListVaults(ctx context.Context, userID string) ([]Vault, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: userId is required", ErrBadRequest)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, name, revision, status, updated_at, COALESCE(deleted_at, '')
		FROM vaults
		WHERE user_id = ? AND status = ?
		ORDER BY updated_at DESC, name ASC
	`, userID, VaultStatusActive)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	defer rows.Close()

	vaults := []Vault{}
	for rows.Next() {
		vault, err := scanVault(rows)
		if err != nil {
			return nil, err
		}
		vaults = append(vaults, vault)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate vaults: %w", err)
	}
	return vaults, nil
}

// CreateVault creates a new active vault for one user.
func (s *Store) CreateVault(ctx context.Context, userID string, name string) (Vault, error) {
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID == "" {
		return Vault{}, fmt.Errorf("%w: userId is required", ErrBadRequest)
	}
	if name == "" {
		return Vault{}, fmt.Errorf("%w: vault name is required", ErrBadRequest)
	}
	if len(name) > 160 {
		return Vault{}, fmt.Errorf("%w: vault name is too long", ErrBadRequest)
	}

	vaultID, err := randomID(vaultIDPrefix)
	if err != nil {
		return Vault{}, err
	}
	now := timestamp(time.Now())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO vaults (id, user_id, name, revision, status, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?, ?)
	`, vaultID, userID, name, VaultStatusActive, now, now); err != nil {
		return Vault{}, fmt.Errorf("create vault: %w", err)
	}

	return s.VaultByID(ctx, userID, vaultID)
}

// VaultByID returns one active vault owned by the user.
func (s *Store) VaultByID(ctx context.Context, userID string, vaultID string) (Vault, error) {
	userID = strings.TrimSpace(userID)
	vaultID = strings.TrimSpace(vaultID)
	if userID == "" || vaultID == "" {
		return Vault{}, fmt.Errorf("%w: userId and vaultId are required", ErrBadRequest)
	}

	var vault Vault
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, revision, status, updated_at, COALESCE(deleted_at, '')
		FROM vaults
		WHERE user_id = ? AND id = ? AND status = ?
	`, userID, vaultID, VaultStatusActive).Scan(
		&vault.ID,
		&vault.UserID,
		&vault.Name,
		&vault.Revision,
		&vault.Status,
		&vault.UpdatedAt,
		&vault.DeletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Vault{}, ErrNotFound
		}
		return Vault{}, fmt.Errorf("load vault: %w", err)
	}
	return vault, nil
}

// VaultOwnerID returns the owner for a non-deleted vault.
func (s *Store) VaultOwnerID(ctx context.Context, vaultID string) (string, error) {
	vaultID = strings.TrimSpace(vaultID)
	if vaultID == "" {
		return "", fmt.Errorf("%w: vaultId is required", ErrBadRequest)
	}

	var userID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT user_id
		FROM vaults
		WHERE id = ? AND status = ?
	`, vaultID, VaultStatusActive).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("load vault owner: %w", err)
	}
	return userID, nil
}

// SoftDeleteVault hides a vault and blocks future sync/download attempts.
func (s *Store) SoftDeleteVault(ctx context.Context, userID string, vaultID string) error {
	userID = strings.TrimSpace(userID)
	vaultID = strings.TrimSpace(vaultID)
	if userID == "" || vaultID == "" {
		return fmt.Errorf("%w: userId and vaultId are required", ErrBadRequest)
	}

	now := timestamp(time.Now())
	result, err := s.db.ExecContext(ctx, `
		UPDATE vaults
		SET status = ?, deleted_at = ?, updated_at = ?
		WHERE user_id = ? AND id = ? AND status = ?
	`, VaultStatusDeleted, now, now, userID, vaultID, VaultStatusActive)
	if err != nil {
		return fmt.Errorf("soft delete vault: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check vault delete: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// CurrentVaultFiles returns all current non-deleted file metadata for zip export.
func (s *Store) CurrentVaultFiles(ctx context.Context, userID string, vaultID string) ([]DownloadResult, error) {
	if _, err := s.VaultByID(ctx, userID, vaultID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT path, current_hash, size, revision
		FROM files
		WHERE vault_id = ? AND deleted = 0
		ORDER BY path ASC
	`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list current vault files: %w", err)
	}
	defer rows.Close()

	files := []DownloadResult{}
	for rows.Next() {
		var file DownloadResult
		if err := rows.Scan(&file.Path, &file.Hash, &file.Size, &file.Revision); err != nil {
			return nil, fmt.Errorf("scan current vault file: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current vault files: %w", err)
	}
	return files, nil
}

func ensureActiveVaultTx(ctx context.Context, tx *sql.Tx, userID string, vaultID string) (int64, error) {
	var revision int64
	if err := tx.QueryRowContext(ctx, `
		SELECT revision
		FROM vaults
		WHERE user_id = ? AND id = ? AND status = ?
	`, userID, vaultID, VaultStatusActive).Scan(&revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("load active vault: %w", err)
	}
	return revision, nil
}

func scanVault(rows interface {
	Scan(dest ...any) error
}) (Vault, error) {
	var vault Vault
	if err := rows.Scan(
		&vault.ID,
		&vault.UserID,
		&vault.Name,
		&vault.Revision,
		&vault.Status,
		&vault.UpdatedAt,
		&vault.DeletedAt,
	); err != nil {
		return Vault{}, fmt.Errorf("scan vault: %w", err)
	}
	return vault, nil
}

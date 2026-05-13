package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const APIKeyPrefix = "noxsync_"

// CurrentAPIKey returns the active API key so the admin page can display it.
func (s *Store) CurrentAPIKey(ctx context.Context) (string, error) {
	var key string
	if err := s.db.QueryRowContext(ctx, `
		SELECT key_value
		FROM api_keys
		WHERE id = 1
	`).Scan(&key); err != nil {
		return "", fmt.Errorf("load current API key: %w", err)
	}

	return key, nil
}

// RotateAPIKey replaces the active API key and invalidates the previous key.
func (s *Store) RotateAPIKey(ctx context.Context) (string, error) {
	now := timestamp(time.Now())
	key, keyHash, err := newAPIKey()
	if err != nil {
		return "", err
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, key_prefix, key_value, key_hash, created_at, rotated_at)
		VALUES (1, ?, ?, ?, ?, NULL)
		ON CONFLICT(id) DO UPDATE SET
			key_prefix = excluded.key_prefix,
			key_value = excluded.key_value,
			key_hash = excluded.key_hash,
			rotated_at = excluded.created_at
	`, APIKeyPrefix, key, keyHash, now); err != nil {
		return "", fmt.Errorf("rotate API key: %w", err)
	}

	return key, nil
}

// ValidateAPIKey checks whether a submitted API key matches the active key.
func (s *Store) ValidateAPIKey(ctx context.Context, submitted string) (bool, error) {
	submitted = strings.TrimSpace(submitted)
	if !strings.HasPrefix(submitted, APIKeyPrefix) {
		return false, nil
	}

	var expectedHash string
	if err := s.db.QueryRowContext(ctx, `
		SELECT key_hash
		FROM api_keys
		WHERE id = 1
	`).Scan(&expectedHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("load API key hash: %w", err)
	}

	submittedHash := hashAPIKey(submitted)
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(submittedHash)) == 1, nil
}

func ensureAPIKeyTx(ctx context.Context, tx txAPI, now string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM api_keys
		WHERE id = 1
	`).Scan(&count); err != nil {
		return fmt.Errorf("check API key row: %w", err)
	}

	if count > 0 {
		return nil
	}

	key, keyHash, err := newAPIKey()
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO api_keys (id, key_prefix, key_value, key_hash, created_at)
		VALUES (1, ?, ?, ?, ?)
	`, APIKeyPrefix, key, keyHash, now); err != nil {
		return fmt.Errorf("insert initial API key: %w", err)
	}

	return nil
}

func newAPIKey() (string, string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", fmt.Errorf("generate API key: %w", err)
	}

	key := APIKeyPrefix + base64.RawURLEncoding.EncodeToString(random)
	return key, hashAPIKey(key), nil
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

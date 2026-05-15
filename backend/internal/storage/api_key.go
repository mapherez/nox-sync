package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const APIKeyPrefix = "noxsync_"

// CurrentAPIKey returns the active API key for a dashboard user.
func (s *Store) CurrentAPIKey(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("%w: userId is required", ErrBadRequest)
	}

	var key string
	if err := s.db.QueryRowContext(ctx, `
		SELECT key_value
		FROM api_keys
		WHERE user_id = ?
	`, userID).Scan(&key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s.RotateAPIKey(ctx, userID)
		}
		return "", fmt.Errorf("load current API key: %w", err)
	}

	return key, nil
}

// RotateAPIKey replaces one user's active API key and invalidates the previous key.
func (s *Store) RotateAPIKey(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("%w: userId is required", ErrBadRequest)
	}

	now := timestamp(time.Now())
	key, keyHash, err := newAPIKey()
	if err != nil {
		return "", err
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (user_id, key_prefix, key_value, key_hash, created_at, rotated_at)
		VALUES (?, ?, ?, ?, ?, NULL)
		ON CONFLICT(user_id) DO UPDATE SET
			key_prefix = excluded.key_prefix,
			key_value = excluded.key_value,
			key_hash = excluded.key_hash,
			rotated_at = excluded.created_at
	`, userID, APIKeyPrefix, key, keyHash, now); err != nil {
		return "", fmt.Errorf("rotate API key: %w", err)
	}

	return key, nil
}

// AuthenticateAPIKey resolves a submitted plugin API key to exactly one active user.
func (s *Store) AuthenticateAPIKey(ctx context.Context, submitted string) (User, bool, error) {
	submitted = strings.TrimSpace(submitted)
	if !strings.HasPrefix(submitted, APIKeyPrefix) {
		return User{}, false, nil
	}

	var user User
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			u.id,
			COALESCE(u.google_sub, ''),
			u.email,
			u.first_name,
			u.display_name,
			u.role,
			u.status,
			COALESCE(u.last_login_at, '')
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.key_hash = ? AND u.status = ?
	`, hashAPIKey(submitted), UserStatusActive).Scan(
		&user.ID,
		&user.GoogleSub,
		&user.Email,
		&user.FirstName,
		&user.DisplayName,
		&user.Role,
		&user.Status,
		&user.LastLoginAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, false, nil
		}
		return User{}, false, fmt.Errorf("load API key user: %w", err)
	}

	return user, true, nil
}

// ValidateAPIKey checks whether a submitted API key belongs to an active user.
func (s *Store) ValidateAPIKey(ctx context.Context, submitted string) (bool, error) {
	_, ok, err := s.AuthenticateAPIKey(ctx, submitted)
	return ok, err
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

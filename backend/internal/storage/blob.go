package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StageUpload writes an upload to staging, validates its SHA-256 hash, and
// records it against the active sync plan.
func (s *Store) StageUpload(ctx context.Context, userID string, sessionID string, clientID string, vaultPath string, expectedHash string, expectedSize int64, body io.Reader) error {
	userID = stringsTrim(userID)
	sessionID = stringsTrim(sessionID)
	clientID = stringsTrim(clientID)
	expectedHash = stringsTrim(expectedHash)
	if userID == "" || sessionID == "" || clientID == "" {
		return fmt.Errorf("%w: userId, sessionId, and clientId are required", ErrBadRequest)
	}
	if err := validateSHA256(expectedHash); err != nil {
		return err
	}

	normalizedPath, err := NormalizeVaultPath(vaultPath)
	if err != nil {
		return err
	}

	if _, err := s.ensureActiveSession(ctx, userID, sessionID, clientID); err != nil {
		return err
	}

	planned, err := s.uploadPlanned(ctx, sessionID, normalizedPath, expectedHash)
	if err != nil {
		return err
	}
	if !planned {
		return fmt.Errorf("%w: upload was not in the sync plan", ErrBadRequest)
	}

	targetPath := s.stagingPath(sessionID, expectedHash)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}

	tempPath := targetPath + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}

	hasher := sha256.New()
	size, copyErr := io.Copy(tempFile, io.TeeReader(body, hasher))
	closeErr := tempFile.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("write staging file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close staging file: %w", closeErr)
	}
	if expectedSize >= 0 && size != expectedSize {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: expected size %d got %d", ErrBadRequest, expectedSize, size)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: expected %s got %s", ErrHashMismatch, expectedHash, actualHash)
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("promote staging file: %w", err)
	}

	now := timestamp(time.Now())
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM staged_uploads
		WHERE session_id = ? AND path = ? AND expected_hash = ?
	`, sessionID, normalizedPath, expectedHash); err != nil {
		return fmt.Errorf("replace staged upload: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO staged_uploads (session_id, path, expected_hash, actual_hash, size, staging_path, status, created_at, validated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, normalizedPath, expectedHash, actualHash, size, targetPath, UploadStatusValidated, now, now); err != nil {
		return fmt.Errorf("record staged upload: %w", err)
	}

	return nil
}

func (s *Store) ensureBlob(ctx context.Context, hash string, stagingPath string) error {
	_ = ctx
	if err := validateSHA256(hash); err != nil {
		return err
	}

	blobPath := s.BlobPath(hash)
	if _, err := os.Stat(blobPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat blob: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return fmt.Errorf("create blob directory: %w", err)
	}

	if err := copyFile(stagingPath, blobPath); err != nil {
		return err
	}

	return nil
}

func copyFile(src string, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer input.Close()

	output, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}

	removeOutput := true
	defer func() {
		_ = output.Close()
		if removeOutput {
			_ = os.Remove(dst)
		}
	}()

	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy blob: %w", err)
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("sync blob: %w", err)
	}

	removeOutput = false
	return nil
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}

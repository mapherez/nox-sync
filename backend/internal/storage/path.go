package storage

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// NormalizeVaultPath validates and normalizes a path relative to a vault root.
func NormalizeVaultPath(raw string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if value == "" {
		return "", fmt.Errorf("%w: path is required", ErrBadRequest)
	}
	if strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("%w: path must be relative", ErrBadRequest)
	}

	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("%w: path is required", ErrBadRequest)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("%w: path escapes vault root", ErrBadRequest)
	}

	return cleaned, nil
}

func validateSHA256(hash string) error {
	if !sha256Pattern.MatchString(hash) {
		return fmt.Errorf("%w: expected SHA-256 hash", ErrBadRequest)
	}
	return nil
}

func blobRelativePath(hash string) string {
	return filepath.Join(hash[0:2], hash[2:4], hash)
}

func (s *Store) BlobPath(hash string) string {
	return filepath.Join(s.dataDir, "blobs", blobRelativePath(hash))
}

func (s *Store) stagingPath(sessionID string, hash string) string {
	return filepath.Join(s.dataDir, "staging", sessionID, hash)
}

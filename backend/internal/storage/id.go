package storage

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func randomID(prefix string) (string, error) {
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}

package storage

import "errors"

var (
	ErrBadRequest          = errors.New("bad request")
	ErrSyncLocked          = errors.New("sync locked")
	ErrSyncSessionNotFound = errors.New("sync session not found")
	ErrSyncSessionStale    = errors.New("sync session stale")
	ErrHashMismatch        = errors.New("hash mismatch")
	ErrNotFound            = errors.New("not found")
	ErrConflictDetected    = errors.New("conflict detected")
)

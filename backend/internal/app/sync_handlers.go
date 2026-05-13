package app

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

const maxJSONBodyBytes = 1 << 20

func (s *Server) handleBeginSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPost) || !s.requireAuth(w, r) {
		return
	}

	var req storage.BeginSyncRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	result, err := s.store.BeginSync(r.Context(), req)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	s.broadcastStatus(r)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleHeartbeatSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPost) || !s.requireAuth(w, r) {
		return
	}

	var req storage.HeartbeatRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := s.store.HeartbeatSync(r.Context(), req); err != nil {
		writeStorageError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleManifestSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPost) || !s.requireAuth(w, r) {
		return
	}

	var req storage.ManifestRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	plan, err := s.store.PlanSync(r.Context(), req)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleUploadSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPut) || !s.requireAuth(w, r) {
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/v1/sync/upload/")
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || strings.Contains(sessionID, "/") {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Valid sessionId is required in the upload path.")
		return
	}

	clientID := strings.TrimSpace(r.URL.Query().Get("clientId"))
	vaultPath := strings.TrimSpace(r.URL.Query().Get("path"))
	expectedHash := strings.TrimSpace(r.URL.Query().Get("hash"))
	expectedSize := int64(-1)
	if rawSize := strings.TrimSpace(r.URL.Query().Get("size")); rawSize != "" {
		size, err := strconv.ParseInt(rawSize, 10, 64)
		if err != nil || size < 0 {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Upload size must be a non-negative integer.")
			return
		}
		expectedSize = size
	}

	if err := s.store.StageUpload(r.Context(), sessionID, clientID, vaultPath, expectedHash, expectedSize, r.Body); err != nil {
		writeStorageError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCommitSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPost) || !s.requireAuth(w, r) {
		return
	}

	var req storage.CommitRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	result, err := s.store.CommitSync(r.Context(), req)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	s.broadcastStatus(r)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAbortSync(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodPost) || !s.requireAuth(w, r) {
		return
	}

	var req storage.AbortRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := s.store.AbortSync(r.Context(), req); err != nil {
		writeStorageError(w, err)
		return
	}

	s.broadcastStatus(r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if !s.requireMethod(w, r, http.MethodGet) || !s.requireAuth(w, r) {
		return
	}

	vaultPath := strings.TrimSpace(r.URL.Query().Get("path"))
	result, err := s.store.DownloadFile(r.Context(), vaultPath)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	file, err := os.Open(s.store.BlobPath(result.Hash))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to open remote file content.")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-NoX-Sync-Path", result.Path)
	w.Header().Set("X-NoX-Sync-Hash", result.Hash)
	w.Header().Set("X-NoX-Sync-Revision", strconv.FormatInt(result.Revision, 10))
	w.Header().Set("Content-Length", strconv.FormatInt(result.Size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func (s *Server) requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	w.Header().Set("Allow", method)
	writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
	return false
}

func (s *Server) broadcastStatus(r *http.Request) {
	payload, err := s.statusPayload(r.Context())
	if err != nil {
		return
	}
	s.events.broadcast(payload)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer r.Body.Close()

	decoder := jsonDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Request body must be valid JSON.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Request body must contain a single JSON value.")
		return false
	}

	return true
}

func writeStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrBadRequest):
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, storage.ErrSyncLocked):
		writeJSONError(w, http.StatusConflict, "SYNC_LOCKED", "Another sync is already in progress.")
	case errors.Is(err, storage.ErrSyncSessionNotFound):
		writeJSONError(w, http.StatusNotFound, "SYNC_SESSION_NOT_FOUND", "Sync session was not found.")
	case errors.Is(err, storage.ErrSyncSessionStale):
		writeJSONError(w, http.StatusConflict, "SYNC_SESSION_STALE", "Sync session is stale.")
	case errors.Is(err, storage.ErrHashMismatch):
		writeJSONError(w, http.StatusBadRequest, "HASH_MISMATCH", err.Error())
	case errors.Is(err, storage.ErrConflictDetected):
		writeJSONError(w, http.StatusConflict, "CONFLICT_DETECTED", "Unresolved conflicts must be handled before commit.")
	case errors.Is(err, storage.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Requested file was not found.")
	default:
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", fmt.Sprintf("Backend error: %v", err))
	}
}

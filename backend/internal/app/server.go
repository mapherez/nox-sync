package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

const sessionCookieName = "nox_sync_session"

// Server owns HTTP routing for the backend.
type Server struct {
	cfg    Config
	store  *storage.Store
	events *statusBroker
}

// NewServer creates a backend server instance.
func NewServer(cfg Config, store *storage.Store) *Server {
	return &Server{
		cfg:    cfg,
		store:  store,
		events: newStatusBroker(),
	}
}

// Routes returns the backend HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/vault-dashboard", s.handleDashboard)
	mux.HandleFunc("/vault-dashboard/api-key/rotate", s.handleRotateAPIKey)
	mux.HandleFunc("/vault-dashboard/users/add", s.handleAddUser)
	mux.HandleFunc("/vault-dashboard/users/status", s.handleSetUserStatus)
	mux.HandleFunc("/vault-dashboard/users/role", s.handleSetUserRole)
	mux.HandleFunc("/vault-dashboard/users/delete", s.handleDeleteUser)
	mux.HandleFunc("/vault-dashboard/vaults/delete", s.handleDeleteVault)
	mux.HandleFunc("/vault-dashboard/vaults/restore", s.handleRestoreVault)
	mux.HandleFunc("/vault-dashboard/vaults/purge", s.handlePurgeVault)
	mux.HandleFunc("/vault-dashboard/vaults/download", s.handleDownloadVaultZip)
	mux.HandleFunc("/auth/google/start", s.handleGoogleStart)
	mux.HandleFunc("/auth/google/callback", s.handleGoogleCallback)
	mux.HandleFunc("/auth/logout", s.handleLogout)
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/auth/check", s.handleAuthCheck)
	mux.HandleFunc("/v1/vaults", s.handlePluginVaults)
	mux.HandleFunc("/v1/vaults/restore", s.handlePluginRestoreVault)
	mux.HandleFunc("/v1/vaults/purge", s.handlePluginPurgeVault)
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/sync/events", s.handleSyncEvents)
	mux.HandleFunc("/v1/sync/begin", s.handleBeginSync)
	mux.HandleFunc("/v1/sync/heartbeat", s.handleHeartbeatSync)
	mux.HandleFunc("/v1/sync/manifest", s.handleManifestSync)
	mux.HandleFunc("/v1/sync/upload/", s.handleUploadSync)
	mux.HandleFunc("/v1/sync/commit", s.handleCommitSync)
	mux.HandleFunc("/v1/sync/abort", s.handleAbortSync)
	mux.HandleFunc("/v1/files/download", s.handleDownloadFile)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ready",
		"version":            s.cfg.Version,
		"dataDirInitialized": true,
		"databasePath":       s.store.DBPath(),
	})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	user, ok := s.requireAPIUser(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"user":  user.Email,
		"role":  user.Role,
		"vault": "selected-in-settings",
	})
}

func (s *Server) handlePluginVaults(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAPIUser(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		vaults, err := s.store.ListVaults(r.Context(), user.ID)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		deletedVaults, err := s.store.ListDeletedVaults(r.Context(), user.ID)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, storage.VaultListResponse{Vaults: vaults, DeletedVaults: deletedVaults})
	case http.MethodPost:
		var req storage.CreateVaultRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		vault, err := s.store.CreateVault(r.Context(), user.ID, req.Name)
		if err != nil {
			writeStorageError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, vault)
	case http.MethodDelete:
		vaultID, ok := vaultIDFromQuery(w, r)
		if !ok {
			return
		}
		if err := s.store.SoftDeleteVault(r.Context(), user.ID, vaultID); err != nil {
			writeStorageError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
	}
}

func (s *Server) handlePluginRestoreVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	user, ok := s.requireAPIUser(w, r)
	if !ok {
		return
	}
	vaultID, ok := vaultIDFromQuery(w, r)
	if !ok {
		return
	}
	if err := s.store.RestoreVault(r.Context(), user.ID, vaultID); err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handlePluginPurgeVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	user, ok := s.requireAPIUser(w, r)
	if !ok {
		return
	}
	vaultID, ok := vaultIDFromQuery(w, r)
	if !ok {
		return
	}
	if err := s.store.PurgeDeletedVault(r.Context(), user.ID, vaultID); err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	user, ok := s.requireAPIUser(w, r)
	if !ok {
		return
	}
	vaultID, ok := vaultIDFromQuery(w, r)
	if !ok {
		return
	}

	payload, reaped, err := s.statusPayloadWithRefresh(r.Context(), user.ID, vaultID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if reaped {
		s.events.broadcast(vaultID, payload)
	}

	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSyncEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	user, ok := s.requireAPIUser(w, r)
	if !ok {
		return
	}
	vaultID, ok := vaultIDFromQuery(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	payload, reaped, err := s.statusPayloadWithRefresh(r.Context(), user.ID, vaultID)
	if err != nil {
		writeSSEError(w, "SERVER_ERROR", "Failed to load status.")
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		writeSSEError(w, "SERVER_ERROR", "Failed to encode status.")
		return
	}

	_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", body)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	if reaped {
		s.events.broadcast(vaultID, payload)
	}

	events, unsubscribe := s.events.subscribe(vaultID)
	defer unsubscribe()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", event)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

func (s *Server) statusPayload(ctx context.Context, userID string, vaultID string) (map[string]any, error) {
	payload, _, err := s.statusPayloadWithRefresh(ctx, userID, vaultID)
	return payload, err
}

func (s *Server) statusPayloadWithRefresh(ctx context.Context, userID string, vaultID string) (map[string]any, bool, error) {
	revision, err := s.store.ServerRevision(ctx, userID, vaultID)
	if err != nil {
		return nil, false, err
	}

	syncStatus, reaped, err := s.store.RefreshSyncStatus(ctx, userID, vaultID)
	if err != nil {
		return nil, false, err
	}

	return map[string]any{
		"vaultId":        vaultID,
		"serverRevision": revision,
		"sync": map[string]any{
			"state":      syncStatus.State,
			"sessionId":  syncStatus.SessionID,
			"clientId":   syncStatus.ClientID,
			"clientName": syncStatus.ClientName,
			"startedAt":  syncStatus.StartedAt,
		},
	}, reaped, nil
}

func (s *Server) monitorStaleLocks(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.broadcastStaleLocksIfNeeded(ctx)
		}
	}
}

func (s *Server) broadcastStaleLocksIfNeeded(ctx context.Context) {
	vaultIDs, err := s.store.ReapExpiredLocks(ctx)
	if err != nil {
		return
	}
	for _, vaultID := range vaultIDs {
		s.broadcastVaultStatusContext(ctx, vaultID)
	}
}

func (s *Server) requireAPIUser(w http.ResponseWriter, r *http.Request) (storage.User, bool) {
	token := authToken(r)
	if token == "" {
		writeJSONError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "NoX Sync API key is required.")
		return storage.User{}, false
	}

	user, valid, err := s.store.AuthenticateAPIKey(r.Context(), token)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to validate API key.")
		return storage.User{}, false
	}

	if !valid {
		writeJSONError(w, http.StatusUnauthorized, "AUTH_FAILED", "Invalid NoX Sync API key.")
		return storage.User{}, false
	}

	return user, true
}

func (s *Server) requireWebUser(w http.ResponseWriter, r *http.Request) (storage.User, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return storage.User{}, false
	}
	user, ok, err := s.store.UserBySessionToken(r.Context(), cookie.Value)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to load session.")
		return storage.User{}, false
	}
	return user, ok
}

func (s *Server) requireAdminWebUser(w http.ResponseWriter, r *http.Request) (storage.User, bool) {
	user, ok := s.requireWebUser(w, r)
	if !ok {
		http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
		return storage.User{}, false
	}
	if user.Role != storage.UserRoleAdmin {
		writeJSONError(w, http.StatusForbidden, "FORBIDDEN", "Admin access is required.")
		return storage.User{}, false
	}
	return user, true
}

func (s *Server) publicURL(r *http.Request) string {
	if strings.TrimSpace(s.cfg.PublicURL) != "" {
		return strings.TrimRight(strings.TrimSpace(s.cfg.PublicURL), "/")
	}
	return serverURL(r)
}

func authToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}

	return strings.TrimSpace(r.URL.Query().Get("api_key"))
}

func vaultIDFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	vaultID := strings.TrimSpace(r.URL.Query().Get("vaultId"))
	if vaultID == "" {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "vaultId is required.")
		return "", false
	}
	return vaultID, true
}

func (s *Server) broadcastStatus(r *http.Request, userID string, vaultID string) {
	s.broadcastStatusContext(r.Context(), userID, vaultID)
}

func (s *Server) broadcastStatusContext(ctx context.Context, userID string, vaultID string) {
	payload, err := s.statusPayload(ctx, userID, vaultID)
	if err != nil {
		return
	}
	s.events.broadcast(vaultID, payload)
}

func (s *Server) broadcastVaultStatusContext(ctx context.Context, vaultID string) {
	userID, err := s.store.VaultOwnerID(ctx, vaultID)
	if err != nil {
		return
	}
	s.broadcastStatusContext(ctx, userID, vaultID)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]string{
		"code":    code,
		"message": message,
	})
}

func writeSSEError(w http.ResponseWriter, code string, message string) {
	payload, err := json.Marshal(map[string]string{
		"code":    code,
		"message": message,
	})
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
}

func serverURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost:8080"
	}

	return scheme + "://" + host
}

// Timestamp is kept here to make future status payloads use a consistent format.
func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

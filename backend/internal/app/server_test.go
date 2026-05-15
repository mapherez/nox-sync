package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

func TestHealthEndpoint(t *testing.T) {
	server, _, _, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
}

func TestStatusEndpointRequiresAuth(t *testing.T) {
	server, _, _, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}
}

func TestStatusEndpointAcceptsActiveAPIKey(t *testing.T) {
	server, store, user, vault := newTestServer(t)
	apiKey, err := store.CurrentAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/status?vaultId="+vault.ID, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func TestAdminPageShowsReusableAPIKey(t *testing.T) {
	server, store, user, _ := newTestServer(t)
	apiKey, err := store.CurrentAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load api key: %v", err)
	}
	sessionToken, err := store.CreateWebSession(context.Background(), user.ID, webSessionDuration)
	if err != nil {
		t.Fatalf("create web session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/vault-dashboard", nil)
	req.Host = "localhost:8080"
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if !strings.Contains(res.Body.String(), apiKey) {
		t.Fatalf("expected dashboard to include active api key")
	}
}

func TestRotateAPIKeyInvalidatesPreviousKey(t *testing.T) {
	server, store, user, _ := newTestServer(t)
	oldKey, err := store.CurrentAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load old api key: %v", err)
	}
	sessionToken, err := store.CreateWebSession(context.Background(), user.ID, webSessionDuration)
	if err != nil {
		t.Fatalf("create web session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/vault-dashboard/api-key/rotate", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, res.Code)
	}

	newKey, err := store.CurrentAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load new api key: %v", err)
	}
	if newKey == oldKey {
		t.Fatalf("expected rotated key to change")
	}

	validOld, err := store.ValidateAPIKey(context.Background(), oldKey)
	if err != nil {
		t.Fatalf("validate old key: %v", err)
	}
	if validOld {
		t.Fatalf("expected old key to be invalid")
	}

	validNew, err := store.ValidateAPIKey(context.Background(), newKey)
	if err != nil {
		t.Fatalf("validate new key: %v", err)
	}
	if !validNew {
		t.Fatalf("expected new key to be valid")
	}
}

func TestPluginVaultLifecycleCanDeleteRestoreAndPurge(t *testing.T) {
	server, store, user, vault := newTestServer(t)
	apiKey, err := store.CurrentAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load api key: %v", err)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/vaults?vaultId="+vault.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+apiKey)
	deleteRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected delete status %d, got %d", http.StatusOK, deleteRes.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/vaults", nil)
	listReq.Header.Set("Authorization", "Bearer "+apiKey)
	listRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRes.Code)
	}
	var listed storage.VaultListResponse
	if err := json.NewDecoder(listRes.Body).Decode(&listed); err != nil {
		t.Fatalf("decode vault list: %v", err)
	}
	if len(listed.Vaults) != 0 || len(listed.DeletedVaults) != 1 {
		t.Fatalf("expected one deleted vault, got active=%d deleted=%d", len(listed.Vaults), len(listed.DeletedVaults))
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/v1/vaults/restore?vaultId="+vault.ID, nil)
	restoreReq.Header.Set("Authorization", "Bearer "+apiKey)
	restoreRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(restoreRes, restoreReq)
	if restoreRes.Code != http.StatusOK {
		t.Fatalf("expected restore status %d, got %d", http.StatusOK, restoreRes.Code)
	}
	if _, err := store.VaultByID(context.Background(), user.ID, vault.ID); err != nil {
		t.Fatalf("expected restored vault to load: %v", err)
	}

	deleteReq = httptest.NewRequest(http.MethodDelete, "/v1/vaults?vaultId="+vault.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+apiKey)
	deleteRes = httptest.NewRecorder()
	server.Routes().ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected second delete status %d, got %d", http.StatusOK, deleteRes.Code)
	}

	purgeReq := httptest.NewRequest(http.MethodPost, "/v1/vaults/purge?vaultId="+vault.ID, nil)
	purgeReq.Header.Set("Authorization", "Bearer "+apiKey)
	purgeRes := httptest.NewRecorder()
	server.Routes().ServeHTTP(purgeRes, purgeReq)
	if purgeRes.Code != http.StatusOK {
		t.Fatalf("expected purge status %d, got %d", http.StatusOK, purgeRes.Code)
	}
	if _, err := store.VaultByID(context.Background(), user.ID, vault.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected purged vault to be gone, got %v", err)
	}
}

func TestAdminDashboardCannotDeleteAdminUser(t *testing.T) {
	server, store, user, _ := newTestServer(t)
	sessionToken, err := store.CreateWebSession(context.Background(), user.ID, webSessionDuration)
	if err != nil {
		t.Fatalf("create web session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/vault-dashboard/users/delete", strings.NewReader("userId="+user.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
	if _, err := store.UserByID(context.Background(), user.ID); err != nil {
		t.Fatalf("expected admin user to remain: %v", err)
	}
}

func newTestServer(t *testing.T) (*Server, *storage.Store, storage.User, storage.Vault) {
	t.Helper()

	store, err := storage.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	user, err := store.UpsertAllowedUser(context.Background(), "test@example.com", storage.UserRoleAdmin)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	vault, err := store.CreateVault(context.Background(), user.ID, "Test Vault")
	if err != nil {
		t.Fatalf("seed vault: %v", err)
	}

	return NewServer(Config{Version: "test"}, store), store, user, vault
}

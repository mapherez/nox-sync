package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

func TestHealthEndpoint(t *testing.T) {
	server, _ := newTestServer(t)

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
	server, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}
}

func TestStatusEndpointAcceptsActiveAPIKey(t *testing.T) {
	server, store := newTestServer(t)
	apiKey, err := store.CurrentAPIKey(context.Background())
	if err != nil {
		t.Fatalf("load api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func TestAdminPageShowsReusableAPIKey(t *testing.T) {
	server, store := newTestServer(t)
	apiKey, err := store.CurrentAPIKey(context.Background())
	if err != nil {
		t.Fatalf("load api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8080"
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if !strings.Contains(res.Body.String(), apiKey) {
		t.Fatalf("expected admin page to include active api key")
	}
}

func TestRotateAPIKeyInvalidatesPreviousKey(t *testing.T) {
	server, store := newTestServer(t)
	oldKey, err := store.CurrentAPIKey(context.Background())
	if err != nil {
		t.Fatalf("load old api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/api-key/rotate", nil)
	res := httptest.NewRecorder()

	server.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, res.Code)
	}

	newKey, err := store.CurrentAPIKey(context.Background())
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

func newTestServer(t *testing.T) (*Server, *storage.Store) {
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

	return NewServer(Config{Version: "test"}, store), store
}

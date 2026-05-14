package app

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

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
	mux.HandleFunc("/", s.handleAdmin)
	mux.HandleFunc("/admin/api-key/rotate", s.handleRotateAPIKey)
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/auth/check", s.handleAuthCheck)
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

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	apiKey, err := s.store.CurrentAPIKey(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to load API key.")
		return
	}

	pageData := adminPageData{
		ServerURL: serverURL(r),
		APIKey:    apiKey,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminTemplate.Execute(w, pageData); err != nil {
		return
	}
}

func (s *Server) handleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	if _, err := s.store.RotateAPIKey(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to generate a new API key.")
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
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

	if !s.requireAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	payload, reaped, err := s.statusPayloadWithRefresh(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to load status.")
		return
	}
	if reaped {
		s.events.broadcast(payload)
	}

	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSyncEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	if !s.requireAuth(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	payload, reaped, err := s.statusPayloadWithRefresh(r.Context())
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
		s.events.broadcast(payload)
	}

	events, unsubscribe := s.events.subscribe()
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

func (s *Server) statusPayload(ctx context.Context) (map[string]any, error) {
	payload, _, err := s.statusPayloadWithRefresh(ctx)
	return payload, err
}

func (s *Server) statusPayloadWithRefresh(ctx context.Context) (map[string]any, bool, error) {
	revision, err := s.store.ServerRevision(ctx)
	if err != nil {
		return nil, false, err
	}

	syncStatus, reaped, err := s.store.RefreshSyncStatus(ctx)
	if err != nil {
		return nil, false, err
	}

	return map[string]any{
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
			s.broadcastStaleLockIfNeeded(ctx)
		}
	}
}

func (s *Server) broadcastStaleLockIfNeeded(ctx context.Context) {
	reaped, err := s.store.ReapExpiredLock(ctx)
	if err != nil || !reaped {
		return
	}

	s.broadcastStatusContext(ctx)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	token := authToken(r)
	if token == "" {
		writeJSONError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "NoX Sync API key is required.")
		return false
	}

	valid, err := s.store.ValidateAPIKey(r.Context(), token)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to validate API key.")
		return false
	}

	if !valid {
		writeJSONError(w, http.StatusUnauthorized, "AUTH_FAILED", "Invalid NoX Sync API key.")
		return false
	}

	return true
}

func authToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}

	return strings.TrimSpace(r.URL.Query().Get("api_key"))
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

type adminPageData struct {
	ServerURL string
	APIKey    string
}

var adminTemplate = template.Must(template.New("admin").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NoX Sync</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f9fafb;
      --panel: #ffffff;
      --text: #111827;
      --muted: #4b5563;
      --border: #d1d5db;
      --button: #111827;
      --button-text: #ffffff;
      --danger: #ef4444;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #111827;
        --panel: #1f2937;
        --text: #f9fafb;
        --muted: #d1d5db;
        --border: #374151;
        --button: #f9fafb;
        --button-text: #111827;
      }
    }
    * { box-sizing: border-box; }
    body {
      background: var(--bg);
      color: var(--text);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      margin: 0;
      padding: 3rem 1rem;
    }
    main {
      margin: 0 auto;
      max-width: 44rem;
    }
    h1 {
      font-size: 2rem;
      letter-spacing: 0;
      margin: 0 0 2rem;
    }
    .field {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      margin-bottom: 1rem;
      padding: 1rem;
    }
    label {
      color: var(--muted);
      display: block;
      font-size: 0.875rem;
      margin-bottom: 0.5rem;
    }
    .copy-row {
      display: flex;
      gap: 0.5rem;
    }
    input {
      background: transparent;
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--text);
      flex: 1;
      font: inherit;
      min-width: 0;
      padding: 0.65rem 0.75rem;
    }
    button {
      background: var(--button);
      border: 0;
      border-radius: 6px;
      color: var(--button-text);
      cursor: pointer;
      font: inherit;
      padding: 0.65rem 0.85rem;
      white-space: nowrap;
    }
    .danger {
      background: var(--danger);
      color: #ffffff;
      margin-top: 0.5rem;
    }
    .note {
      color: var(--muted);
      font-size: 0.925rem;
      line-height: 1.5;
      margin-top: 1.5rem;
    }
    @media (max-width: 520px) {
      body { padding-top: 1.5rem; }
      .copy-row { flex-direction: column; }
      button { width: 100%; }
    }
  </style>
</head>
<body>
  <main>
    <h1>NoX Sync</h1>

    <section class="field">
      <label for="server-url">Server URL</label>
      <div class="copy-row">
        <input id="server-url" readonly value="{{.ServerURL}}">
        <button type="button" data-copy="server-url">Copy</button>
      </div>
    </section>

    <section class="field">
      <label for="api-key">API Key</label>
      <div class="copy-row">
        <input id="api-key" readonly value="{{.APIKey}}">
        <button type="button" data-copy="api-key">Copy</button>
      </div>
      <form method="post" action="/admin/api-key/rotate">
        <button class="danger" type="submit">Generate New API Key</button>
      </form>
    </section>

    <p class="note">Generating a new API key invalidates the previous key. Existing Obsidian devices must be updated manually.</p>
  </main>
  <script>
    document.querySelectorAll("[data-copy]").forEach((button) => {
      button.addEventListener("click", async () => {
        const input = document.getElementById(button.dataset.copy);
        if (!input) return;
        await navigator.clipboard.writeText(input.value);
        const oldText = button.textContent;
        button.textContent = "Copied";
        window.setTimeout(() => { button.textContent = oldText; }, 1200);
      });
    });
  </script>
</body>
</html>`))

// Timestamp is kept here to make future status payloads use a consistent format.
func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

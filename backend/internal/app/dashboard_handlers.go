package app

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mapherez/nox-sync/backend/internal/storage"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/vault-dashboard" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}

	user, authenticated := s.requireWebUser(w, r)
	data := dashboardPageData{
		Authenticated:   authenticated,
		OAuthConfigured: s.googleOAuthConfigured(),
		LoginURL:        "/auth/google/start?redirect=/vault-dashboard",
		ServerURL:       s.publicURL(r),
		Message:         dashboardMessage(r.URL.Query().Get("message")),
	}
	if authenticated {
		apiKey, err := s.store.CurrentAPIKey(r.Context(), user.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to load API key.")
			return
		}
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
		data.User = user
		data.APIKey = apiKey
		data.Vaults = vaults
		data.DeletedVaults = deletedVaults
		data.IsAdmin = user.Role == storage.UserRoleAdmin
		if data.IsAdmin {
			users, err := s.store.ListUsers(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to load users.")
				return
			}
			data.Users = users
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dashboardTemplate.Execute(w, data)
}

func (s *Server) handleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	user, ok := s.requireWebUser(w, r)
	if !ok {
		http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
		return
	}
	if _, err := s.store.RotateAPIKey(r.Context(), user.ID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "SERVER_ERROR", "Failed to generate a new API key.")
		return
	}

	http.Redirect(w, r, "/vault-dashboard?message=api-key-rotated", http.StatusSeeOther)
}

func (s *Server) handleAddUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	if _, ok := s.requireAdminWebUser(w, r); !ok {
		return
	}
	if _, err := s.store.UpsertAllowedUser(r.Context(), r.FormValue("email"), r.FormValue("role")); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=user-saved", http.StatusSeeOther)
}

func (s *Server) handleSetUserStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	admin, ok := s.requireAdminWebUser(w, r)
	if !ok {
		return
	}
	userID := strings.TrimSpace(r.FormValue("userId"))
	status := strings.TrimSpace(r.FormValue("status"))
	target, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if target.ID == admin.ID && status == storage.UserStatusDisabled {
		writeStorageError(w, fmt.Errorf("%w: admins cannot disable their own account", storage.ErrBadRequest))
		return
	}
	if target.Role == storage.UserRoleAdmin && status == storage.UserStatusDisabled {
		writeStorageError(w, fmt.Errorf("%w: admin users cannot be disabled", storage.ErrBadRequest))
		return
	}
	if err := s.store.SetUserStatus(r.Context(), userID, status); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=user-saved", http.StatusSeeOther)
}

func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	admin, ok := s.requireAdminWebUser(w, r)
	if !ok {
		return
	}
	userID := strings.TrimSpace(r.FormValue("userId"))
	role := strings.ToUpper(strings.TrimSpace(r.FormValue("role")))
	target, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if target.ID == admin.ID && role != storage.UserRoleAdmin {
		writeStorageError(w, fmt.Errorf("%w: admins cannot demote their own account", storage.ErrBadRequest))
		return
	}
	if target.Role == storage.UserRoleAdmin && role != storage.UserRoleAdmin {
		writeStorageError(w, fmt.Errorf("%w: admin users cannot be demoted", storage.ErrBadRequest))
		return
	}
	if err := s.store.SetUserRole(r.Context(), userID, role); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=user-saved", http.StatusSeeOther)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	admin, ok := s.requireAdminWebUser(w, r)
	if !ok {
		return
	}
	userID := strings.TrimSpace(r.FormValue("userId"))
	target, err := s.store.UserByID(r.Context(), userID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if target.ID == admin.ID || target.Role == storage.UserRoleAdmin {
		writeStorageError(w, fmt.Errorf("%w: admin users cannot be deleted", storage.ErrBadRequest))
		return
	}
	if err := s.store.DeleteUser(r.Context(), userID); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=user-deleted", http.StatusSeeOther)
}

func (s *Server) handleDeleteVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	user, ok := s.requireWebUser(w, r)
	if !ok {
		http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
		return
	}
	if err := s.store.SoftDeleteVault(r.Context(), user.ID, r.FormValue("vaultId")); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=vault-deleted", http.StatusSeeOther)
}

func (s *Server) handleRestoreVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	user, ok := s.requireWebUser(w, r)
	if !ok {
		http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
		return
	}
	if err := s.store.RestoreVault(r.Context(), user.ID, r.FormValue("vaultId")); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=vault-restored", http.StatusSeeOther)
}

func (s *Server) handlePurgeVault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	user, ok := s.requireWebUser(w, r)
	if !ok {
		http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
		return
	}
	if err := s.store.PurgeDeletedVault(r.Context(), user.ID, r.FormValue("vaultId")); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=vault-purged", http.StatusSeeOther)
}

func (s *Server) handleDownloadVaultZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "BAD_REQUEST", "Method not allowed.")
		return
	}
	user, ok := s.requireWebUser(w, r)
	if !ok {
		http.Redirect(w, r, "/vault-dashboard", http.StatusSeeOther)
		return
	}
	vaultID := strings.TrimSpace(r.URL.Query().Get("vaultId"))
	vault, err := s.store.VaultByID(r.Context(), user.ID, vaultID)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	files, err := s.store.CurrentVaultFiles(r.Context(), user.ID, vaultID)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, sanitizeFilename(vault.Name)))
	writer := zip.NewWriter(w)
	defer writer.Close()

	for _, file := range files {
		if err := writeZipFile(writer, s.store.BlobPath(file.Hash), file.Path); err != nil {
			return
		}
	}
}

func writeZipFile(writer *zip.Writer, blobPath string, vaultPath string) error {
	source, err := os.Open(blobPath)
	if err != nil {
		return err
	}
	defer source.Close()

	entry, err := writer.CreateHeader(&zip.FileHeader{
		Name:   vaultPath,
		Method: zip.Deflate,
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, source)
	return err
}

func dashboardMessage(code string) string {
	switch code {
	case "api-key-rotated":
		return "A new API key was generated. Update existing Obsidian devices manually."
	case "user-saved":
		return "User settings were saved."
	case "user-deleted":
		return "User deleted."
	case "vault-deleted":
		return "Vault deleted."
	case "vault-restored":
		return "Vault restored."
	case "vault-purged":
		return "Vault permanently deleted."
	default:
		return ""
	}
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "nox-sync-vault"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		case r == ' ':
			builder.WriteRune('-')
		}
	}
	if builder.Len() == 0 {
		return "nox-sync-vault"
	}
	return builder.String()
}

func formatDashboardBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 MB"
	}
	mb := float64(bytes) / (1024 * 1024)
	if mb < 0.1 {
		return "< 0.1 MB"
	}
	if mb >= 10 {
		return fmt.Sprintf("%.0f MB", mb)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

func dashboardIcon(name string) template.HTML {
	icons := map[string]string{
		"copy":       `<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`,
		"download":   `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><path d="M7 10l5 5 5-5"></path><path d="M12 15V3"></path></svg>`,
		"logout":     `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path><path d="M16 17l5-5-5-5"></path><path d="M21 12H9"></path></svg>`,
		"refresh":    `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12a9 9 0 0 0-15-6.7L3 8"></path><path d="M3 3v5h5"></path><path d="M3 12a9 9 0 0 0 15 6.7L21 16"></path><path d="M21 21v-5h-5"></path></svg>`,
		"restore":    `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 12a9 9 0 1 0 3-6.7"></path><path d="M3 3v6h6"></path><path d="M12 7v5l3 2"></path></svg>`,
		"shield":     `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10"></path><path d="M9 12l2 2 4-4"></path></svg>`,
		"toggle-off": `<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="2" y="7" width="20" height="10" rx="5"></rect><circle cx="7" cy="12" r="3"></circle></svg>`,
		"toggle-on":  `<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="2" y="7" width="20" height="10" rx="5"></rect><circle cx="17" cy="12" r="3"></circle></svg>`,
		"trash":      `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18"></path><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"></path><path d="M10 11v6"></path><path d="M14 11v6"></path></svg>`,
		"user":       `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 21a8 8 0 0 0-16 0"></path><circle cx="12" cy="7" r="4"></circle></svg>`,
		"user-x":     `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle><path d="M17 8l5 5"></path><path d="M22 8l-5 5"></path></svg>`,
		"x":          `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6L6 18"></path><path d="M6 6l12 12"></path></svg>`,
	}
	return template.HTML(icons[name])
}

func isAdminDashboardUser(user storage.User) bool {
	return user.Role == storage.UserRoleAdmin
}

type dashboardPageData struct {
	Authenticated   bool
	OAuthConfigured bool
	LoginURL        string
	ServerURL       string
	Message         string
	User            storage.User
	APIKey          string
	Vaults          []storage.Vault
	DeletedVaults   []storage.Vault
	IsAdmin         bool
	Users           []storage.User
}

var dashboardTemplate = template.Must(template.New("dashboard").Funcs(template.FuncMap{
	"formatBytes": formatDashboardBytes,
	"icon":        dashboardIcon,
	"isAdminUser": isAdminDashboardUser,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NoX Sync Dashboard</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f7f8fa;
      --panel: #ffffff;
      --text: #111827;
      --muted: #526070;
      --border: #d8dee8;
      --button: #111827;
      --button-text: #ffffff;
      --danger: #c2410c;
      --danger-bg: #fff7ed;
      --success-bg: #edf7f1;
      --toggle-off: #cbd5e1;
      --toggle-on: #16a34a;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #101318;
        --panel: #171c23;
        --text: #f5f7fa;
        --muted: #b8c0cc;
        --border: #2d3642;
        --button: #f5f7fa;
        --button-text: #101318;
        --danger-bg: #2b170d;
        --success-bg: #102017;
        --toggle-off: #475569;
        --toggle-on: #22c55e;
      }
    }
    * { box-sizing: border-box; }
    body {
      background: var(--bg);
      color: var(--text);
      font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      margin: 0;
      padding: 2rem 1rem;
    }
    main {
      margin: 0 auto;
      max-width: 72rem;
    }
    header, .section-heading {
      align-items: center;
      display: flex;
      justify-content: space-between;
      gap: 1rem;
    }
    header {
      margin-bottom: 1.5rem;
    }
    .header-actions {
      align-items: center;
      display: flex;
      gap: 0.5rem;
    }
    h1, h2 {
      letter-spacing: 0;
      margin: 0;
    }
    h1 { font-size: 1.7rem; }
    h2 { font-size: 1.05rem; }
    .muted {
      color: var(--muted);
      font-size: 0.92rem;
    }
    .stack {
      display: grid;
      gap: 1rem;
    }
    .grid {
      display: grid;
      gap: 1rem;
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    section, .login-panel {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 1rem;
    }
    label {
      color: var(--muted);
      display: block;
      font-size: 0.85rem;
      margin-bottom: 0.45rem;
    }
    .copy-row, .form-row, .actions, .action-cell {
      align-items: center;
      display: flex;
      gap: 0.5rem;
    }
    .action-cell {
      flex-wrap: wrap;
    }
    input, select {
      background: transparent;
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--text);
      flex: 1;
      font: inherit;
      min-width: 0;
      padding: 0.62rem 0.72rem;
    }
    .role-select, .role-select option {
      background: #ffffff;
      color: #111827;
    }
    button, .button {
      background: var(--button);
      border: 0;
      border-radius: 6px;
      color: var(--button-text);
      cursor: pointer;
      display: inline-block;
      font: inherit;
      padding: 0.62rem 0.82rem;
      text-decoration: none;
      white-space: nowrap;
    }
    button:disabled, .button[aria-disabled="true"] {
      cursor: not-allowed;
      opacity: 0.4;
    }
    .secondary {
      background: transparent;
      border: 1px solid var(--border);
      color: var(--text);
    }
    .danger {
      background: var(--danger);
      color: #ffffff;
    }
    .icon-button {
      align-items: center;
      display: inline-flex;
      height: 2.25rem;
      justify-content: center;
      padding: 0;
      width: 2.25rem;
    }
    .icon-button svg {
      fill: none;
      height: 1.05rem;
      stroke: currentColor;
      stroke-linecap: round;
      stroke-linejoin: round;
      stroke-width: 2;
      width: 1.05rem;
    }
    .message {
      align-items: center;
      background: var(--success-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      display: flex;
      gap: 1rem;
      justify-content: space-between;
      margin-bottom: 1rem;
      padding: 0.65rem 0.75rem 0.65rem 1rem;
    }
    .message-close {
      flex: 0 0 auto;
      height: 1.8rem;
      width: 1.8rem;
    }
    .add-user-panel {
      border-bottom: 1px solid var(--border);
      margin-bottom: 1rem;
      padding-bottom: 1rem;
    }
    table {
      border-collapse: collapse;
      width: 100%;
    }
    th, td {
      border-bottom: 1px solid var(--border);
      padding: 0.7rem 0.4rem;
      text-align: left;
      vertical-align: middle;
    }
    th {
      color: var(--muted);
      font-size: 0.8rem;
      font-weight: 600;
      text-transform: uppercase;
    }
    td form {
      display: inline-flex;
    }
    .status-toggle {
      background: var(--toggle-off);
      border: 0;
      border-radius: 999px;
      height: 1.45rem;
      padding: 0.16rem;
      position: relative;
      width: 2.65rem;
    }
    .status-toggle span {
      background: #ffffff;
      border-radius: 999px;
      display: block;
      height: 1.13rem;
      transform: translateX(0);
      transition: transform 140ms ease;
      width: 1.13rem;
    }
    .status-toggle.is-active {
      background: var(--toggle-on);
    }
    .status-toggle.is-active span {
      transform: translateX(1.2rem);
    }
    dialog {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      color: var(--text);
      max-width: min(34rem, calc(100vw - 2rem));
      padding: 1rem;
    }
    dialog::backdrop {
      background: rgba(0, 0, 0, 0.42);
    }
    .modal-table {
      margin-top: 0.75rem;
    }
    .modal-table td:last-child {
      white-space: nowrap;
    }
    @media (max-width: 760px) {
      body { padding-top: 1rem; }
      header, .copy-row, .form-row { align-items: stretch; flex-direction: column; }
      .header-actions, .section-heading { align-items: center; flex-direction: row; }
      .grid { grid-template-columns: 1fr; }
      table { display: block; overflow-x: auto; }
      button:not(.icon-button), .button:not(.icon-button) { width: 100%; }
      .icon-button { width: 2.25rem; }
    }
  </style>
</head>
<body>
  <main>
    {{if .Authenticated}}
      <header>
        <div>
          <h1>Welcome {{.User.FirstName}}</h1>
          <div class="muted">{{.User.Email}}</div>
        </div>
        <div class="header-actions">
          <a class="icon-button secondary" href="/vault-dashboard" title="Refresh dashboard" aria-label="Refresh dashboard">{{icon "refresh"}}</a>
          <form method="post" action="/auth/logout">
            <button class="icon-button secondary" type="submit" title="Log out" aria-label="Log out">{{icon "logout"}}</button>
          </form>
        </div>
      </header>

      {{if .Message}}
        <div class="message">
          <span>{{.Message}}</span>
          <button class="icon-button secondary message-close" type="button" data-close-message title="Close message" aria-label="Close message">{{icon "x"}}</button>
        </div>
      {{end}}

      <div class="grid">
        <section>
          <h2>Server URL</h2>
          <div class="copy-row" style="margin-top: 0.75rem;">
            <input id="server-url" readonly value="{{.ServerURL}}">
            <button class="icon-button" type="button" data-copy="server-url" title="Copy server URL" aria-label="Copy server URL">{{icon "copy"}}</button>
          </div>
        </section>

        <section>
          <h2>API Key</h2>
          <div class="copy-row" style="margin-top: 0.75rem;">
            <input id="api-key" readonly value="{{.APIKey}}">
            <button class="icon-button" type="button" data-copy="api-key" title="Copy API key" aria-label="Copy API key">{{icon "copy"}}</button>
          </div>
          <form method="post" action="/vault-dashboard/api-key/rotate" style="margin-top: 0.75rem;">
            <button class="danger" type="submit">Generate New API Key</button>
          </form>
          <p class="muted">Generating a new API key invalidates this user's previous key.</p>
        </section>
      </div>

      <div class="stack" style="margin-top: 1rem;">
        <section>
          <div class="section-heading">
            <h2>Vaults</h2>
            <button class="icon-button secondary" type="button" data-open-restore-vaults title="Restore deleted vaults" aria-label="Restore deleted vaults" {{if not .DeletedVaults}}disabled{{end}}>{{icon "restore"}}</button>
          </div>
          {{if .Vaults}}
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Revision</th>
                  <th>Updated</th>
                  <th>Size</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {{range .Vaults}}
                  <tr>
                    <td>{{.Name}}</td>
                    <td>{{.Revision}}</td>
                    <td>{{.UpdatedAt}}</td>
                    <td>{{formatBytes .SizeBytes}}</td>
                    <td class="action-cell">
                      <a class="icon-button secondary" href="/vault-dashboard/vaults/download?vaultId={{.ID}}" title="Download vault" aria-label="Download vault">{{icon "download"}}</a>
                      <button class="icon-button danger" type="button" data-delete-vault="{{.ID}}" data-delete-name="{{.Name}}" title="Delete vault" aria-label="Delete vault">{{icon "trash"}}</button>
                    </td>
                  </tr>
                {{end}}
              </tbody>
            </table>
          {{else}}
            <p class="muted">No remote vaults yet. Create one from the Obsidian plugin settings.</p>
          {{end}}
        </section>

        {{if .IsAdmin}}
          <section>
            <h2>User Management</h2>
            <div class="add-user-panel">
              <form class="form-row" method="post" action="/vault-dashboard/users/add">
                <input name="email" type="email" placeholder="user@example.com" required>
                <select class="role-select" name="role">
                  <option value="USER">User</option>
                  <option value="ADMIN">Admin</option>
                </select>
                <button type="submit">Add User</button>
              </form>
            </div>

            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {{range .Users}}
                  <tr>
                    <td>{{.Email}}</td>
                    <td>{{.Role}}</td>
                    <td>
                      <form method="post" action="/vault-dashboard/users/status">
                        <input type="hidden" name="userId" value="{{.ID}}">
                        {{if eq .Status "ACTIVE"}}
                          <input type="hidden" name="status" value="DISABLED">
                          <button class="status-toggle is-active" type="submit" title="Disable user" aria-label="Disable user" {{if isAdminUser .}}disabled{{end}}><span></span></button>
                        {{else}}
                          <input type="hidden" name="status" value="ACTIVE">
                          <button class="status-toggle" type="submit" title="Enable user" aria-label="Enable user" {{if isAdminUser .}}disabled{{end}}><span></span></button>
                        {{end}}
                      </form>
                    </td>
                    <td class="action-cell">
                      {{if eq .Role "ADMIN"}}
                        <button class="icon-button secondary" type="button" title="Admin users cannot be demoted" aria-label="Admin users cannot be demoted" disabled>{{icon "user"}}</button>
                      {{else}}
                        <form method="post" action="/vault-dashboard/users/role">
                          <input type="hidden" name="userId" value="{{.ID}}">
                          <input type="hidden" name="role" value="ADMIN">
                          <button class="icon-button secondary" type="submit" title="Make admin" aria-label="Make admin">{{icon "shield"}}</button>
                        </form>
                      {{end}}
                      <button class="icon-button danger" type="button" data-delete-user="{{.ID}}" data-delete-user-email="{{.Email}}" title="Delete user" aria-label="Delete user" {{if isAdminUser .}}disabled{{end}}>{{icon "user-x"}}</button>
                    </td>
                  </tr>
                {{end}}
              </tbody>
            </table>
          </section>
        {{end}}
      </div>

      <dialog id="delete-dialog">
        <form method="post" action="/vault-dashboard/vaults/delete">
          <h2>Delete Vault</h2>
          <p id="delete-dialog-text" class="muted">This vault will be hidden and blocked from future sync.</p>
          <input id="delete-vault-id" type="hidden" name="vaultId" value="">
          <div class="actions" style="justify-content: flex-end;">
            <button class="danger" type="submit">Yes</button>
            <button class="secondary" type="button" data-close-delete>No</button>
          </div>
        </form>
      </dialog>

      <dialog id="restore-vaults-dialog">
        <div class="section-heading">
          <h2>Deleted Vaults</h2>
          <button class="icon-button secondary" type="button" data-close-restore-vaults title="Close" aria-label="Close">{{icon "x"}}</button>
        </div>
        {{if .DeletedVaults}}
          <table class="modal-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Size</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {{range .DeletedVaults}}
                <tr>
                  <td>{{.Name}}</td>
                  <td>{{formatBytes .SizeBytes}}</td>
                  <td class="action-cell">
                    <form method="post" action="/vault-dashboard/vaults/restore">
                      <input type="hidden" name="vaultId" value="{{.ID}}">
                      <button class="icon-button secondary" type="submit" title="Restore vault" aria-label="Restore vault">{{icon "restore"}}</button>
                    </form>
                    <button class="icon-button danger" type="button" data-purge-vault="{{.ID}}" data-purge-name="{{.Name}}" title="Permanently delete vault" aria-label="Permanently delete vault">{{icon "trash"}}</button>
                  </td>
                </tr>
              {{end}}
            </tbody>
          </table>
        {{else}}
          <p class="muted">No deleted vaults can be restored.</p>
        {{end}}
      </dialog>

      <dialog id="purge-vault-dialog">
        <form method="post" action="/vault-dashboard/vaults/purge">
          <h2>Permanently Delete Vault</h2>
          <p id="purge-vault-text" class="muted">This vault will be permanently deleted and cannot be restored.</p>
          <input id="purge-vault-id" type="hidden" name="vaultId" value="">
          <div class="actions" style="justify-content: flex-end;">
            <button class="danger" type="submit">Yes</button>
            <button class="secondary" type="button" data-close-purge-vault>No</button>
          </div>
        </form>
      </dialog>

      <dialog id="delete-user-dialog">
        <form method="post" action="/vault-dashboard/users/delete">
          <h2>Delete User</h2>
          <p id="delete-user-text" class="muted">This user and their sync data will be permanently deleted.</p>
          <input id="delete-user-id" type="hidden" name="userId" value="">
          <div class="actions" style="justify-content: flex-end;">
            <button class="danger" type="submit">Yes</button>
            <button class="secondary" type="button" data-close-delete-user>No</button>
          </div>
        </form>
      </dialog>
    {{else}}
      <div class="login-panel">
        <h1>NoX Sync</h1>
        <p class="muted">Sign in with an allowlisted Google account to manage your vaults and API key.</p>
        {{if .OAuthConfigured}}
          <a class="button" href="{{.LoginURL}}">Continue with Google</a>
        {{else}}
          <p class="muted">Google OAuth is not configured. Set NOX_SYNC_GOOGLE_CLIENT_ID and NOX_SYNC_GOOGLE_CLIENT_SECRET.</p>
        {{end}}
      </div>
    {{end}}
  </main>
  <script>
    document.querySelectorAll("[data-copy]").forEach((button) => {
      button.addEventListener("click", async () => {
        const input = document.getElementById(button.dataset.copy);
        if (!input) return;
        await navigator.clipboard.writeText(input.value);
        const oldTitle = button.title;
        button.title = "Copied";
        button.setAttribute("aria-label", "Copied");
        window.setTimeout(() => {
          button.title = oldTitle;
          button.setAttribute("aria-label", oldTitle);
        }, 1200);
      });
    });

    document.querySelectorAll("[data-close-message]").forEach((button) => {
      button.addEventListener("click", () => button.closest(".message")?.remove());
    });

    const deleteDialog = document.getElementById("delete-dialog");
    const vaultInput = document.getElementById("delete-vault-id");
    const deleteDialogText = document.getElementById("delete-dialog-text");
    document.querySelectorAll("[data-delete-vault]").forEach((button) => {
      button.addEventListener("click", () => {
        if (!deleteDialog || !vaultInput || !deleteDialogText) return;
        vaultInput.value = button.dataset.deleteVault;
        deleteDialogText.textContent = 'Delete "' + button.dataset.deleteName + '"? This vault will be hidden and blocked from future sync.';
        deleteDialog.showModal();
      });
    });
    document.querySelectorAll("[data-close-delete]").forEach((button) => {
      button.addEventListener("click", () => deleteDialog && deleteDialog.close());
    });

    const restoreDialog = document.getElementById("restore-vaults-dialog");
    document.querySelectorAll("[data-open-restore-vaults]").forEach((button) => {
      button.addEventListener("click", () => restoreDialog && restoreDialog.showModal());
    });
    document.querySelectorAll("[data-close-restore-vaults]").forEach((button) => {
      button.addEventListener("click", () => restoreDialog && restoreDialog.close());
    });

    const purgeDialog = document.getElementById("purge-vault-dialog");
    const purgeVaultInput = document.getElementById("purge-vault-id");
    const purgeVaultText = document.getElementById("purge-vault-text");
    document.querySelectorAll("[data-purge-vault]").forEach((button) => {
      button.addEventListener("click", () => {
        if (!purgeDialog || !purgeVaultInput || !purgeVaultText) return;
        purgeVaultInput.value = button.dataset.purgeVault;
        purgeVaultText.textContent = 'Permanently delete "' + button.dataset.purgeName + '"? This cannot be restored.';
        purgeDialog.showModal();
      });
    });
    document.querySelectorAll("[data-close-purge-vault]").forEach((button) => {
      button.addEventListener("click", () => purgeDialog && purgeDialog.close());
    });

    const deleteUserDialog = document.getElementById("delete-user-dialog");
    const deleteUserInput = document.getElementById("delete-user-id");
    const deleteUserText = document.getElementById("delete-user-text");
    document.querySelectorAll("[data-delete-user]").forEach((button) => {
      button.addEventListener("click", () => {
        if (!deleteUserDialog || !deleteUserInput || !deleteUserText || button.disabled) return;
        deleteUserInput.value = button.dataset.deleteUser;
        deleteUserText.textContent = 'Delete "' + button.dataset.deleteUserEmail + '" and all owned sync data? This cannot be undone.';
        deleteUserDialog.showModal();
      });
    });
    document.querySelectorAll("[data-close-delete-user]").forEach((button) => {
      button.addEventListener("click", () => deleteUserDialog && deleteUserDialog.close());
    });
  </script>
</body>
</html>`))

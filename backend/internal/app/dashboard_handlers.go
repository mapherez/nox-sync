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
		data.User = user
		data.APIKey = apiKey
		data.Vaults = vaults
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
	if userID == admin.ID && status == storage.UserStatusDisabled {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Admins cannot disable their own account.")
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
	if userID == admin.ID && role != storage.UserRoleAdmin {
		writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Admins cannot demote their own account.")
		return
	}
	if err := s.store.SetUserRole(r.Context(), userID, role); err != nil {
		writeStorageError(w, err)
		return
	}
	http.Redirect(w, r, "/vault-dashboard?message=user-saved", http.StatusSeeOther)
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
	case "vault-deleted":
		return "Vault deleted."
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

type dashboardPageData struct {
	Authenticated   bool
	OAuthConfigured bool
	LoginURL        string
	ServerURL       string
	Message         string
	User            storage.User
	APIKey          string
	Vaults          []storage.Vault
	IsAdmin         bool
	Users           []storage.User
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
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
      max-width: 68rem;
    }
    header {
      align-items: center;
      display: flex;
      justify-content: space-between;
      gap: 1rem;
      margin-bottom: 1.5rem;
    }
    h1, h2 {
      letter-spacing: 0;
      margin: 0;
    }
    h1 { font-size: 1.7rem; }
    h2 { font-size: 1.05rem; margin-bottom: 0.75rem; }
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
    .copy-row, .form-row, .actions {
      display: flex;
      gap: 0.5rem;
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
    .secondary {
      background: transparent;
      border: 1px solid var(--border);
      color: var(--text);
    }
    .danger {
      background: var(--danger);
      color: #ffffff;
    }
    .message {
      background: var(--success-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      margin-bottom: 1rem;
      padding: 0.75rem 1rem;
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
      display: inline;
    }
    dialog {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: 8px;
      color: var(--text);
      max-width: 28rem;
      padding: 1rem;
    }
    dialog::backdrop {
      background: rgba(0, 0, 0, 0.42);
    }
    @media (max-width: 760px) {
      body { padding-top: 1rem; }
      header, .copy-row, .form-row { align-items: stretch; flex-direction: column; }
      .grid { grid-template-columns: 1fr; }
      table { display: block; overflow-x: auto; }
      button, .button { width: 100%; }
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
        <form method="post" action="/auth/logout">
          <button class="secondary" type="submit">Log out</button>
        </form>
      </header>

      {{if .Message}}<div class="message">{{.Message}}</div>{{end}}

      <div class="grid">
        <section>
          <h2>Server URL</h2>
          <div class="copy-row">
            <input id="server-url" readonly value="{{.ServerURL}}">
            <button type="button" data-copy="server-url">Copy</button>
          </div>
        </section>

        <section>
          <h2>API Key</h2>
          <div class="copy-row">
            <input id="api-key" readonly value="{{.APIKey}}">
            <button type="button" data-copy="api-key">Copy</button>
          </div>
          <form method="post" action="/vault-dashboard/api-key/rotate" style="margin-top: 0.75rem;">
            <button class="danger" type="submit">Generate New API Key</button>
          </form>
          <p class="muted">Generating a new API key invalidates this user's previous key.</p>
        </section>
      </div>

      <div class="stack" style="margin-top: 1rem;">
        <section>
          <h2>Vaults</h2>
          {{if .Vaults}}
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Revision</th>
                  <th>Updated</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {{range .Vaults}}
                  <tr>
                    <td>{{.Name}}</td>
                    <td>{{.Revision}}</td>
                    <td>{{.UpdatedAt}}</td>
                    <td class="actions">
                      <a class="button secondary" href="/vault-dashboard/vaults/download?vaultId={{.ID}}">Download</a>
                      <button class="danger" type="button" data-delete-vault="{{.ID}}" data-delete-name="{{.Name}}">Delete</button>
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
            <form class="form-row" method="post" action="/vault-dashboard/users/add" style="margin-bottom: 1rem;">
              <input name="email" type="email" placeholder="user@example.com" required>
              <select name="role">
                <option value="USER">User</option>
                <option value="ADMIN">Admin</option>
              </select>
              <button type="submit">Add User</button>
            </form>

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
                    <td>{{.Status}}</td>
                    <td class="actions">
                      <form method="post" action="/vault-dashboard/users/role">
                        <input type="hidden" name="userId" value="{{.ID}}">
                        {{if eq .Role "ADMIN"}}
                          <input type="hidden" name="role" value="USER">
                          <button class="secondary" type="submit">Make User</button>
                        {{else}}
                          <input type="hidden" name="role" value="ADMIN">
                          <button class="secondary" type="submit">Make Admin</button>
                        {{end}}
                      </form>
                      <form method="post" action="/vault-dashboard/users/status">
                        <input type="hidden" name="userId" value="{{.ID}}">
                        {{if eq .Status "ACTIVE"}}
                          <input type="hidden" name="status" value="DISABLED">
                          <button class="danger" type="submit">Disable</button>
                        {{else}}
                          <input type="hidden" name="status" value="ACTIVE">
                          <button type="submit">Enable</button>
                        {{end}}
                      </form>
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
        const oldText = button.textContent;
        button.textContent = "Copied";
        window.setTimeout(() => { button.textContent = oldText; }, 1200);
      });
    });

    const dialog = document.getElementById("delete-dialog");
    const vaultInput = document.getElementById("delete-vault-id");
    const dialogText = document.getElementById("delete-dialog-text");
    document.querySelectorAll("[data-delete-vault]").forEach((button) => {
      button.addEventListener("click", () => {
        if (!dialog || !vaultInput || !dialogText) return;
        vaultInput.value = button.dataset.deleteVault;
        dialogText.textContent = 'Delete "' + button.dataset.deleteName + '"? This vault will be hidden and blocked from future sync.';
        dialog.showModal();
      });
    });
    document.querySelectorAll("[data-close-delete]").forEach((button) => {
      button.addEventListener("click", () => dialog && dialog.close());
    });
  </script>
</body>
</html>`))

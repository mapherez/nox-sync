# NoX Sync — Official Technology Stack

## 1. Purpose of this document

This document defines the official technology stack for **NoX Sync**.

It is a source-of-truth document. Implementation agents must follow this stack exactly unless the product owner explicitly requests a change.

No alternative technologies should be introduced, suggested, or implemented without approval.

---

## 2. Project summary

**NoX Sync** is a private, self-hosted, manual sync system for Obsidian vaults.

The project consists of two main parts:

1. A self-hosted backend that stores and synchronizes vault files.
2. An Obsidian plugin that connects to the backend and lets the user manually sync their vault.

The product must stay simple, reliable, and easy to self-host.

The intended user experience is:

1. User runs a Docker Compose file.
2. NoX Sync backend starts.
3. User opens the local admin page in a browser.
4. User copies the server URL and API key.
5. User pastes them into the Obsidian plugin.
6. User clicks the ribbon sync button whenever they want to sync.

---

## 3. Official backend stack

### Language

**Go**

The backend must be written in Go.

Reasons:

- Produces a simple standalone binary.
- Good performance with low resource usage.
- Easy to package in Docker.
- Stable, mature, and suitable for long-running backend services.
- Avoids unnecessary runtime complexity.

---

### HTTP server

Use Go's standard HTTP capabilities unless a small router is clearly useful.

Allowed approach:

- Go standard library `net/http`
- A lightweight router only if needed

The backend must expose a simple HTTP API under:

```txt
/v1/...
```

The admin page must be served by the same backend service.

---

### Database

**SQLite**

SQLite is the official metadata database.

It stores:

- API key data
- server settings
- file metadata
- file revisions
- sync lock state
- sync sessions
- tombstones
- conflict records
- server revision state

The SQLite database must be stored inside the persistent Docker volume.

Expected path inside the container:

```txt
/data/nox-sync.db
```

---

### File storage

**Local filesystem storage**

The backend stores actual file contents on disk, not inside SQLite.

Expected persistent storage layout:

```txt
/data/
  nox-sync.db
  blobs/
  staging/
  logs/
```

`blobs/` stores finalized file content.

`staging/` stores temporary uploads before they are validated and committed.

`logs/` may be used for optional file logs, but console logs must remain the primary logging target for Docker.

---

### Blob storage model

Use content-addressed storage.

Files are stored by hash, not by original vault path.

Example:

```txt
/data/blobs/ab/cd/abcdef123456...
```

The vault path is stored in SQLite metadata.

This prevents partial overwrite issues and makes integrity validation easier.

---

### Hashing

Use **SHA-256** for file integrity.

Every uploaded file must be validated by hash before it is accepted.

A file must never be marked as synced unless its hash has been verified.

---

### Backend realtime status

Use **Server-Sent Events** for backend status updates.

SSE is used only to notify connected plugins about backend sync state changes.

It is not used for realtime file sync.

The backend should expose an endpoint similar to:

```txt
GET /v1/sync/events
```

The SSE stream must send the current sync status immediately when a client connects.

---

### API format

Use **HTTP + JSON**.

All API responses must be predictable and explicit.

Errors must use clear error codes and messages.

Example:

```json
{
  "code": "SYNC_LOCKED",
  "message": "Another sync is already in progress."
}
```

---

### Authentication model

Use a simple reusable API key.

API keys must use this prefix:

```txt
noxsync_
```

The same API key can be used on multiple devices.

NoX Sync does not use per-device API keys by default.

The backend validates whether the API key is correct. It does not require a unique key per device.

---

### Admin page

The backend must serve a small local admin/settings page.

The admin page must allow the user to:

- View the server URL.
- Copy the server URL.
- View the current API key.
- Copy the current API key.
- Generate a new API key.

Generating a new API key replaces the previous one.

The existing API key must remain visible in the admin page so the user can copy it again later.

No complex key flow is required.

Do not add:

- QR setup
- connection strings
- per-device keys
- setup tokens
- hidden-once API keys
- external account flows

---

### Backend deployment

The backend must be deployable with Docker Compose.

The preferred user experience is:

```bash
docker compose up -d
```

The backend image should eventually be publishable to a container registry so users can run NoX Sync with a minimal compose file.

Expected compose style:

```yaml
services:
  nox-sync:
    image: ghcr.io/mapherez/nox-sync:latest
    container_name: nox-sync
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
```

---

## 4. Official plugin stack

### Platform

**Obsidian plugin**

The plugin must run inside Obsidian.

It is not a separate desktop app.

---

### Language

**TypeScript**

The plugin must be written in TypeScript using the Obsidian Plugin API.

---

### UI framework

No external UI framework should be used.

The plugin UI should use:

- Obsidian Plugin API
- TypeScript
- HTML elements
- CSS

Do not introduce React, Vue, Svelte, Lit, or any other UI framework unless explicitly approved later.

---

### Button location

The sync button must be placed in the **Obsidian ribbon**.

Do not place the button beside the read/edit mode button.

Do not use DOM injection for the read/edit toolbar.

The ribbon button is the official UI location.

---

### Plugin settings

The plugin must provide a settings tab with at least:

- Server URL
- API key
- Client name
- Vault ID
- Test connection button

The same API key can be used on multiple devices.

Each plugin installation may generate and store its own local client ID for display and sync lock ownership, but this is separate from the API key.

---

### Local plugin storage

The plugin must store local sync state in its plugin data.

It must store:

- Server URL
- API key
- Client name
- local client ID
- vault ID
- last known server revision
- known file hashes
- known file revisions
- pending deleted paths
- pending conflicts

The plugin must not sync its own internal state file as vault content.

---

### Manual sync only

NoX Sync must be manual sync.

The plugin must not automatically download or apply remote changes when Obsidian opens.

When Obsidian opens, the plugin may check backend status and remote revision, but it must only update the button state.

Actual file sync must happen only when the user manually triggers it through:

- the ribbon button
- the plugin command
- a configured hotkey

---

### Backend communication

The plugin communicates with the backend using HTTP requests.

The plugin must use Obsidian-compatible request APIs where needed, especially for mobile compatibility.

The plugin listens to backend sync status through SSE when available.

If SSE is unavailable or disconnected, the plugin must fall back to explicit status checks before starting sync.

---

## 5. Official sync model

### Sync type

NoX Sync uses manual, file-based, manifest-driven sync.

It is not realtime collaborative editing.

It does not sync per keystroke.

It does not apply remote changes automatically.

---

### Sync lock

The backend must enforce one active sync session at a time.

If one device is syncing, another device must not be allowed to start a sync.

The plugin must visually block the sync button when the backend reports that a sync is already in progress.

The backend must still validate the lock when `/sync/begin` is called.

The UI state is helpful, but the backend is the authority.

---

### Manifest-based sync

The plugin sends a local manifest to the backend.

The backend compares it with the remote manifest.

The backend returns a sync plan.

The sync plan tells the plugin which files need to be:

- uploaded
- downloaded
- deleted locally
- deleted remotely
- marked as conflicts
- ignored because they are already synced

---

### File versions

The backend keeps:

- current version
- previous version

for each file.

There is no requirement for unlimited file history.

The previous version exists as a simple safety net.

---

### Deletes

Deletes must use tombstones.

A deleted file must not accidentally return from another device that still has an old copy.

---

### Conflicts

Conflicts must be explicit.

NoX Sync must not silently overwrite conflicting changes.

Markdown conflicts should be handled with a clear diff/merge UI in the plugin.

Binary conflicts should preserve both versions and let the user choose what to keep.

---

## 6. Official button states

The ribbon button must support the following states.

### UNKNOWN

The plugin has not yet confirmed backend status.

Visual behavior:

- Button disabled.
- Neutral appearance.
- Tooltip: `Checking NoX Sync status...`

---

### SYNCED

The local vault is synced with the last known remote state.

Visual behavior:

- Floppy/sync icon with reduced opacity.
- Optional small green check mark.
- Tooltip: `Vault synced.`

---

### LOCAL_DIRTY

There are local changes that have not been synced.

Visual behavior:

- Main sync icon at full opacity.
- Tooltip: `Local changes pending sync.`

---

### REMOTE_DIRTY

The backend has newer changes available.

Visual behavior:

- Download-style icon or sync icon with download indicator.
- Tooltip: `Remote changes available. Click to sync.`

No files are downloaded automatically in this state.

---

### BOTH_DIRTY

There are local changes and remote changes.

Visual behavior:

- Sync/warning style indicator.
- Tooltip: `Local and remote changes detected. Sync may require conflict resolution.`

---

### SYNCING_LOCAL

This device is currently syncing.

Visual behavior:

- Button disabled.
- Progress ring around the icon.
- Green progress indicator.
- Tooltip should include current progress when available.

---

### BLOCKED_REMOTE

Another device is syncing.

Visual behavior:

- Button disabled.
- Forbidden or lock overlay.
- Tooltip should identify the syncing device if known.

Example tooltip:

```txt
Sync in progress on iPhone.
```

---

### SERVER_UNREACHABLE

The backend cannot be reached.

Visual behavior:

- Warning/error indicator.
- Button must not start sync.
- Clicking may attempt a health check.
- Tooltip: `NoX Sync server unavailable.`

---

### CONFLICT

There are unresolved conflicts.

Visual behavior:

- Orange warning indicator.
- Click opens conflict resolution UI.
- Tooltip: `Conflicts pending.`

---

### ERROR

The previous sync failed.

Visual behavior:

- Red error indicator.
- Tooltip should include the error summary if available.

---

### AUTH_FAILED

The API key is invalid or missing.

Visual behavior:

- Button disabled.
- Red/auth warning indicator.
- Tooltip: `Authentication failed. Check NoX Sync settings.`

---

## 7. Official styling constants

Use these colors unless explicitly changed later.

```txt
Success green: #22c55e
Warning orange: #f97316
Error red: #ef4444
Muted opacity: 0.45
Active opacity: 1
Disabled opacity: 0.65
```

Progress ring:

- Circular ring around the ribbon icon.
- Implementable with CSS `conic-gradient`.
- Green while syncing.
- Orange for conflict/warning.
- Red for error.
- Controlled by a CSS variable such as `--nox-sync-progress`.

---

## 8. Explicit non-goals

NoX Sync must not implement these unless explicitly requested later:

- Realtime collaborative editing
- Auto-sync on every keystroke
- Auto-download/apply remote changes when Obsidian opens
- External accounts
- External databases
- Cloud-only services
- Per-device API keys by default
- QR setup
- Connection strings
- Complex onboarding flows
- A separate desktop app
- A web-based note editor
- Unlimited file history

---

## 9. Final stack summary

```txt
Backend language: Go
Backend database: SQLite
Backend storage: local filesystem blobs
Backend API: HTTP + JSON
Backend status updates: Server-Sent Events
Backend deployment: Docker Compose
Admin UI: minimal web page served by backend
Auth: simple reusable noxsync_ API key

Plugin language: TypeScript
Plugin platform: Obsidian Plugin API
Plugin UI: Obsidian ribbon + settings tab
Plugin sync mode: manual only
Plugin state: local plugin data
Plugin backend communication: HTTP + SSE
```

This is the official stack for NoX Sync.

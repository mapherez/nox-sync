# NoX Sync

NoX Sync is a private, self-hosted, manually triggered sync system for Obsidian vaults.

The project is intentionally split into two parts:

- `backend/` - Go backend, SQLite metadata, local filesystem blob storage, HTTP + JSON API, SSE status updates.
- `plugin/` - TypeScript Obsidian plugin using the Obsidian Plugin API, ribbon button, settings tab, and manual sync command.

The product rules and implementation milestones live in `SPECS/`.

## Current State

This repository is at Milestone 6:

- backend skeleton
- plugin skeleton
- initial API contract
- SQLite migration wiring
- persistent backend `/data` layout
- generated reusable `noxsync_` API key
- admin page with Server URL/API key display and key rotation
- health, auth check, status, and SSE status endpoints
- backend sync lock begin, heartbeat, stale lock handling, commit, and abort
- manifest planning with upload, download, delete, conflict, and no-op actions
- staged upload SHA-256 validation
- content-addressed blob promotion on commit
- current/previous backend file metadata and tombstones
- plugin settings for Server URL, API key, client name, Vault ID, and Test connection
- generated local plugin client ID and vault ID
- Obsidian ribbon sync button and `NoX Sync: Sync vault` command
- plugin button state machine for backend/auth/reachability/sync-lock states
- official plugin button colors, opacity values, icons, and progress ring
- plugin SSE status stream handling with explicit status-check fallback
- local vault manifest scanner with normalized paths, SHA-256 hashes, sizes, deletes, and last-known revisions
- plugin-local settings, credentials, and sync state excluded from the manifest
- local dirty detection from Obsidian create, modify, delete, and rename events
- full local manifest scan before manual sync/status correction
- remote/local dirty state derivation from backend revision plus local pending state
- Sync hidden files setting, with NoX Sync plugin data still excluded
- plugin manual sync begin, manifest submission, plan execution, commit, and abort-on-failure flow
- plugin upload execution for planned local changes
- plugin remote download execution with SHA-256 verification before writing
- safe local replacement/delete handling through an excluded `.nox-sync-trash/` folder
- plugin sync state updates only after successful backend commit
- backend planner handling for remote-only edits and remote deletes against unchanged local files
- conflict details persisted in plugin-local state when the backend returns conflict actions
- conflict resolver modal opened from the `CONFLICT` ribbon state
- Markdown conflict comparison with keep local, keep remote, keep both, and manual merge choices
- binary/non-Markdown conflict handling with keep local, keep remote, and keep both choices
- syncable conflict copy naming using `filename.sync-conflict.<client-name>.<date>.ext`
- explicit resolved delete/upload planner paths so user conflict choices do not loop back into conflict
- Docker scaffolding
- fixture vaults
- naming conventions

The first non-conflicting plugin sync path and initial user-facing conflict resolution are implemented. Later milestones add failure-recovery hardening and release readiness.

## Development

Backend:

```bash
cd backend
go test ./...
go run ./cmd/nox-sync
```

Plugin:

```bash
cd plugin
npm install
npm run build
```

Local backend with Docker Compose:

```bash
docker compose up --build
```

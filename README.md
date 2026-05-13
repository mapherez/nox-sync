# NoX Sync

NoX Sync is a private, self-hosted, manually triggered sync system for Obsidian vaults.

The project is intentionally split into two parts:

- `backend/` - Go backend, SQLite metadata, local filesystem blob storage, HTTP + JSON API, SSE status updates.
- `plugin/` - TypeScript Obsidian plugin using the Obsidian Plugin API, ribbon button, settings tab, and manual sync command.

The product rules and implementation milestones live in `SPECS/`.

## Current State

This repository is at Milestone 1:

- backend skeleton
- plugin skeleton
- initial API contract
- SQLite migration wiring
- persistent backend `/data` layout
- generated reusable `noxsync_` API key
- admin page with Server URL/API key display and key rotation
- health, auth check, status, and SSE status endpoints
- Docker scaffolding
- fixture vaults
- naming conventions

The sync engine is not implemented yet. Later milestones add lock enforcement, manifest planning, staging, commit behavior, dirty detection, and conflict resolution.

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

# NoX Sync

NoX Sync is a private, self-hosted sync system for Obsidian vaults. It is designed for people who want a backend they control, Google-authenticated dashboard access, per-user API keys, multiple remote vaults, and explicit manual sync instead of background cloud synchronization.

The project has two parts:

- `backend/` - Go HTTP service with SQLite metadata, local filesystem blob storage, and Docker support.
- `plugin/` - Obsidian plugin written in TypeScript using the Obsidian API, HTML, and CSS.

NoX Sync does not use external databases, external sync providers, vault sharing, realtime collaboration services, or automatic background sync.

## Features

- Manual sync from the Obsidian ribbon button or `NoX Sync: Sync vault` command.
- Self-hosted backend with persistent `/data` storage.
- Google-authenticated `/vault-dashboard` with admin allowlist management.
- One reusable `noxsync_` API key per active user.
- Multiple remote vaults per user, selected from the Obsidian plugin settings.
- HTTP + JSON API under `/v1`.
- Server-sent events for backend status updates.
- Backend-enforced per-vault sync lock with heartbeat and stale-lock recovery.
- Manifest-based planning for uploads, downloads, deletes, conflicts, and no-op actions.
- SHA-256 validation for uploads and downloads.
- Staged upload and commit flow so interrupted syncs do not change current remote state.
- Content-addressed local blob storage.
- Current and previous backend file versions plus tombstones for deletes.
- Explicit Markdown and binary conflict handling.
- Safe local replacement/delete behavior through `.nox-sync-trash/`.
- Plugin-local settings, credentials, sync state, and trash excluded from sync.

## Quick Start

Start the backend with Docker Compose:

```bash
docker compose up -d
```

Set the required dashboard environment variables before production use:

```bash
NOX_SYNC_PUBLIC_URL=https://sync.example.com
NOX_SYNC_GOOGLE_CLIENT_ID=...
NOX_SYNC_GOOGLE_CLIENT_SECRET=...
NOX_SYNC_ADMIN_EMAILS=you@example.com
```

Open the dashboard:

```text
http://localhost:5710/vault-dashboard
```

Sign in with an allowlisted Google account. Copy the Server URL and API key into the NoX Sync plugin settings in Obsidian, use **Refresh vaults**, then select or create the backend vault to sync with.

For the full first-run flow, see [User setup guide](docs/user-setup.md).

## Plugin Install

Build the plugin from source:

```bash
cd plugin
npm install
npm run build
```

The installable plugin files are written to `plugin/dist/`:

```text
main.js
manifest.json
styles.css
```

Copy those files into:

```text
<vault>/.obsidian/plugins/nox-sync/
```

Then enable NoX Sync from Obsidian's Community Plugins settings.

## Backend Data

The backend stores all persistent state under `/data`:

- `/data/nox-sync.db`
- `/data/blobs`
- `/data/staging`
- `/data/logs`

Back up the whole `/data` directory as one unit. Restoring only the database or only the blobs can leave metadata and file content out of sync. The `multi-vault` schema is a breaking change from `0.1.0`; old single-vault data is not automatically migrated.

See [Backend configuration](docs/backend-configuration.md) for environment variables and Docker details.

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
npm run typecheck
npm run test
npm run build
```

Local development backend:

```bash
docker compose -f docker-compose.dev.yml up --build
```

## Release

Backend releases are Docker images. The production Compose example uses:

```text
ghcr.io/mapherez/nox-sync:latest
```

Plugin releases should attach these files as GitHub release assets:

```text
main.js
manifest.json
styles.css
```

The release tag should match the version in `plugin/manifest.json`.

## Documentation

- [User setup guide](docs/user-setup.md)
- [Backend configuration](docs/backend-configuration.md)
- [Troubleshooting](docs/troubleshooting.md)

## Safety Model

NoX Sync treats the backend as the authority for users, vault ownership, sync locks, sessions, remote state, and commits. Files are not considered synced until content hashes are verified and the backend commit succeeds. Users cannot access each other's vaults, conflicts are explicit, and normal sync does not silently overwrite local or remote changes.

# NoX Sync

NoX Sync is a private, self-hosted sync system for Obsidian vaults. It is designed for people who want a simple backend they control, a reusable API key, and explicit manual sync instead of background cloud synchronization.

The project has two parts:

- `backend/` - Go HTTP service with SQLite metadata, local filesystem blob storage, and Docker support.
- `plugin/` - Obsidian plugin written in TypeScript using the Obsidian API, HTML, and CSS.

NoX Sync does not use external databases, external sync providers, user accounts, realtime collaboration services, or automatic background sync.

## Features

- Manual sync from the Obsidian ribbon button or `NoX Sync: Sync vault` command.
- Self-hosted backend with persistent `/data` storage.
- Reusable `noxsync_` API key shown on a local admin page.
- HTTP + JSON API under `/v1`.
- Server-sent events for backend status updates.
- Backend-enforced single sync lock with heartbeat and stale-lock recovery.
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

Open the admin page:

```text
http://localhost:8080/
```

Copy the Server URL and API key into the NoX Sync plugin settings in Obsidian, then use **Test connection**.

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

Back up the whole `/data` directory as one unit. Restoring only the database or only the blobs can leave metadata and file content out of sync.

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

See [Release checklist](docs/release-checklist.md) for the full release flow.

## Documentation

- [User setup guide](docs/user-setup.md)
- [Backend configuration](docs/backend-configuration.md)
- [Troubleshooting](docs/troubleshooting.md)
- [API contract](docs/api-contract.md)
- [Naming conventions](docs/naming-conventions.md)
- [Release checklist](docs/release-checklist.md)
- [Project specs](SPECS/1-project-description.md)
- [Official stack](SPECS/2-official-stack.md)

## Safety Model

NoX Sync treats the backend as the authority for sync locks, sessions, remote state, and commits. Files are not considered synced until content hashes are verified and the backend commit succeeds. Conflicts are explicit, and normal sync does not silently overwrite local or remote changes.

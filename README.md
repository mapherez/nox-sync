# NoX Sync - an [Obsidian](https://obsidian.md/) plugin

[![CI](https://github.com/mapherez/nox-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/ci.yml)
[![CodeQL](https://github.com/mapherez/nox-sync/actions/workflows/codeql.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/mapherez/nox-sync/badge)](https://securityscorecards.dev/viewer/?uri=github.com/mapherez/nox-sync)
[![Docker](https://github.com/mapherez/nox-sync/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/docker-publish.yml)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

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
- Dashboard vault list with revision, updated time, cloud size, download, soft delete, restore, and permanent delete.
- Plugin settings vault manager with create, select, delete, restore, permanent delete, and cloud size display.
- Admin dashboard controls for adding, enabling, disabling, deleting, and promoting users.
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
- Settings shortcut from the ribbon when no backend vault is selected.
- Local `.nox-sync-trash/` size display and clear-trash action.
- Settings-page support link for donations.
- Plugin-local settings, credentials, sync state, and trash excluded from sync.

## Install From Release

This is the recommended path for normal use.

### 1. Run The Backend

The backend is published as a Docker image:

```text
ghcr.io/mapherez/nox-sync:latest
```

Download `docker-compose.yml` from this repository into an empty folder on your server, homelab, or Docker Desktop machine:

```powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/mapherez/nox-sync/master/docker-compose.yml -OutFile docker-compose.yml
```

Create a `.env` file in the same folder:

```bash
NOX_SYNC_PUBLIC_URL=https://sync.example.com
NOX_SYNC_GOOGLE_CLIENT_ID=your-google-client-id
NOX_SYNC_GOOGLE_CLIENT_SECRET=your-google-client-secret
NOX_SYNC_ADMIN_EMAILS=you@example.com
```

For local-only testing, `NOX_SYNC_PUBLIC_URL` can be:

```bash
NOX_SYNC_PUBLIC_URL=http://localhost:5710
```

Start the backend:

```bash
docker compose up -d
```

The Compose file maps host port `5710` to container port `8080`. Open the dashboard at:

```text
http://localhost:5710/vault-dashboard
```

For a public domain, open:

```text
https://sync.example.com/vault-dashboard
```

### 2. Install The Obsidian Plugin

Download the plugin release assets from:

```text
https://github.com/mapherez/nox-sync/releases
```

You need these three files from the plugin release:

```text
main.js
manifest.json
styles.css
```

Create this folder inside each Obsidian vault where you want to use NoX Sync:

```text
<vault>/.obsidian/plugins/nox-sync/
```

Put the three release files in that folder, then enable NoX Sync from Obsidian's Community Plugins settings.

### 3. Connect The Plugin

In the backend dashboard:

- Sign in with your allowlisted Google account.
- Copy the Server URL.
- Copy your API key.

In Obsidian:

- Open NoX Sync settings.
- Paste the Server URL and API key.
- Set a readable Client name, such as `Laptop` or `Desktop`.
- Click **Test connection**.
- Create or select a backend vault.
- Use the ribbon button to manually sync.

For the full first-run flow, see [User setup guide](docs/user-setup.md).

## Google OAuth

The dashboard uses Google login. Create a Google OAuth web client and add this authorized redirect URI:

```text
https://sync.example.com/auth/google/callback
```

For local testing with the default Compose port, use:

```text
http://localhost:5710/auth/google/callback
```

`NOX_SYNC_PUBLIC_URL` must match the public origin you use in the browser. Do not include a trailing slash.

## Backend Data

The backend stores all persistent state under `/data`:

- `/data/nox-sync.db`
- `/data/blobs`
- `/data/staging`
- `/data/logs`

Back up the whole `/data` directory as one unit. Restoring only the database or only the blobs can leave metadata and file content out of sync.

Soft-deleted vaults can be restored from the dashboard or plugin. Permanently deleting a vault removes its metadata and cleans up unreferenced finalized blobs where safe.

See [Backend configuration](docs/backend-configuration.md) for environment variables and Docker details.

## Build From Source

Use this path if you want to modify the code or build everything locally.

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

The installable plugin files are written to `plugin/dist/`:

```text
main.js
manifest.json
styles.css
```

Local development backend with Docker:

```bash
docker compose -f docker-compose.dev.yml up --build
```

## Release Notes

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
- [Security policy](SECURITY.md)

## Security And Quality

NoX Sync uses GitHub Actions for backend tests, plugin type checks, plugin tests, plugin builds, and Docker image builds. CodeQL and OpenSSF Scorecard workflows are included for automated security scanning and repository health checks.

Security issues should be reported privately. See [Security Policy](SECURITY.md).

## License

NoX Sync is released under the [GNU General Public License v3.0](LICENSE). You can use, copy, modify, self-host, and redistribute it, but distributed modified versions must remain open-source under the GPL. The software is provided without warranty.

## Safety Model

NoX Sync treats the backend as the authority for users, vault ownership, sync locks, sessions, remote state, and commits. Files are not considered synced until content hashes are verified and the backend commit succeeds. Users cannot access each other's vaults, conflicts are explicit, and normal sync does not silently overwrite local or remote changes.

## Obsidian Policy Disclosures

NoX Sync is intended to be clear about the behaviors Obsidian asks plugin authors to disclose:

- Account requirement: Full sync access requires an allowlisted Google account on the self-hosted backend. The plugin itself authenticates with the user's backend API key.
- Network use: The plugin sends vault manifests, file content, sync status requests, and vault management requests only to the Server URL configured by the user. The backend dashboard uses Google OAuth for login. The optional support link opens Buy Me a Coffee only when clicked.
- Payment: No payment is required for full access. The Buy Me a Coffee link is optional.
- Telemetry: The plugin does not include client-side telemetry or analytics.
- Ads: The plugin does not load dynamic ads. The settings page includes an optional static support link.
- File access: The plugin uses Obsidian's vault APIs for files inside the currently opened vault, including `.nox-sync-trash/`. It does not access files outside the vault.
- Updates: The plugin does not include a self-update mechanism. Updates are installed through Obsidian/GitHub release files.

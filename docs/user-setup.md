# User Setup Guide

This guide covers the expected first-run path for a self-hosted NoX Sync backend and the Obsidian plugin.

## 1. Configure And Start The Backend

The backend listens on port `8080` inside the container. The provided Compose files expose it on host port `5710`.

For production, configure Google OAuth and at least one bootstrap admin email:

```bash
NOX_SYNC_PUBLIC_URL=https://sync.example.com
NOX_SYNC_GOOGLE_CLIENT_ID=...
NOX_SYNC_GOOGLE_CLIENT_SECRET=...
NOX_SYNC_ADMIN_EMAILS=you@example.com
```

The Google OAuth redirect URL must be:

```text
https://sync.example.com/auth/google/callback
```

Then start the backend from a folder containing `docker-compose.yml`:

```bash
docker compose up -d
```

## 2. Open The Dashboard

Open:

```text
http://localhost:5710/vault-dashboard
```

Sign in with an allowlisted Google account. The dashboard shows:

- Server URL
- Your reusable API key
- Your backend vaults
- Admin user management, if your account is an admin

Each user has their own API key. Generating a new key invalidates only that user's previous key, so that user's existing Obsidian devices must be updated manually.

## 3. Build Or Install The Plugin

For a source checkout:

```bash
cd plugin
npm install
npm run build
```

The build writes the manual Obsidian install package to `plugin/dist/`:

- `plugin/dist/main.js`
- `plugin/dist/manifest.json`
- `plugin/dist/styles.css`

Copy the contents of `plugin/dist/` into this Obsidian vault folder:

```text
.obsidian/plugins/nox-sync/
```

Then enable NoX Sync from Obsidian's Community Plugins settings.

## 4. Configure The Plugin

In Obsidian, open NoX Sync settings and set:

- Server URL from the backend dashboard.
- API key from the backend dashboard.
- Client name, such as `Laptop` or `Desktop`.

Use **Test connection** to verify the backend and API key. Then use **Refresh vaults** and either select an existing backend vault or create a backend vault from the current Obsidian vault name.

The selected backend vault is the remote sync target for this local Obsidian vault. Switching the selected backend vault also switches the plugin's local sync state, including known hashes, known revisions, pending deletes, and pending conflicts.

If the ribbon button shows that no backend vault is selected, clicking it opens the NoX Sync settings tab directly.

The client name is display metadata only. It is shown when this device owns a sync lock and is used in conflict-copy filenames. Changing it does not break existing backend vaults, API keys, or sync identity.

The settings page also shows the local `.nox-sync-trash/` size and includes a clear-trash action for files NoX Sync preserved during local replacement or delete operations.

## 5. Manually Sync

Use the NoX Sync ribbon button or the `NoX Sync: Sync vault` command.

The plugin does not automatically upload, download, delete, or overwrite vault files on startup. Sync is manual by design.

For a new backend vault, the first manual sync uploads the current vault. A second device receives those files only after it selects the same backend vault and performs its own manual sync.

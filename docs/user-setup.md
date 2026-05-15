# User Setup Guide

This guide covers the normal first-run path for NoX Sync: start the backend with Docker, sign in to the dashboard, install the Obsidian plugin, and sync your first vault.

## What You Need

- Docker Desktop, Docker Engine, or a server that can run Docker Compose.
- An Obsidian desktop install.
- A Google account for dashboard login.
- A Google OAuth web client for the dashboard.
- A domain or local URL for the backend.

For local testing, the default URL is:

```text
http://localhost:5710
```

For a real deployment, use your own HTTPS domain, for example:

```text
https://sync.example.com
```

## 1. Prepare Google Login

NoX Sync uses Google login only for the web dashboard. The Obsidian plugin uses the API key shown in the dashboard.

Create a Google OAuth web client and add the redirect URI that matches your backend URL:

```text
https://sync.example.com/auth/google/callback
```

For local testing with the default Docker Compose port:

```text
http://localhost:5710/auth/google/callback
```

Keep the Google Client ID and Client Secret. They are used in the backend `.env` file.

## 2. Start The Backend With Docker Compose

Create an empty folder for NoX Sync on the machine that will run the backend.

Download `docker-compose.yml` from the repository into that folder. On Windows PowerShell:

```powershell
Invoke-WebRequest -Uri https://raw.githubusercontent.com/mapherez/nox-sync/master/docker-compose.yml -OutFile docker-compose.yml
```

On macOS or Linux:

```bash
curl -L https://raw.githubusercontent.com/mapherez/nox-sync/master/docker-compose.yml -o docker-compose.yml
```

Create a file named `.env` next to `docker-compose.yml`:

```bash
NOX_SYNC_PUBLIC_URL=https://sync.example.com
NOX_SYNC_GOOGLE_CLIENT_ID=your-google-client-id
NOX_SYNC_GOOGLE_CLIENT_SECRET=your-google-client-secret
NOX_SYNC_ADMIN_EMAILS=you@example.com
```

For local testing:

```bash
NOX_SYNC_PUBLIC_URL=http://localhost:5710
NOX_SYNC_GOOGLE_CLIENT_ID=your-google-client-id
NOX_SYNC_GOOGLE_CLIENT_SECRET=your-google-client-secret
NOX_SYNC_ADMIN_EMAILS=you@example.com
```

Start the backend:

```bash
docker compose up -d
```

Docker will pull the published image automatically:

```text
ghcr.io/mapherez/nox-sync:latest
```

The provided Compose file exposes the backend on host port `5710` and stores persistent data in the Docker volume `nox-sync-data`.

## 3. Open The Dashboard

Open:

```text
http://localhost:5710/vault-dashboard
```

For a domain deployment:

```text
https://sync.example.com/vault-dashboard
```

Sign in with the admin email listed in `NOX_SYNC_ADMIN_EMAILS`.

The dashboard shows:

- Server URL.
- Your reusable API key.
- Your backend vaults.
- Vault revision, updated time, and cloud size.
- Vault download, delete, restore, and permanent delete controls.
- Admin user management, if your account is an admin.

Each user has their own API key. Generating a new key invalidates only that user's previous key, so that user's existing Obsidian devices must be updated manually.

## 4. Install The Plugin From Release Files

Download the plugin release files from:

```text
https://github.com/mapherez/nox-sync/releases
```

Download these three files:

```text
main.js
manifest.json
styles.css
```

Do not use the GitHub source code zip as the Obsidian plugin install package. The source zip contains the repository source, not just the built plugin files.

In your Obsidian vault, create this folder:

```text
.obsidian/plugins/nox-sync/
```

Copy the three downloaded files into it:

```text
.obsidian/plugins/nox-sync/main.js
.obsidian/plugins/nox-sync/manifest.json
.obsidian/plugins/nox-sync/styles.css
```

Restart Obsidian if needed, then enable NoX Sync from:

```text
Settings > Community plugins > Installed plugins
```

## 5. Configure The Plugin

In Obsidian, open NoX Sync settings and set:

- Server URL from the backend dashboard.
- API key from the backend dashboard.
- Client name, such as `Laptop` or `Desktop`.

Use **Test connection** to verify the backend and API key.

Then use the **Backend vault** section to:

- Refresh the vault list.
- Create a backend vault from the current Obsidian vault name.
- Select the backend vault you want this local vault to sync with.
- See the cloud size used by each backend vault.
- Delete, restore, or permanently delete backend vaults.

The selected backend vault is the remote sync target for this local Obsidian vault. Switching the selected backend vault also switches the plugin's local sync state, including known hashes, known revisions, pending deletes, and pending conflicts.

If the ribbon button shows that no backend vault is selected, clicking it opens the NoX Sync settings tab directly.

The client name is display metadata only. It is shown when this device owns a sync lock and is used in conflict-copy filenames. Changing it does not break existing backend vaults, API keys, or sync identity.

The settings page also shows the local `.nox-sync-trash/` size and includes a clear-trash action for files NoX Sync preserved during local replacement or delete operations.

## 6. Manually Sync

Use the NoX Sync ribbon button or the `NoX Sync: Sync vault` command.

The plugin does not automatically upload, download, delete, or overwrite vault files on startup. Sync is manual by design.

For a new backend vault, the first manual sync uploads the current vault. A second device receives those files only after it selects the same backend vault and performs its own manual sync.

## 7. Add Another User

Admin users can add more allowlisted users from the dashboard.

After the second user signs in with Google:

- They get their own API key.
- They see only their own vaults.
- They cannot access another user's vaults.
- Admins can enable, disable, promote, or delete non-admin users.

Admin users are protected from being disabled, demoted, or deleted from the dashboard.

## Build The Plugin Locally Instead

If you prefer to build the plugin from source:

```bash
git clone https://github.com/mapherez/nox-sync.git
cd nox-sync/plugin
npm install
npm run build
```

The built plugin files will be in:

```text
plugin/dist/
```

Copy `main.js`, `manifest.json`, and `styles.css` from `plugin/dist/` into:

```text
<vault>/.obsidian/plugins/nox-sync/
```

## Build The Backend Locally Instead

If you prefer to build the backend image from source:

```bash
git clone https://github.com/mapherez/nox-sync.git
cd nox-sync
docker compose -f docker-compose.dev.yml up --build
```

The development Compose file builds `./backend` locally and exposes the backend on:

```text
http://localhost:5710
```

For a standalone local image:

```bash
docker build -t nox-sync:dev ./backend
docker run --rm --name nox-sync-dev -p 5710:8080 -v nox-sync-dev-data:/data nox-sync:dev
```

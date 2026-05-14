# User Setup Guide

This guide covers the expected first-run path for a self-hosted NoX Sync backend and the Obsidian plugin.

## 1. Start The Backend

From a folder containing `docker-compose.yml`:

```bash
docker compose up -d
```

The backend listens on port `8080` by default.

## 2. Open The Admin Page

Open:

```text
http://localhost:8080/
```

Copy both values shown on the page:

- Server URL
- API key

The API key is reusable across your own Obsidian devices. Generating a new key invalidates the previous key, so existing devices must be updated manually.

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

- Server URL from the backend admin page.
- API key from the backend admin page.
- Client name, such as `Laptop` or `Desktop`.
- Vault ID. Keep the same Vault ID for devices syncing the same vault.

Use **Test connection** to verify the backend and API key.

## 5. Manually Sync

Use the NoX Sync ribbon button or the `NoX Sync: Sync vault` command.

The plugin does not automatically upload, download, delete, or overwrite vault files on startup. Sync is manual by design.

For a new backend, the first manual sync uploads the current vault. A second device receives those files only after its own manual sync.

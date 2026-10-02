# User Setup Guide

This guide covers installing the NoX Sync Obsidian plugin, connecting it to a separately managed [NoX Backend](https://github.com/mapherez/nox-backend), and manually syncing your first vault.

## What You Need

- An Obsidian installation compatible with the plugin's `manifest.json`.
- A reachable NoX Backend instance.
- The backend's Server URL and your API key.

The server is an independent project and an external runtime dependency. If you need to install or administer one, follow the [NoX Backend documentation](https://github.com/mapherez/nox-backend). Server deployment, account setup, API documentation, and backups are maintained there.

## 1. Obtain Your Server URL And API Key

Obtain the Server URL and your API key from the backend dashboard or your server administrator. The plugin uses the API key to authenticate; it does not perform the dashboard's account login.

Use the backend's base URL, for example:

```text
https://sync.example.com
```

Do not append `/v1` or `/vault-dashboard` to the Server URL. The plugin adds API paths itself. Use a URL reachable from each device; `localhost` refers to the device running Obsidian.

Keep your API key private. If you replace or regenerate it, update the plugin settings on every device using that key.

## 2. Install The Plugin From Release Files

Download these three assets from a [NoX Sync plugin release](https://github.com/mapherez/nox-sync/releases):

```text
main.js
manifest.json
styles.css
```

Do not use the GitHub source code zip as the Obsidian plugin install package. It contains repository source rather than the built plugin assets.

In your Obsidian vault, create this folder and copy the downloaded files into it:

```text
.obsidian/plugins/nox-sync/main.js
.obsidian/plugins/nox-sync/manifest.json
.obsidian/plugins/nox-sync/styles.css
```

Restart Obsidian if needed, then enable NoX Sync from:

```text
Settings > Community plugins > Installed plugins
```

## 3. Configure The Plugin

Open NoX Sync settings and set:

- **Server URL**: the backend's base URL.
- **API key**: your key from that backend.
- **Client name**: a readable device name, such as `Laptop` or `Desktop`.

Click **Test connection** to verify that the backend is reachable and the API key is valid.

The client name is display metadata. It appears when this device owns a sync lock and is used in conflict-copy filenames. Changing it does not change your credentials or remote vault selection.

## 4. Select Or Create A Remote Vault

In the **Backend vault** section of plugin settings:

1. Refresh the vault list.
2. Select an existing remote vault, or create one using the current Obsidian vault name.

The selected remote vault is the sync target for this local Obsidian vault. Switching the selection also switches the plugin's local sync state, including known hashes, known revisions, pending deletes, and pending conflicts.

This section also shows cloud size and provides delete, restore, and permanent-delete controls. Permanent deletion cannot be undone.

When no remote vault is selected, clicking the ribbon icon opens NoX Sync settings.

## 5. Manually Sync

Use the NoX Sync ribbon button or the `NoX Sync: Sync vault` command. You can assign a shortcut in Obsidian's Hotkeys settings.

The plugin does not automatically upload, download, delete, or overwrite vault files on startup. Sync is manual by design.

For a new remote vault, the first manual sync uploads the current local vault. A second device receives those files after it selects the same remote vault and performs its own manual sync.

The **Sync hidden files** setting includes dot-prefixed vault files while still excluding NoX Sync's own plugin data and local trash.

## 6. Connect Another Device

Install the same three plugin release files in the other device's local vault. Configure the same Server URL and the same user's API key, give the device its own Client name, test the connection, and select the same remote vault.

Perform a manual sync on each device when you want to exchange changes. Resolve reported conflicts explicitly rather than replacing files manually.

Additional users need their own credentials and remote vaults, managed through [NoX Backend](https://github.com/mapherez/nox-backend).

## Local Safety Copies

NoX Sync preserves replaced or deleted local files in `.nox-sync-trash/`. This folder is excluded from synchronization and is not uploaded to the backend.

Plugin settings display its size and provide a clear-trash action. Clearing it permanently removes those local safety copies. See [Troubleshooting](troubleshooting.md) for connection, conflict, and sync-state guidance.

## Build The Plugin Locally Instead

CI uses Node.js 22. To build the plugin from source:

```bash
git clone https://github.com/mapherez/nox-sync.git
cd nox-sync/plugin
npm ci
npm run typecheck
npm run test
npm run build
```

The built files are `main.js`, `manifest.json`, and `styles.css` in `plugin/dist/`, relative to the repository root. Copy them into:

```text
<vault>/.obsidian/plugins/nox-sync/
```

Building and running unit tests does not require the backend. Actual synchronization requires a separately running NoX Backend instance.

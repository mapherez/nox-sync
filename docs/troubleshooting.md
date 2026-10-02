# Troubleshooting

This guide covers the NoX Sync Obsidian plugin. For server installation, dashboard login, hosting, administration, or backend logs and backups, use the [NoX Backend documentation](https://github.com/mapherez/nox-backend).

## Plugin Does Not Appear In Obsidian

Check that these three files are inside your vault:

```text
<vault>/.obsidian/plugins/nox-sync/main.js
<vault>/.obsidian/plugins/nox-sync/manifest.json
<vault>/.obsidian/plugins/nox-sync/styles.css
```

Download the built assets from a [plugin release](https://github.com/mapherez/nox-sync/releases), rather than using GitHub's source code zip. Check the Obsidian version requirement in `manifest.json`, restart Obsidian if needed, and enable NoX Sync under Community Plugins.

## Test Connection Fails

Check the **Server URL** and **API key** in NoX Sync settings, then click **Test connection** again.

Server URL must be the backend's base URL, including its protocol and any required port, for example `https://sync.example.com`. Do not append `/v1` or `/vault-dashboard`. The URL must be reachable from the device running Obsidian.

If the server is hosted on another device, `localhost` will not point to that server. Ask your server administrator for the correct reachable URL and current API key.

## `AUTH_FAILED`

The plugin reached the backend, but authentication failed.

Check that the API key exactly matches the current key supplied by that backend. If you regenerated it, update every device using the old key. Make sure the key belongs to the user who owns the remote vault you want to select.

For disabled accounts, dashboard login problems, or server-side authorization issues, contact the server administrator or follow the [NoX Backend guidance](https://github.com/mapherez/nox-backend).

## `SERVER_UNREACHABLE`

The plugin could not reach the backend.

Verify the Server URL, network connection, protocol, and port from the device running Obsidian. Confirm with the server administrator that the backend is available at that URL, then retry **Test connection**.

Server startup, reverse proxy configuration, and server log diagnosis belong to [NoX Backend](https://github.com/mapherez/nox-backend).

## Select Or Create Backend Vault

No remote vault is selected. Open NoX Sync settings, use the **Backend vault** section, and select a remote vault or create one using the current Obsidian vault name.

If a previously selected vault was deleted, restore it through plugin settings or select a different vault before syncing again. A permanently deleted vault cannot be restored; create or select another remote vault.

When no remote vault is selected, clicking the ribbon icon opens NoX Sync settings directly.

## `BLOCKED_REMOTE`

Another sync session owns the selected remote vault's sync lock. Wait for the other device to finish before trying again.

If that device crashed or lost connectivity, its lock can remain until the backend's heartbeat timeout expires. Persistent lock problems require server-side diagnosis through [NoX Backend](https://github.com/mapherez/nox-backend). Different remote vaults can sync independently.

## `CONFLICT`

Both local and remote versions changed since the last common synced revision.

Open the conflict resolver from the NoX Sync ribbon state. Markdown conflicts can be kept local, kept remote, kept both, or manually merged. Binary conflicts preserve copies and allow keep local, keep remote, or keep both.

NoX Sync does not silently overwrite conflicting changes.

## `ERROR`

The last sync failed. Possible causes include interrupted transfers, stale sessions, hash mismatches, or missing remote content. Retry when the connection and credentials are valid.

If the error persists, preserve your local vault and collect the plugin version, steps to reproduce, and error details with secrets removed. Report plugin issues in this repository. Ask the server administrator to investigate backend failures using the [NoX Backend project](https://github.com/mapherez/nox-backend).

## Local NoX Sync Trash

NoX Sync moves replaced or deleted local files into `.nox-sync-trash/` before applying remote changes. This is local safety storage; it is excluded from synchronization and is not uploaded to the backend.

Plugin settings show its size and provide a clear-trash action. Clearing it permanently removes `.nox-sync-trash/` from the currently opened vault, including its safety copies.

## Deleted Remote Vaults Still Use Space

Soft-deleted remote vaults can be restored and may still use server storage. Plugin settings provide restore and permanent-delete controls. Permanent deletion cannot be undone.

For server storage accounting, cleanup, and backups, follow the [NoX Backend documentation](https://github.com/mapherez/nox-backend).

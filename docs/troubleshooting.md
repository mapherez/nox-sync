# Troubleshooting

NoX Sync surfaces explicit states instead of silently continuing when sync is unsafe.

## `AUTH_FAILED`

The plugin reached the backend, but the API key was missing or invalid.

Check that the API key in plugin settings exactly matches the current key shown on the backend dashboard. If you generated a new key, update every existing device manually.

In the multi-user backend, API keys are per user. Make sure the key belongs to the same Google user that owns the backend vault you are selecting.

## Select Or Create Backend Vault

The plugin reached the backend, but no remote vault is selected.

Open NoX Sync settings, use **Refresh vaults**, then select a backend vault or create one from the current Obsidian vault name. If a previously selected vault was deleted from the dashboard, select a different vault before syncing again.

## `SERVER_UNREACHABLE`

The plugin could not reach the backend.

Check that the container is running:

```bash
docker compose ps
```

Check backend logs:

```bash
docker compose logs nox-sync
```

Verify the Server URL in plugin settings, including protocol and port.

## `BLOCKED_REMOTE`

Another sync session currently owns the selected backend vault's sync lock.

Wait for the other device to finish. If that device crashed or lost connectivity, the backend heartbeat timeout eventually marks that vault's lock stale and unblocks future syncs. Different backend vaults can sync at the same time.

## `CONFLICT`

Both local and remote versions changed since the last common synced revision.

Open the conflict resolver from the NoX Sync ribbon state. Markdown conflicts can be kept local, kept remote, kept both, or manually merged. Binary conflicts preserve copies and allow keep local, keep remote, or keep both.

NoX Sync does not silently overwrite conflicting changes.

## `ERROR`

The last sync failed in a recoverable or unsafe state.

Common causes include interrupted uploads, stale sessions, hash mismatches, or missing remote content. The plugin maps known backend error codes to safe retry behavior where possible. If retry fails repeatedly, check backend logs and preserve the vault before making manual changes.

## Stale Locks

During sync, the plugin sends regular heartbeats. If Obsidian closes, the network drops, or a sync hangs, heartbeats stop. The backend marks the lock stale after expiry, removes abandoned staging content, and broadcasts the stale state to connected clients.

## Backups

Back up the complete backend `/data` directory, not just the SQLite database. The database and `/data/blobs` directory must stay together.

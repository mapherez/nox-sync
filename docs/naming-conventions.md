# NoX Sync Naming Conventions

Use these names consistently across the backend, plugin, database, API contracts, tests, and docs.

## IDs

- API keys: `noxsync_<random>`
- Client IDs: `client_<random>`
- Vault IDs: `vault_<random>`
- Sync session IDs: `sync_<random>`
- Conflict IDs: `conflict_<random>`

Random IDs should be generated with cryptographically strong randomness when used for authentication or sync ownership.

## Revisions

- `serverRevision` is the monotonically increasing backend vault revision.
- `lastKnownServerRevision` is the plugin's last committed backend revision.
- File-level `revision` is the backend revision at which the file metadata last changed.

## Hashes

- File content hashes use SHA-256.
- API fields should use `hash`, `currentHash`, `previousHash`, `expectedHash`, `actualHash`, or `baseHash`.
- Blob paths are derived from hashes under `/data/blobs`.

## Sync States

Backend states:

- `READY`
- `IDLE`
- `SYNCING`
- `FAILED`
- `STALE_LOCK`

Plugin button states:

- `UNKNOWN`
- `SYNCED`
- `LOCAL_DIRTY`
- `REMOTE_DIRTY`
- `BOTH_DIRTY`
- `SYNCING_LOCAL`
- `BLOCKED_REMOTE`
- `SERVER_UNREACHABLE`
- `CONFLICT`
- `ERROR`
- `AUTH_FAILED`

## Error Codes

Error codes use upper snake case.

Initial error code set:

- `BAD_REQUEST`
- `AUTH_REQUIRED`
- `AUTH_FAILED`
- `NOT_FOUND`
- `SYNC_LOCKED`
- `SYNC_SESSION_NOT_FOUND`
- `SYNC_SESSION_STALE`
- `HASH_MISMATCH`
- `CONFLICT_DETECTED`
- `COMMIT_FAILED`
- `SERVER_ERROR`

## Paths

- Vault paths use forward slashes in API payloads and database records.
- Paths must be relative to the vault root.
- Paths must be normalized before hashing manifests or comparing metadata.
- Paths must never be allowed to escape the vault root or backend data directory.

## Database Tables

Table names use lower snake case.

Initial tables:

- `schema_migrations`
- `server_settings`
- `api_keys`
- `server_state`
- `files`
- `file_revisions`
- `tombstones`
- `sync_locks`
- `sync_sessions`
- `staged_uploads`
- `conflicts`

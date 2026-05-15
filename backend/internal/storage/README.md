# Backend Storage

Backend metadata uses SQLite at:

```text
/data/nox-sync.db
```

File contents are stored outside SQLite:

```text
/data/
  nox-sync.db
  blobs/
  staging/
  logs/
```

## Model

The multi-user, multi-vault schema stores:

- users and Google identity metadata
- server-side web sessions
- OAuth state tokens
- per-user API keys
- per-user vaults
- vault-scoped files, revisions, tombstones, locks, sessions, plans, uploads, and conflicts

Finalized file content is stored as content-addressed blobs under `/data/blobs/{sha256}`.

Upload content is staged under `/data/staging/{sessionId}` until a sync commit succeeds or the session is aborted/reaped.

## Migrations

Migrations live in `migrations/` and are applied in numeric order. The `schema_migrations` table records applied versions.

Migration execution is wired into backend startup.

The multi-vault migration is intentionally breaking for earlier private single-vault test builds. Old single-vault metadata is not automatically migrated.

## Delete Behavior

Vault delete is soft by default:

- the vault status becomes `DELETED`
- normal sync/download/list operations hide or reject it
- the vault can be restored

Permanent delete removes the deleted vault row and cascades its vault-scoped metadata. After the transaction commits, storage cleanup removes finalized blob files that are no longer referenced by any remaining file or revision metadata.

Shared content-addressed blobs remain on disk if any remaining vault still references the same SHA-256 hash.

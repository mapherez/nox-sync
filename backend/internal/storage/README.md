# Backend Storage

Backend metadata uses SQLite at:

```txt
/data/nox-sync.db
```

File contents are stored outside SQLite:

```txt
/data/
  nox-sync.db
  blobs/
  staging/
  logs/
```

Migrations live in `migrations/` and are applied in numeric order. The `schema_migrations` table records applied versions.

Migration execution is wired into backend startup. On first boot the backend creates required directories, applies embedded migrations, inserts singleton server state rows, and generates the initial reusable `noxsync_` API key.

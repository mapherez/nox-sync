# Backend Configuration

NoX Sync backend is configured with environment variables and stores all persistent state under `/data` inside the container.

## Environment Variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `NOX_SYNC_ADDR` | `:8080` | HTTP listen address inside the container or process. |
| `NOX_SYNC_DATA_DIR` | `/data` | Persistent backend data directory. |
| `NOX_SYNC_VERSION` | `dev` | Version string returned by `/v1/health`. |

## Persistent Data

The backend creates and uses these paths under `NOX_SYNC_DATA_DIR`:

| Path | Purpose |
| --- | --- |
| `/data/nox-sync.db` | SQLite database with API key, file metadata, revisions, tombstones, locks, sessions, staged upload records, and conflict records. |
| `/data/blobs` | Finalized file contents stored by SHA-256 hash. |
| `/data/staging` | Temporary upload content for active sync sessions. |
| `/data/logs` | Reserved log directory. Docker console logs are still the primary runtime log target. |

Back up the whole `/data` directory as one unit. Restoring only the database or only the blobs can leave metadata and file content out of sync.

## Production Compose

The root `docker-compose.yml` is the minimal production-oriented example:

```bash
docker compose up -d
```

It uses the published image `ghcr.io/mapherez/nox-sync:latest`, exposes port `8080`, and stores backend data in a named Docker volume.

## Development Compose

For local source builds, use the development compose file:

```bash
docker compose -f docker-compose.dev.yml up --build
```

This builds `./backend` locally and mounts `./data` to `/data` for easy inspection while developing.

## Docker Image Build

To build the backend image locally:

```bash
docker build -t nox-sync:dev ./backend
```

For a release image:

```bash
docker build -t ghcr.io/mapherez/nox-sync:latest ./backend
```

Publishing that image is the only release step that depends on the chosen container registry. Running NoX Sync does not require external databases, external sync providers, or user accounts.

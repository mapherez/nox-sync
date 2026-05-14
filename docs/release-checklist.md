# Release Checklist

Milestone 8 release work should keep the install path simple and self-hosted.

## Backend Image

Build the backend image:

```bash
docker build -t ghcr.io/mapherez/nox-sync:latest ./backend
```

Before publishing, run:

```bash
cd backend
go test ./...
```

The published image should expose the same runtime behavior as the root `docker-compose.yml` example: HTTP on port `8080` and persistent data under `/data`.

## Plugin Package

Build and test the plugin:

```bash
cd plugin
npm install
npm run typecheck
npm run test
npm run build
```

The manual install package consists of:

- `plugin/manifest.json`
- `plugin/styles.css`
- `plugin/dist/main.js`

Do not include plugin-local sync state or credentials in any release artifact.

## Documentation

Release docs should cover:

- backend startup
- admin page URL and API key copy flow
- plugin install and configuration
- Test connection
- manual sync
- common troubleshooting states
- backup expectations for `/data`

## CI

CI should run backend tests, backend formatting checks, plugin typecheck, plugin tests, plugin build, and a Docker image build smoke test.

# NoX Sync - Implementation Milestones

## 1. Purpose

This file divides the NoX Sync implementation into practical milestones based on the project description and official technology stack.

The milestones are ordered to prove the highest-risk product guarantees early:

- manual sync only
- backend-enforced sync locking
- manifest-based planning
- staged uploads and commits
- SHA-256 validation
- explicit conflict handling
- no silent overwrites
- no external accounts, databases, or sync providers

The product owner specs remain the source of truth. This plan must not change the architecture, sync behavior, authentication model, UI placement, technology stack, or product scope.

## 2. Implementation Practices

Use these practices throughout the implementation.

### Source-of-truth discipline

- Follow the SPECS documents exactly unless the product owner explicitly changes them.
- Keep NoX Sync focused on manual Obsidian vault sync.
- Do not introduce alternate stacks, UI frameworks, external databases, cloud services, per-device API keys, QR setup, realtime collaboration, or automatic background sync.

### Data safety

- Treat the backend as the authority for sync locks, sessions, remote state, and commits.
- Never mark a file synced until its content hash has been verified and the commit has succeeded.
- Never silently overwrite local or remote changes.
- Use staging for uploads and promote staged content only during commit.
- Preserve current and previous backend file versions.
- Use tombstones for deletes so old clients do not accidentally resurrect deleted files.
- Prefer safe local write flows in the plugin: write temporary content, verify hashes, then replace or move existing files through a safe trash/conflict path.

### Backend practices

- Implement the backend in Go with SQLite, local filesystem blobs, HTTP + JSON, and SSE.
- Keep API routes versioned under `/v1/...`.
- Use explicit JSON error codes and messages.
- Store all persistent backend data under `/data`.
- Store finalized file contents by SHA-256 hash under `/data/blobs`.
- Store partial uploads under `/data/staging`.
- Keep Docker console logs as the primary logging target and never log API keys or file contents.
- Use SQLite transactions for metadata changes that must be committed atomically.
- Keep filesystem promotion and metadata updates coordinated so failed syncs cannot corrupt the current remote vault state.
- Validate vault paths to prevent path traversal and normalize path separators consistently.

### Plugin practices

- Implement the plugin in TypeScript using the Obsidian Plugin API, HTML elements, and CSS.
- Use the Obsidian ribbon for the sync button.
- Do not inject UI beside the read/edit mode control.
- Use Obsidian-compatible request APIs for desktop and mobile compatibility.
- Store credentials, client identity, vault ID, local manifests, revisions, deleted paths, and conflicts in plugin-local data.
- Exclude plugin-local settings and sync state from vault sync.
- Use vault events for responsive dirty indicators, but perform a full manifest scan before every sync.
- On startup, check backend status and revision information only. Do not download, apply, delete, overwrite, or upload vault files automatically.

### Testing practices

- Add focused unit tests for hash validation, path normalization, manifest comparison, tombstones, conflict detection, sync lock behavior, and API key handling.
- Add backend integration tests using temporary data directories and SQLite databases.
- Add plugin tests for state transitions, settings persistence, manifest scanning, exclusions, and sync command behavior where practical.
- Add end-to-end fixture tests for common sync flows: first sync, upload changes, download changes, delete propagation, binary files, conflicts, failed sync recovery, and concurrent sync rejection.
- Include failure-path tests for interrupted uploads, backend restart during sync, stale locks, invalid API keys, server unreachable, and hash mismatch.

## 3. Milestones

## Milestone 0 - Contracts and Project Skeleton

Goal: establish the repo shape and shared contracts before implementing behavior.

Deliverables:

- Backend project skeleton in Go.
- Obsidian plugin project skeleton in TypeScript.
- Shared API contract documentation for request and response shapes.
- Initial SQLite schema and migration approach.
- Initial Dockerfile and local Docker Compose shape.
- Development fixture vaults for sync testing.
- Naming conventions for sync states, error codes, revisions, sessions, manifests, tombstones, and conflict records.

Acceptance criteria:

- The backend and plugin can build from clean checkouts.
- The documented API routes are versioned under `/v1/...`.
- The chosen project layout clearly separates backend, plugin, specs, tests, and release artifacts.
- No implementation decisions conflict with the official stack.

## Milestone 1 - Backend Bootstrap, Admin Page, and API Key

Goal: make the backend runnable, persistent, and configurable through the required simple admin page.

Deliverables:

- Go HTTP server.
- Persistent `/data` initialization:
  - `/data/nox-sync.db`
  - `/data/blobs`
  - `/data/staging`
  - `/data/logs`
- SQLite initialization and schema migration on startup.
- `noxsync_` API key generation and replacement.
- Admin page at `/` showing:
  - server URL
  - current API key
  - copy controls
  - generate new API key control
- Health, status, and authentication-check endpoints.
- Dockerfile and Compose workflow for local execution.

Acceptance criteria:

- `docker compose up` starts the backend.
- Recreating the container preserves data when the mounted `/data` directory is preserved.
- The active API key is visible again later from the admin page.
- Generating a new API key invalidates the previous one.
- API keys are not logged.

## Milestone 2 - Backend Sync Core

Goal: implement the authoritative backend model for remote vault state and sync sessions.

Deliverables:

- SQLite tables for:
  - server settings
  - file metadata
  - file revisions
  - tombstones
  - sync locks
  - sync sessions
  - staged uploads
  - conflict records
  - server revision state
- Content-addressed blob storage under `/data/blobs`.
- SHA-256 validation for uploaded content.
- Sync lock begin, heartbeat, stale expiry, release, and rejection behavior.
- SSE status stream at an endpoint such as `GET /v1/sync/events`.
- Immediate current-status event when an SSE client connects.
- Sync planning endpoint that compares a submitted local manifest with the remote state.
- Commit and abort endpoints.

Acceptance criteria:

- Only one active sync session can exist at a time.
- A second sync begin request is rejected even if the client UI is stale.
- Staged uploads do not affect current remote metadata before commit.
- Hash mismatches are rejected.
- SSE clients receive current status immediately and receive updates when sync state changes.

## Milestone 3 - Plugin Foundation and Status UI

Goal: make the Obsidian plugin configurable and able to show accurate status without modifying vault files.

Deliverables:

- Obsidian plugin settings tab with:
  - Server URL
  - API key
  - client name
  - vault ID
  - Test connection button
- Local generated client ID.
- Ribbon sync button.
- `NoX Sync: Sync vault` command.
- Button state machine for:
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
- CSS for official colors, opacity values, badges, and circular progress ring.
- Backend health, auth, status, and SSE connection handling.
- Explicit status-check fallback when SSE is unavailable.

Acceptance criteria:

- Plugin startup does not upload, download, delete, apply, or overwrite any vault file.
- Invalid or missing API keys produce `AUTH_FAILED`.
- Unreachable backend produces `SERVER_UNREACHABLE`.
- Remote sync lock produces `BLOCKED_REMOTE`.
- The sync command and ribbon button follow the same behavior.

## Milestone 4 - Manifest Scanning and Dirty Detection

Goal: make the plugin accurately understand local and remote differences before any sync begins.

Deliverables:

- Local vault scanner that produces a manifest with normalized paths, hashes, sizes, timestamps where useful, deleted paths, and last-known revisions.
- Exclusion rules for plugin-local data, credentials, and sync state.
- Optional include and exclude pattern support if implemented in settings.
- Local dirty detection from Obsidian vault events.
- Full pre-sync manifest scan as the source of truth.
- Remote dirty detection from backend revision and status data.
- Local sync state persistence:
  - last known server revision
  - known file hashes
  - known file revisions
  - pending deleted paths
  - pending conflicts

Acceptance criteria:

- Local creates, edits, deletes, and renames update the visible dirty state.
- A full manifest scan can correct stale event-derived state.
- Remote changes produce `REMOTE_DIRTY` or `BOTH_DIRTY` without downloading content.
- Plugin-local settings and sync state are never included in the manifest.

## Milestone 5 - First Safe End-to-End Sync

Goal: complete the main sync path for non-conflicting files.

Deliverables:

- Plugin sync begin request with client ID, client name, and vault ID.
- Local manifest submission.
- Backend sync plan response.
- Plugin upload flow for local changed files.
- Backend staging, hash validation, and staged upload tracking.
- Plugin download flow for remote changed files.
- Plugin-side download hash verification before write/replace.
- Safe local handling for remote deletes.
- Backend commit flow that promotes staged uploads and updates metadata.
- Plugin local state update only after successful backend commit.
- Final button state update after success or failure.

Acceptance criteria:

- First sync uploads a vault to an empty backend.
- A second device can download the remote vault only after manual sync.
- Local-only changes upload correctly.
- Remote-only changes download correctly after manual sync.
- Deletes are represented by tombstones.
- Failed or interrupted sync does not corrupt remote state.
- Files are not considered synced until commit succeeds.

## Milestone 6 - Conflict Detection and Resolution

Goal: make conflicts explicit and data-safe.

Deliverables:

- Backend conflict detection when local and remote changed since the last known common synced version.
- Conflict records in backend and plugin-local state.
- `CONFLICT` button state.
- Markdown conflict UI with options:
  - keep local
  - keep remote
  - keep both
  - manually merge content
- Binary conflict handling with preserved copies and options:
  - keep local
  - keep remote
  - keep both
- Conflict copy naming format such as `filename.sync-conflict.<client-name>.<date>.ext`.
- Sync blocking or guarded behavior while unresolved conflicts exist.

Acceptance criteria:

- Conflicting Markdown changes are never silently resolved.
- Conflicting binary changes preserve both versions.
- Normal sync does not proceed as if conflicts are cleanly synced.
- User choices update local state and backend state predictably.

## Milestone 7 - Failure Recovery and Reliability Hardening

Goal: make the sync system predictable under real-world interruptions.

Deliverables:

- Sync heartbeat behavior.
- Stale lock detection and release.
- Staging cleanup for abandoned sessions.
- Backend restart recovery from persisted lock/session state.
- Clear API error codes for lock, auth, validation, conflict, stale session, unavailable content, and commit failure cases.
- Plugin retry and error-display behavior for safe cases.
- Better progress reporting for `SYNCING_LOCAL`.
- Backend and plugin tests for failure paths.

Acceptance criteria:

- Backend rejects unsafe commit attempts.
- Interrupted uploads leave remote current state unchanged.
- Stale locks eventually unblock sync.
- The plugin shows `ERROR`, `SERVER_UNREACHABLE`, or `BLOCKED_REMOTE` accurately instead of silently continuing.
- Failure paths have regression tests.

## Milestone 8 - Packaging, Documentation, and Release Readiness

Goal: make NoX Sync installable and maintainable for self-hosted users.

Deliverables:

- Production Docker image build.
- Minimal `docker-compose.yml` example.
- Backend configuration documentation.
- Plugin build/package instructions.
- User setup guide:
  - run backend
  - open admin page
  - copy server URL and API key
  - configure plugin
  - test connection
  - manually sync
- Troubleshooting guide for common states:
  - auth failed
  - server unreachable
  - blocked by another device
  - conflicts
  - failed sync
- CI checks for backend tests, plugin tests/build, formatting, and linting.

Acceptance criteria:

- A user can run the backend without cloning the source repo once an image is published.
- A user can configure a new Obsidian device with the same reusable API key.
- The documented first-run path matches the product description.
- Release artifacts do not require external accounts, external databases, or third-party sync providers.

## 4. Suggested Build Order

1. Build backend foundation and API key/admin page.
2. Build backend sync data model, lock, SSE, manifest planning, staging, and commit.
3. Build plugin settings, ribbon button, command, status checks, and SSE handling.
4. Build plugin manifest scanner and dirty detection.
5. Connect the safe non-conflicting sync path end to end.
6. Add conflict handling.
7. Harden failures, stale locks, and recovery.
8. Package, document, and prepare release workflows.

This order keeps backend authority and data integrity ahead of UI polish, while still giving the plugin enough early status behavior to validate the manual-sync user experience.

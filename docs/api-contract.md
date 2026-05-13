# NoX Sync API Contract

This document defines the shared backend/plugin API contract. The product specs remain the source of truth.

All API routes are versioned under `/v1/...`.

All non-SSE responses use JSON.

Authentication uses a reusable API key with the `noxsync_` prefix. Authenticated plugin requests should send:

```txt
Authorization: Bearer noxsync_xxxxxxxxxxxxxxxxx
```

For SSE clients that cannot attach custom headers, the backend may also accept the same key as an `api_key` query parameter on the event stream endpoint. Implementations must not log API keys.

## Common Error Shape

```json
{
  "code": "SYNC_LOCKED",
  "message": "Another sync is already in progress."
}
```

Error codes use upper snake case. Messages should be clear enough to show directly in plugin notices or logs when appropriate.

## Status Names

Backend sync status names:

- `READY`
- `IDLE`
- `SYNCING`
- `FAILED`
- `STALE_LOCK`

Plugin button status names:

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

## Routes

### `GET /`

Serves the backend admin page.

The admin page is intentionally small and must eventually show:

- server URL
- current API key
- copy controls
- generate new API key control

### `POST /admin/api-key/rotate`

Generates a new reusable API key, invalidates the previous key, and redirects back to `/`.

This endpoint is used by the admin page.

### `GET /v1/health`

Lightweight availability check.

Response:

```json
{
  "status": "ready",
  "version": "dev",
  "dataDirInitialized": true,
  "databasePath": "/data/nox-sync.db"
}
```

### `GET /v1/auth/check`

Checks whether the submitted API key is valid.

Requires authentication.

Response:

```json
{
  "ok": true
}
```

### `GET /v1/status`

Returns lightweight backend state for plugin startup and fallback checks.

Requires authentication.

Response:

```json
{
  "serverRevision": 0,
  "sync": {
    "state": "IDLE",
    "sessionId": "",
    "clientId": "",
    "clientName": "",
    "startedAt": ""
  }
}
```

### `GET /v1/sync/events`

Server-Sent Events stream for backend status changes.

Requires authentication.

The backend must send the current sync status immediately after a client connects.

Example event:

```txt
event: status
data: {"serverRevision":0,"sync":{"state":"IDLE","sessionId":"","clientId":"","clientName":"","startedAt":""}}
```

SSE is for status updates only. It is not used for realtime file sync.

### `POST /v1/sync/begin`

Begins a manual sync session if the backend lock is available.

Requires authentication.

Request:

```json
{
  "clientId": "client_...",
  "clientName": "Windows PC",
  "vaultId": "vault_..."
}
```

Success:

```json
{
  "sessionId": "sync_...",
  "serverRevision": 12,
  "heartbeatAfterSeconds": 10
}
```

Lock rejection:

```json
{
  "code": "SYNC_LOCKED",
  "message": "Another sync is already in progress."
}
```

### `POST /v1/sync/heartbeat`

Refreshes the active lock for a sync session.

Requires authentication.

Request:

```json
{
  "sessionId": "sync_...",
  "clientId": "client_..."
}
```

### `POST /v1/sync/manifest`

Submits the local manifest and returns a sync plan.

Requires authentication.

Request:

```json
{
  "sessionId": "sync_...",
  "clientId": "client_...",
  "vaultId": "vault_...",
  "lastKnownServerRevision": 11,
  "files": [
    {
      "path": "Notes/example.md",
      "hash": "sha256...",
      "size": 128,
      "lastKnownRevision": 10,
      "deleted": false
    }
  ],
  "deletedPaths": []
}
```

Deleted files may also be sent in `files` with `"deleted": true` when the client needs to preserve the file's `lastKnownRevision` for safe conflict resolution. `deletedPaths` remains valid for simple delete reporting.

Response:

```json
{
  "sessionId": "sync_...",
  "serverRevision": 12,
  "actions": [
    {
      "type": "upload",
      "path": "Notes/example.md",
      "expectedHash": "sha256..."
    },
    {
      "type": "conflict",
      "path": "Notes/deleted-remotely.md",
      "expectedHash": "local-sha256...",
      "remoteHash": "previous-remote-sha256...",
      "revision": 12,
      "remoteDeleted": true
    }
  ]
}
```

Allowed action types:

- `upload`
- `download`
- `delete_remote`
- `delete_local`
- `conflict`
- `none`

### `PUT /v1/sync/upload/{sessionId}`

Uploads one local file to backend staging.

Requires authentication.

Required query parameters:

- `clientId`
- `path`
- `hash`
- `size`

Example:

```txt
PUT /v1/sync/upload/sync_xxx?clientId=client_xxx&path=Notes/example.md&hash=<sha256>&size=128
```

The backend must calculate SHA-256 itself and reject mismatches.

### `GET /v1/files/download`

Downloads one remote file by path and expected revision/hash.

Requires authentication.

Request:

```txt
GET /v1/files/download?path=Notes/example.md
```

Response body:

```txt
application/octet-stream
```

Response headers:

- `X-NoX-Sync-Path`
- `X-NoX-Sync-Hash`
- `X-NoX-Sync-Revision`

The plugin must verify the downloaded SHA-256 hash before replacing or writing local content.

### `POST /v1/sync/commit`

Commits a sync session after all planned actions are complete.

Requires authentication.

Request:

```json
{
  "sessionId": "sync_...",
  "clientId": "client_..."
}
```

Success:

```json
{
  "serverRevision": 13
}
```

No file is considered synced until this succeeds.

### `POST /v1/sync/abort`

Aborts a sync session and releases the lock when safe.

Requires authentication.

Request:

```json
{
  "sessionId": "sync_...",
  "clientId": "client_...",
  "reason": "client cancelled"
}
```

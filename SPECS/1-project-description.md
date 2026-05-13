# NoX Sync — Project Description

## 1. Product Summary

**NoX Sync** is a private, self-hosted, manual sync system for Obsidian vaults.

The goal is to let a user run their own lightweight backend in Docker, connect the Obsidian plugin to it using a server URL and API key, and manually sync their vault across devices.

The product must be simple, reliable, predictable, and easy to install. It is intended for personal use first, but the design should be clean enough that other users can self-host it without needing external accounts, external databases, third-party sync services, or complex setup flows.

The core experience should be:

```txt
1. User starts the NoX Sync backend with Docker Compose.
2. User opens the backend web page in the browser.
3. User copies the Server URL and API key.
4. User installs the NoX Sync Obsidian plugin.
5. User pastes the Server URL and API key into the plugin settings.
6. User clicks Test Connection.
7. User manually syncs the vault using the ribbon button or command.
```

NoX Sync is not intended to be a real-time collaborative editor. It is a manual, user-controlled sync system.

---

## 2. Product Owner Rule

The product owner controls the architecture and product decisions.

Implementations must follow this document as the source of truth. The implementation must not change the architecture, data model, sync behaviour, UI behaviour, API key behaviour, or product scope unless the product owner explicitly requests that change.

The system must prioritise clarity, reliability, and simplicity over cleverness.

---

## 3. Core Product Principles

NoX Sync must follow these principles:

```txt
Manual sync only.
No automatic file download or automatic file application when Obsidian opens.
No real-time document editing.
No sync without explicit user action.
No destructive conflict resolution.
No silent overwrites.
No external accounts.
No external databases.
No third-party sync provider.
No unnecessary setup complexity.
```

The system should feel simple:

```txt
Run backend.
Copy URL.
Copy API key.
Paste into plugin.
Sync manually.
```

---

## 4. High-Level Architecture

NoX Sync has two main parts:

```txt
NoX Sync Backend
NoX Sync Obsidian Plugin
```

The backend is responsible for:

```txt
- hosting the sync API
- storing the remote vault state
- storing file metadata
- storing file contents
- managing the global sync lock
- exposing the admin web page
- exposing server status to connected plugins
- generating and displaying the API key
- validating sync requests
- detecting conflicts
- preserving the current and previous version of files
```

The Obsidian plugin is responsible for:

```txt
- showing the sync button in the Obsidian ribbon
- showing the current sync state visually
- storing the Server URL and API key locally
- detecting local vault changes
- checking whether remote changes exist
- manually starting sync when the user clicks the button or runs the command
- uploading changed local files
- downloading changed remote files
- showing conflicts clearly
- never applying remote changes automatically on startup
```

---

## 5. Backend Overview

The backend should be a single self-contained service.

It should run in Docker and expose both:

```txt
- a small web admin page
- a JSON HTTP API for the plugin
```

The backend stores its persistent data in a mounted Docker volume.

Expected backend storage layout:

```txt
/data/
  sync.db
  blobs/
  staging/
  logs/
```

`sync.db` stores metadata.

`blobs/` stores file content.

`staging/` stores temporary uploads before they are validated and committed.

`logs/` may store backend logs if file logging is implemented.

The backend must not require the user to manually create a database or connect to an external service.

---

## 6. Docker Installation Experience

The target installation experience should be as simple as possible.

A user should be able to run NoX Sync with a simple `docker-compose.yml` file.

Example target shape:

```yaml
services:
  nox-sync:
    image: ghcr.io/mapherez/nox-sync:latest
    container_name: nox-sync
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
```

The user should not need to clone the source code just to run the backend.

The backend image should include everything required to run the service.

The mounted `./data:/data` volume is where all persistent backend data lives.

Destroying and recreating the container must not delete vault data as long as the `./data` folder is preserved.

---

## 7. Backend Admin Web Page

The backend must expose a simple web admin page at the root URL.

Example:

```txt
http://localhost:8080
```

or, on a homelab:

```txt
http://SERVER-IP:8080
```

The admin page must be intentionally simple.

It should show:

```txt
NoX Sync

Server URL:
[ http://localhost:8080 ] [Copy]

API Key:
[ noxsync_xxxxxxxxxxxxxxxxx ] [Copy]

[Generate New API Key]
```

The admin page exists to make setup easy.

The user should be able to:

```txt
- copy the Server URL
- copy the existing API key again later
- generate a new API key
```

No complex admin system is required.

---

## 8. API Key Behaviour

NoX Sync uses one simple reusable API key.

The API key must use this prefix:

```txt
noxsync_
```

Example:

```txt
noxsync_xxxxxxxxxxxxxxxxxxxxxxxxx
```

The API key is a personal access token for the backend.

It is not device-specific.

The same API key may be used on multiple devices, for example:

```txt
- Windows PC
- MacBook
- iPhone
- iPad
```

The backend only needs to check whether the submitted API key is valid.

The backend must not require one API key per device.

The backend admin page must display the currently active API key so the user can copy it again later.

If the user clicks `Generate New API Key`, the backend should:

```txt
1. create a new noxsync_ API key
2. replace the previous key
3. display the new key on the admin page
4. make the previous key invalid
```

Only one active API key is required for the initial product.

The goal is simplicity.

---

## 9. Plugin Credentials

The Obsidian plugin stores the following settings locally on each device:

```txt
- Server URL
- API key
- client name
- generated local client ID
```

These plugin settings must not be synced as part of the vault sync.

The user configures each Obsidian installation manually by pasting the same Server URL and same API key into the plugin settings.

The API key can be copied again from the backend admin page whenever needed.

---

## 10. Client Identity

Although the API key is shared across devices, each plugin installation should have its own local client identity.

The plugin should generate a local `clientId` when it is first configured.

The user may also set a readable `clientName`, such as:

```txt
Windows PC
MacBook Air
iPhone
IPad
```

The backend can use `clientName` for status messages such as:

```txt
Sync in progress on iPhone
```

The client identity is for display and sync-session tracking. It is not an authentication boundary.

---

## 11. Manual Sync Behaviour

NoX Sync is manual by design.

The plugin must not automatically sync files just because Obsidian opened.

When Obsidian starts, the plugin may contact the backend to check status and remote revision information, but it must not automatically download or apply remote file changes.

Startup behaviour:

```txt
1. Plugin loads.
2. Plugin reads local settings.
3. Plugin checks backend availability.
4. Plugin checks remote revision/status.
5. Plugin updates the ribbon button state.
6. Plugin does not modify local vault files automatically.
```

If remote changes exist, the plugin should show a visual state indicating that remote changes are available.

The user must click the sync button or run the sync command to apply them.

---

## 12. Backend Sync Lock

The backend must allow only one active sync session at a time.

This is a central reliability feature.

If one device is syncing, another device must not be allowed to start a second sync session.

Example:

```txt
iPhone starts sync.
Backend marks sync as active.
Windows PC sees sync is blocked.
Windows PC button becomes disabled.
Windows PC cannot start another sync until the iPhone sync ends.
```

Even if the plugin UI has outdated state, the backend must reject a new sync request if another sync is active.

The backend is the authority.

The button reflects backend state, but it does not replace backend validation.

---

## 13. Backend Status Stream

The backend should expose a lightweight status stream so connected plugins can update their button state without polling constantly.

This stream is only for status updates.

It is not for real-time file sync.

The status stream should notify clients when:

```txt
- backend is ready
- a sync starts
- a sync ends
- a sync fails
- a sync lock becomes stale
```

When a plugin connects to the stream, the backend must immediately send the current status.

This ensures that if Obsidian opens while another device is already syncing, the plugin immediately shows the blocked state.

---

## 14. Vault Storage Model

The backend stores the vault as files plus metadata.

The backend does not need to expose the vault as a human-readable folder structure internally.

The backend should store file contents by hash under `blobs/`.

The database should store metadata such as:

```txt
- path
- current hash
- previous hash
- size
- revision
- deleted status
- updated timestamp
```

The backend should maintain:

```txt
- current version
- previous version
```

for each file.

More than one previous version is not required.

If something goes wrong, the backend should support restoring the previous version of a file.

---

## 15. File Types

NoX Sync must support all normal file types inside an Obsidian vault, including:

```txt
- Markdown files
- images
- audio files
- PDFs
- other files placed in the vault
```

Markdown files are the main focus.

Binary files are supported for completeness, but they do not require text merging.

---

## 16. Manifest-Based Sync

NoX Sync should use a manifest-based sync model.

The plugin keeps local sync state.

The backend keeps remote sync state.

During sync, the plugin sends a local manifest to the backend.

The backend compares the local manifest with the remote manifest and returns a sync plan.

The sync plan may include actions such as:

```txt
- upload local file
- download remote file
- delete remote file
- delete local file
- conflict
- no action
```

The plugin then executes the plan.

Nothing should be considered synced until the backend successfully commits the sync session.

---

## 17. Hash Validation

NoX Sync must use hashes to verify file content.

The plugin should calculate a content hash for changed files.

The backend should calculate and validate hashes for uploaded files.

A file upload must not be accepted as valid unless the backend-calculated hash matches the expected hash.

Downloaded files should also be hash-checked by the plugin before replacing or writing local content.

The goal is to avoid corrupted or partial files being marked as synced.

---

## 18. Staging and Commit

Uploads must go through staging before becoming part of the remote vault state.

Expected flow:

```txt
1. Plugin starts sync.
2. Backend grants sync lock.
3. Plugin sends manifest.
4. Backend returns sync plan.
5. Plugin uploads required files.
6. Backend stores uploads in staging.
7. Backend validates file hashes.
8. Plugin completes all required actions.
9. Plugin asks backend to commit.
10. Backend updates metadata and promotes staged files.
11. Backend releases sync lock.
```

If a sync fails midway, staged files should not corrupt the current remote state.

---

## 19. Deletes and Tombstones

File deletion must be handled safely.

If a file is deleted on one device and synced, the backend must remember that deletion using a tombstone.

This prevents deleted files from being accidentally re-uploaded by another device that still has an old local copy.

Deletes should not silently cause data loss.

The plugin should handle remote deletes carefully. A safe local trash folder may be used instead of immediate permanent deletion.

---

## 20. Conflict Behaviour

Conflicts must be explicit.

The system must not silently overwrite one side.

A conflict happens when both local and remote versions of a file changed since the last known common synced version.

For Markdown files, the plugin should show a conflict resolution UI.

For binary files, the plugin should preserve both versions and let the user choose what to keep.

Conflict handling must prioritise data safety over convenience.

---

## 21. Markdown Conflict Resolution

Markdown conflict resolution should be clear and user-controlled.

When a Markdown conflict exists, the plugin should allow the user to compare:

```txt
- local version
- remote version
```

If available, the system may also use the base version from the last known sync state to support a clearer merge.

The user should be able to choose:

```txt
- keep local version
- keep remote version
- keep both versions
- manually merge content
```

The plugin must not resolve conflicting Markdown files silently.

---

## 22. Binary Conflict Resolution

Binary files should not be merged.

If a binary file changes both locally and remotely, the system should create a conflict and preserve both versions.

Example conflict copy name:

```txt
filename.sync-conflict.<client-name>.<date>.ext
```

The user should be able to choose:

```txt
- keep local
- keep remote
- keep both
```

---

## 23. Obsidian Plugin Ribbon Button

The plugin must add a button to the Obsidian ribbon.

The button must be the main user-facing control for sync.

The button must not be placed next to the read/edit mode control.

The ribbon button must show the current state of sync clearly.

The button must support:

```txt
- click to sync when sync is allowed
- disabled state when sync is blocked
- tooltip explaining the current state
- visual icon changes for different states
- circular progress indicator while syncing
```

---

## 24. Button States

The button should support these states:

```txt
UNKNOWN
SYNCED
LOCAL_DIRTY
REMOTE_DIRTY
BOTH_DIRTY
SYNCING_LOCAL
BLOCKED_REMOTE
SERVER_UNREACHABLE
CONFLICT
ERROR
AUTH_FAILED
```

### UNKNOWN

Meaning:

```txt
The plugin has loaded but does not yet know the backend or sync state.
```

Visual:

```txt
- muted sync/save icon
- disabled
- tooltip: "Checking NoX Sync status..."
```

### SYNCED

Meaning:

```txt
The local vault matches the last known backend state.
No known local or remote changes are pending.
```

Visual:

```txt
- save/sync icon
- low opacity
- optional small green check mark
- tooltip: "Vault synced"
```

### LOCAL_DIRTY

Meaning:

```txt
There are local changes that have not been synced yet.
```

Visual:

```txt
- save/sync icon
- full opacity
- tooltip: "Local changes pending. Click to sync."
```

Click behaviour:

```txt
Start manual sync request.
```

### REMOTE_DIRTY

Meaning:

```txt
The backend has newer changes that are not applied locally.
```

Visual:

```txt
- download-style icon or sync icon with download indicator
- full opacity
- tooltip: "Remote changes available. Click to sync."
```

Click behaviour:

```txt
Start manual sync request.
```

Important:

```txt
Remote changes must not be downloaded automatically.
```

### BOTH_DIRTY

Meaning:

```txt
There are both local pending changes and remote changes available.
```

Visual:

```txt
- sync/warning icon
- full opacity
- orange indicator
- tooltip: "Local and remote changes detected. Sync may require conflict resolution."
```

Click behaviour:

```txt
Start manual sync request.
Backend determines whether conflicts exist.
```

### SYNCING_LOCAL

Meaning:

```txt
This device is currently syncing.
```

Visual:

```txt
- sync icon
- disabled
- green circular progress ring around the icon
- tooltip shows progress, for example: "Syncing 3/12 files..."
```

Click behaviour:

```txt
Disabled. No click action.
```

### BLOCKED_REMOTE

Meaning:

```txt
Another device is currently syncing.
```

Visual:

```txt
- disabled button
- forbidden/lock overlay
- full or semi-full opacity
- tooltip: "Sync in progress on <clientName>"
```

Click behaviour:

```txt
Disabled. No sync request should be started from the UI.
```

### SERVER_UNREACHABLE

Meaning:

```txt
The plugin cannot reach the backend.
```

Visual:

```txt
- warning/offline icon
- red or orange indicator
- tooltip: "NoX Sync server unreachable"
```

Click behaviour:

```txt
Try a lightweight health check.
If still unreachable, show a user notice and do not start sync.
```

### CONFLICT

Meaning:

```txt
There are unresolved conflicts.
```

Visual:

```txt
- warning icon
- orange indicator
- tooltip: "Conflicts pending. Click to resolve."
```

Click behaviour:

```txt
Open conflict resolution UI.
Do not start normal sync until conflicts are handled appropriately.
```

### ERROR

Meaning:

```txt
The last sync failed.
```

Visual:

```txt
- error indicator
- red circular ring or red badge
- tooltip explains the latest error
```

Click behaviour:

```txt
Show error details or retry if backend is reachable and sync is allowed.
```

### AUTH_FAILED

Meaning:

```txt
The configured API key is missing or invalid.
```

Visual:

```txt
- disabled error/auth icon
- red indicator
- tooltip: "Invalid NoX Sync API key"
```

Click behaviour:

```txt
Open plugin settings or show notice explaining that the API key must be fixed.
```

---

## 25. Button Styling

The button must be visually clear and consistent.

Use explicit colours:

```txt
Green:  #22c55e
Orange: #f97316
Red:    #ef4444
Muted opacity: 0.45
Active opacity: 1.0
Disabled opacity: 0.65
```

The sync progress indicator should be a circular ring around the icon.

The ring should fill clockwise as progress increases.

During successful sync progress:

```txt
ring colour: #22c55e
```

For conflict/warning states:

```txt
ring or badge colour: #f97316
```

For error states:

```txt
ring or badge colour: #ef4444
```

The progress ring may be implemented using CSS, for example with `conic-gradient` and a CSS custom property such as:

```txt
--sync-progress
```

The exact implementation may vary, but the visible result must be:

```txt
- circular progress around the ribbon icon
- green while syncing
- orange on conflict/warning
- red on error
```

---

## 26. Plugin Startup Behaviour

When Obsidian opens and the plugin loads, the plugin should:

```txt
1. load local plugin settings
2. check whether Server URL and API key are configured
3. verify backend availability
4. verify authentication
5. connect to the backend status stream if possible
6. ask the backend for lightweight status/revision info
7. compare backend revision with local last known revision
8. scan local state enough to know whether local changes exist
9. update the ribbon button state
```

The plugin must not:

```txt
- automatically download files
- automatically apply remote changes
- automatically overwrite local files
- automatically start sync
```

The plugin only informs the user through the button state.

---

## 27. Local Dirty Detection

The plugin should mark the vault as locally dirty when files are:

```txt
- created
- modified
- deleted
- renamed
```

Obsidian vault events may be used for quick UI updates.

However, before an actual sync, the plugin must perform a reliable scan and manifest comparison.

Events are for responsiveness.

The manifest scan is the source of truth before syncing.

---

## 28. Remote Dirty Detection

The plugin should detect remote changes using lightweight backend revision/status information.

If the backend server revision is newer than the plugin's last known server revision, the plugin should show `REMOTE_DIRTY` unless local changes also exist.

If both local and remote changes exist, the plugin should show `BOTH_DIRTY`.

No remote file content should be downloaded until the user manually starts sync.

---

## 29. Sync Click Behaviour

When the user clicks the ribbon button:

```txt
If state is SYNCED:
  Optionally perform a lightweight check and show "Nothing to sync".

If state is LOCAL_DIRTY:
  Request sync authorization from backend.

If state is REMOTE_DIRTY:
  Request sync authorization from backend.

If state is BOTH_DIRTY:
  Request sync authorization from backend and prepare for possible conflicts.

If state is SYNCING_LOCAL:
  Do nothing. Button is disabled.

If state is BLOCKED_REMOTE:
  Do nothing. Button is disabled.

If state is SERVER_UNREACHABLE:
  Try health check. If unavailable, show notice.

If state is CONFLICT:
  Open conflict resolution UI.

If state is ERROR:
  Show error details or allow retry if safe.

If state is AUTH_FAILED:
  Open plugin settings or show auth notice.
```

Before any actual sync, the plugin must call the backend to request authorization.

If the backend does not return approval, no sync should start.

---

## 30. Sync Flow

A normal sync should follow this order:

```txt
1. User clicks sync button or runs sync command.
2. Plugin asks backend to begin sync.
3. Backend grants lock or rejects the request.
4. Plugin scans local vault and creates local manifest.
5. Plugin sends local manifest to backend.
6. Backend returns sync plan.
7. Plugin uploads required local files.
8. Plugin downloads required remote files.
9. Plugin handles deletes safely.
10. Plugin handles conflicts if present.
11. Plugin asks backend to commit.
12. Backend validates and commits.
13. Backend releases lock.
14. Plugin updates local sync state.
15. Button changes to final state.
```

No file should be considered synced until commit succeeds.

---

## 31. Server Down Behaviour

If the backend is unreachable:

```txt
- the button should show SERVER_UNREACHABLE
- sync must not start
- the user should see a clear message when trying to sync
```

Example notice:

```txt
NoX Sync server is unreachable. Sync was not started.
```

If the backend goes down during sync:

```txt
- plugin marks sync as failed
- local files remain safe
- backend lock eventually expires if heartbeat stops
- partial staged uploads must not corrupt remote state
```

---

## 32. Commands and Hotkeys

The plugin should expose a command:

```txt
NoX Sync: Sync vault
```

The user may bind this command to a hotkey such as `Ctrl+S` if desired.

The command should behave the same as clicking the ribbon button.

The plugin must not assume that `Ctrl+S` is always assigned.

---

## 33. Settings Required in Plugin

The plugin settings should include:

```txt
Server URL
API key
Client name
Test connection button
```

Optional but useful settings:

```txt
Sync hidden files
Include patterns
Exclude patterns
Show status notices
```

The plugin must always exclude its own internal sync state and credentials from vault sync.

---

## 34. Internal Plugin State

The plugin should keep local sync state, including:

```txt
- last known server revision
- known file hashes
- known file revisions
- deleted paths pending sync
- local client ID
- client name
```

This state is necessary for safe sync and conflict detection.

This internal state must not be synced as normal vault content.

---

## 35. Expected User Experience

The desired user experience is:

```txt
I run one Docker container.
I open a small web page.
I copy the Server URL and API key.
I paste them into the Obsidian plugin.
I manually sync when I want.
The button clearly tells me whether there are local changes, remote changes, sync progress, conflicts, errors, or server problems.
The system never silently overwrites my data.
```

The product should feel private, simple, predictable, and reliable.

---

## 36. Non-Goals

NoX Sync should not become unnecessarily complex.

The initial product does not need:

```txt
- real-time collaborative editing
- automatic background sync
- automatic remote file application on startup
- complex user accounts
- per-device API keys
- advanced permission roles
- multi-user sharing UI
- cloud-hosted services
- external database setup
```

The initial product is for simple personal sync.

Future features may be added only if explicitly requested by the product owner.

---

## 37. Final Definition

NoX Sync is a private, self-hosted, manually triggered sync system for Obsidian vaults.

It consists of:

```txt
- a Dockerized backend with a small admin page
- a simple reusable noxsync_ API key
- a local Obsidian plugin
- a ribbon sync button
- manual sync only
- backend sync lock
- manifest-based file comparison
- hash validation
- safe upload/download/commit flow
- explicit conflict handling
- current and previous file versions
```

The project must stay focused on being easy to install, easy to understand, and safe to use.

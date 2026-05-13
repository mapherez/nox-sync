package storage

const (
	SyncStateIdle      = "IDLE"
	SyncStateSyncing   = "SYNCING"
	SyncStateFailed    = "FAILED"
	SyncStateStaleLock = "STALE_LOCK"

	SessionStatusActive    = "ACTIVE"
	SessionStatusCommitted = "COMMITTED"
	SessionStatusAborted   = "ABORTED"
	SessionStatusFailed    = "FAILED"

	PlanActionUpload       = "upload"
	PlanActionDownload     = "download"
	PlanActionDeleteRemote = "delete_remote"
	PlanActionDeleteLocal  = "delete_local"
	PlanActionConflict     = "conflict"
	PlanActionNone         = "none"

	PlanStatusPending   = "PENDING"
	PlanStatusCompleted = "COMPLETED"
	PlanStatusConflict  = "CONFLICT"

	UploadStatusStaged    = "STAGED"
	UploadStatusValidated = "VALIDATED"
)

// BeginSyncRequest contains the client identity for acquiring the global sync lock.
type BeginSyncRequest struct {
	ClientID   string `json:"clientId"`
	ClientName string `json:"clientName"`
	VaultID    string `json:"vaultId"`
}

// BeginSyncResult is returned after the backend grants the sync lock.
type BeginSyncResult struct {
	SessionID             string `json:"sessionId"`
	ServerRevision        int64  `json:"serverRevision"`
	HeartbeatAfterSeconds int    `json:"heartbeatAfterSeconds"`
}

// HeartbeatRequest refreshes ownership of an active sync session.
type HeartbeatRequest struct {
	SessionID string `json:"sessionId"`
	ClientID  string `json:"clientId"`
}

// ManifestRequest is the local vault manifest submitted by the plugin.
type ManifestRequest struct {
	SessionID               string         `json:"sessionId"`
	ClientID                string         `json:"clientId"`
	VaultID                 string         `json:"vaultId"`
	LastKnownServerRevision int64          `json:"lastKnownServerRevision"`
	Files                   []ManifestFile `json:"files"`
	DeletedPaths            []string       `json:"deletedPaths"`
}

// ManifestFile describes one local file in a manifest.
type ManifestFile struct {
	Path              string `json:"path"`
	Hash              string `json:"hash"`
	Size              int64  `json:"size"`
	LastKnownRevision int64  `json:"lastKnownRevision"`
	Deleted           bool   `json:"deleted"`
}

// SyncPlan is the backend's action plan for a sync session.
type SyncPlan struct {
	SessionID      string       `json:"sessionId"`
	ServerRevision int64        `json:"serverRevision"`
	Actions        []PlanAction `json:"actions"`
}

// PlanAction is one action in a sync plan.
type PlanAction struct {
	Type         string `json:"type"`
	Path         string `json:"path"`
	ExpectedHash string `json:"expectedHash,omitempty"`
	RemoteHash   string `json:"remoteHash,omitempty"`
	BaseHash     string `json:"baseHash,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Revision     int64  `json:"revision,omitempty"`
}

// CommitRequest commits a validated sync session.
type CommitRequest struct {
	SessionID string `json:"sessionId"`
	ClientID  string `json:"clientId"`
}

// CommitResult is returned after a successful commit.
type CommitResult struct {
	ServerRevision int64 `json:"serverRevision"`
}

// AbortRequest aborts an active sync session.
type AbortRequest struct {
	SessionID string `json:"sessionId"`
	ClientID  string `json:"clientId"`
	Reason    string `json:"reason"`
}

// DownloadResult points at a finalized backend blob.
type DownloadResult struct {
	Path     string
	Hash     string
	Size     int64
	Revision int64
}

export enum SyncButtonState {
  Unknown = "UNKNOWN",
  Synced = "SYNCED",
  LocalDirty = "LOCAL_DIRTY",
  RemoteDirty = "REMOTE_DIRTY",
  BothDirty = "BOTH_DIRTY",
  SyncingLocal = "SYNCING_LOCAL",
  BlockedRemote = "BLOCKED_REMOTE",
  ServerUnreachable = "SERVER_UNREACHABLE",
  Conflict = "CONFLICT",
  Error = "ERROR",
  AuthFailed = "AUTH_FAILED",
}

export const NOX_SYNC_TRASH_ROOT = ".nox-sync-trash";

export type BackendErrorActionKind =
  | "auth"
  | "sync_locked"
  | "sync_session_stale"
  | "sync_session_not_found"
  | "conflict"
  | "hash_mismatch"
  | "server_unreachable"
  | "error";

export interface BackendErrorInput {
  status: number;
  code: string;
  message: string;
}

export interface BackendErrorAction {
  kind: BackendErrorActionKind;
  state: SyncButtonState;
  detail: string;
  notice: string;
  refreshBackendStatus: boolean;
  closeStatusStream: boolean;
}

export function classifyBackendError(error: BackendErrorInput): BackendErrorAction {
  if (error.code === "AUTH_REQUIRED" || error.code === "AUTH_FAILED" || error.status === 401) {
    return {
      kind: "auth",
      state: SyncButtonState.AuthFailed,
      detail: "NoX Sync authentication failed.",
      notice: "NoX Sync authentication failed.",
      refreshBackendStatus: false,
      closeStatusStream: true,
    };
  }

  if (error.code === "SYNC_LOCKED") {
    return {
      kind: "sync_locked",
      state: SyncButtonState.BlockedRemote,
      detail: "Backend rejected sync because another sync is already in progress.",
      notice: "NoX Sync is already running on another device.",
      refreshBackendStatus: true,
      closeStatusStream: false,
    };
  }

  if (error.code === "SYNC_SESSION_STALE") {
    return {
      kind: "sync_session_stale",
      state: SyncButtonState.Error,
      detail: "NoX Sync session expired before commit. Sync was not completed.",
      notice: "NoX Sync session expired before commit. Sync was not completed.",
      refreshBackendStatus: true,
      closeStatusStream: false,
    };
  }

  if (error.code === "SYNC_SESSION_NOT_FOUND") {
    return {
      kind: "sync_session_not_found",
      state: SyncButtonState.Error,
      detail: "NoX Sync session was no longer active. Sync was not completed.",
      notice: "NoX Sync session was no longer active. Sync was not completed.",
      refreshBackendStatus: false,
      closeStatusStream: false,
    };
  }

  if (error.code === "CONFLICT_DETECTED") {
    return {
      kind: "conflict",
      state: SyncButtonState.Conflict,
      detail: "NoX Sync conflicts need to be resolved.",
      notice: "NoX Sync conflicts need to be resolved.",
      refreshBackendStatus: false,
      closeStatusStream: false,
    };
  }

  if (error.code === "HASH_MISMATCH") {
    return {
      kind: "hash_mismatch",
      state: SyncButtonState.Error,
      detail: "NoX Sync stopped because a file failed hash validation.",
      notice: "NoX Sync stopped because a file failed hash validation.",
      refreshBackendStatus: false,
      closeStatusStream: false,
    };
  }

  if (error.code === "SERVER_UNREACHABLE") {
    return {
      kind: "server_unreachable",
      state: SyncButtonState.ServerUnreachable,
      detail: "NoX Sync server is unreachable.",
      notice: "NoX Sync server is unreachable. Sync was not completed.",
      refreshBackendStatus: false,
      closeStatusStream: false,
    };
  }

  return {
    kind: "error",
    state: SyncButtonState.Error,
    detail: error.message,
    notice: error.message,
    refreshBackendStatus: false,
    closeStatusStream: false,
  };
}

export function normalizeVaultPath(rawPath: string): string | null {
  const normalized = rawPath.trim().replaceAll("\\", "/").replace(/^\/+/, "");
  if (normalized === "" || normalized === "." || normalized === ".." || normalized.startsWith("../")) {
    return null;
  }

  const parts: string[] = [];
  for (const part of normalized.split("/")) {
    if (part === "" || part === ".") {
      continue;
    }
    if (part === "..") {
      return null;
    }
    parts.push(part);
  }

  return parts.length > 0 ? parts.join("/") : null;
}

export function normalizeRequiredPath(rawPath: string): string {
  const path = normalizeVaultPath(rawPath);
  if (!path) {
    throw new Error("Invalid conflict path.");
  }
  return path;
}

export function isMarkdownPath(path: string): boolean {
  const lowerPath = path.toLowerCase();
  return lowerPath.endsWith(".md") || lowerPath.endsWith(".markdown");
}

export function isPluginInternalPath(path: string): boolean {
  return path === ".obsidian/plugins/nox-sync" || path.startsWith(".obsidian/plugins/nox-sync/");
}

export function isNoxSyncTrashPath(path: string): boolean {
  return path === NOX_SYNC_TRASH_ROOT || path.startsWith(`${NOX_SYNC_TRASH_ROOT}/`);
}

export function isHiddenPath(path: string): boolean {
  return path.split("/").some((part) => part.startsWith("."));
}

export function syncActionProgress(completed: number, total: number): number {
  if (total <= 0) {
    return 80;
  }
  return 30 + (Math.max(0, Math.min(total, completed)) / total) * 60;
}

export function parentPathsDeepestFirst(path: string): string[] {
  const normalized = normalizeVaultPath(path);
  if (!normalized) {
    return [];
  }

  const parts = normalized.split("/");
  parts.pop();

  const parents: string[] = [];
  while (parts.length > 0) {
    parents.push(parts.join("/"));
    parts.pop();
  }

  return parents;
}

export function isFolderAlreadyExistsError(error: unknown): boolean {
  let message = "";
  if (error instanceof Error) {
    message = error.message;
  } else if (typeof error === "string") {
    message = error;
  } else if (typeof error === "object" && error !== null && "message" in error) {
    const value = error.message;
    if (typeof value === "string") {
      message = value;
    }
  }
  return /folder already exists/i.test(message);
}

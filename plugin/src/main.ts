import { App, Modal, Notice, Plugin, PluginSettingTab, Setting, TAbstractFile, TFile, requestUrl, setIcon } from "obsidian";

enum SyncButtonState {
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

interface NoxSyncSettings {
  serverUrl: string;
  apiKey: string;
  clientName: string;
  clientId: string;
  vaultId: string;
  lastKnownServerRevision: number;
  knownFileHashes: Record<string, string>;
  knownFileRevisions: Record<string, number>;
  pendingDeletedPaths: string[];
  pendingConflicts: string[];
  pendingConflictDetails: Record<string, PendingConflict>;
  syncHiddenFiles: boolean;
}

interface BackendStatusResponse {
  serverRevision: number;
  sync: {
    state: string;
    sessionId: string;
    clientId: string;
    clientName: string;
    startedAt: string;
  };
}

interface LocalManifest {
  generatedAt: string;
  files: LocalManifestFile[];
  deletedPaths: string[];
}

interface LocalManifestFile {
  path: string;
  hash: string;
  size: number;
  lastKnownRevision: number;
  deleted: boolean;
}

interface BeginSyncResponse {
  sessionId: string;
  serverRevision: number;
  heartbeatAfterSeconds: number;
}

interface ManifestRequest {
  sessionId: string;
  clientId: string;
  vaultId: string;
  lastKnownServerRevision: number;
  files: LocalManifestFile[];
  deletedPaths: string[];
}

interface SyncPlanResponse {
  sessionId: string;
  serverRevision: number;
  actions: PlanAction[];
}

interface PlanAction {
  type: PlanActionType;
  path: string;
  expectedHash?: string;
  remoteHash?: string;
  baseHash?: string;
  size?: number;
  revision?: number;
  remoteDeleted?: boolean;
}

type PlanActionType = "upload" | "download" | "delete_remote" | "delete_local" | "conflict" | "none";

interface CommitResponse {
  serverRevision: number;
}

interface BackendErrorResponse {
  code?: string;
  message?: string;
}

interface DownloadedFile {
  content: ArrayBuffer;
  hash: string;
  revision: number;
}

interface PendingConflict {
  path: string;
  localHash?: string;
  remoteHash?: string;
  baseHash?: string;
  revision: number;
  localDeleted: boolean;
  remoteDeleted: boolean;
}

interface ConflictPreview {
  detail: PendingConflict;
  localText: string | null;
  remoteText: string | null;
  localHash: string;
  remoteHash: string;
  isMarkdown: boolean;
}

type ConflictResolutionChoice = "keep_local" | "keep_remote" | "keep_both" | "manual_merge";

const NOX_SYNC_TRASH_ROOT = ".nox-sync-trash";

const DEFAULT_SETTINGS: NoxSyncSettings = {
  serverUrl: "",
  apiKey: "",
  clientName: "",
  clientId: "",
  vaultId: "",
  lastKnownServerRevision: 0,
  knownFileHashes: {},
  knownFileRevisions: {},
  pendingDeletedPaths: [],
  pendingConflicts: [],
  pendingConflictDetails: {},
  syncHiddenFiles: false,
};

const STATE_CLASSES = Object.values(SyncButtonState).map(
  (state) => `nox-sync-state-${state.toLowerCase()}`,
);

class BackendRequestError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "BackendRequestError";
  }
}

class SyncConflictError extends Error {
  constructor(readonly conflictPaths: string[]) {
    super("NoX Sync conflicts were detected.");
    this.name = "SyncConflictError";
  }
}

export default class NoxSyncPlugin extends Plugin {
  settings: NoxSyncSettings = { ...DEFAULT_SETTINGS };
  private ribbonButtonEl: HTMLElement | null = null;
  private buttonState = SyncButtonState.Unknown;
  private statusEvents: EventSource | null = null;
  private reconnectTimer: number | null = null;
  private settingsRefreshTimer: number | null = null;
  private manifestScanTimer: number | null = null;
  private latestBackendStatus: BackendStatusResponse | null = null;
  private localDirty = false;

  async onload(): Promise<void> {
    await this.loadSettings();
    await this.ensureLocalIdentity();

    this.ribbonButtonEl = this.addRibbonIcon("refresh-cw", "Checking NoX Sync status...", () => {
      void this.handleManualSync();
    });
    this.ribbonButtonEl.addClass("nox-sync-ribbon");

    this.addCommand({
      id: "sync-vault",
      name: "NoX Sync: Sync vault",
      callback: () => {
        void this.handleManualSync();
      },
    });

    this.addSettingTab(new NoxSyncSettingTab(this.app, this));
    this.registerVaultEventHandlers();
    this.setButtonState(SyncButtonState.Unknown);
    if (await this.refreshLocalManifestState()) {
      const connected = await this.refreshBackendStatus();
      if (connected) {
        this.connectStatusStream();
      }
    }
  }

  onunload(): void {
    this.closeStatusStream();
    this.clearReconnectTimer();
    this.clearSettingsRefreshTimer();
    this.clearManifestScanTimer();
  }

  async loadSettings(): Promise<void> {
    const loaded = ((await this.loadData()) ?? {}) as Partial<NoxSyncSettings>;
    this.settings = {
      ...DEFAULT_SETTINGS,
      ...loaded,
      knownFileHashes: { ...(loaded.knownFileHashes ?? {}) },
      knownFileRevisions: { ...(loaded.knownFileRevisions ?? {}) },
      pendingDeletedPaths: [...(loaded.pendingDeletedPaths ?? [])],
      pendingConflicts: [...(loaded.pendingConflicts ?? [])],
      pendingConflictDetails: { ...(loaded.pendingConflictDetails ?? {}) },
    };
  }

  async saveSettings(): Promise<void> {
    await this.saveData(this.settings);
  }

  async testConnection(): Promise<void> {
    if (!this.hasCredentials()) {
      this.setButtonState(SyncButtonState.AuthFailed);
      new Notice("NoX Sync server URL and API key are required.");
      return;
    }

    try {
      const response = await requestUrl({
        url: `${this.normalizedServerUrl()}/v1/auth/check`,
        method: "GET",
        headers: this.authHeaders(),
      });

      if (response.status === 401 || response.status === 403) {
        this.setButtonState(SyncButtonState.AuthFailed);
        this.closeStatusStream();
        new Notice("NoX Sync authentication failed.");
        return;
      }

      if (response.status < 200 || response.status >= 300) {
        this.setButtonState(SyncButtonState.ServerUnreachable);
        new Notice("NoX Sync server is unavailable.");
        return;
      }

      await this.refreshBackendStatus();
      this.connectStatusStream();
      new Notice("NoX Sync connection successful.");
    } catch {
      this.setButtonState(SyncButtonState.ServerUnreachable);
      new Notice("NoX Sync server is unreachable.");
    }
  }

  queueSettingsRefresh(): void {
    this.clearSettingsRefreshTimer();
    this.settingsRefreshTimer = window.setTimeout(() => {
      this.settingsRefreshTimer = null;
      void this.refreshLocalAndBackendStatus().then((ok) => {
        if (ok) {
          this.connectStatusStream();
        }
      });
    }, 350);
  }

  private async ensureLocalIdentity(): Promise<void> {
    let changed = false;

    if (!this.settings.clientId) {
      this.settings.clientId = `client_${randomId()}`;
      changed = true;
    }

    if (!this.settings.vaultId) {
      this.settings.vaultId = `vault_${randomId()}`;
      changed = true;
    }

    if (!this.settings.clientName) {
      this.settings.clientName = this.app.vault.getName() || "Obsidian client";
      changed = true;
    }

    if (changed) {
      await this.saveSettings();
    }
  }

  private registerVaultEventHandlers(): void {
    this.registerEvent(
      this.app.vault.on("create", (file) => {
        this.handleVaultChanged(file);
      }),
    );
    this.registerEvent(
      this.app.vault.on("modify", (file) => {
        this.handleVaultChanged(file);
      }),
    );
    this.registerEvent(
      this.app.vault.on("delete", (file) => {
        void this.handleVaultDeleted(file);
      }),
    );
    this.registerEvent(
      this.app.vault.on("rename", (file, oldPath) => {
        void this.handleVaultRenamed(file, oldPath);
      }),
    );
  }

  private async refreshBackendStatus(): Promise<boolean> {
    if (!this.hasCredentials()) {
      this.setButtonState(SyncButtonState.AuthFailed);
      this.closeStatusStream();
      return false;
    }

    try {
      const response = await requestUrl({
        url: `${this.normalizedServerUrl()}/v1/status`,
        method: "GET",
        headers: this.authHeaders(),
      });

      if (response.status === 401 || response.status === 403) {
        this.setButtonState(SyncButtonState.AuthFailed);
        this.closeStatusStream();
        return false;
      }

      if (response.status < 200 || response.status >= 300) {
        this.setButtonState(SyncButtonState.ServerUnreachable);
        return false;
      }

      this.applyBackendStatus(response.json as BackendStatusResponse);
      return true;
    } catch {
      this.setButtonState(SyncButtonState.ServerUnreachable);
      return false;
    }
  }

  private async refreshLocalAndBackendStatus(): Promise<boolean> {
    if (!(await this.refreshLocalManifestState())) {
      return false;
    }

    return this.refreshBackendStatus();
  }

  private async refreshLocalManifestState(): Promise<boolean> {
    try {
      const manifest = await this.createLocalManifest();
      this.localDirty = this.manifestHasLocalChanges(manifest);
      await this.persistPendingDeletedPaths(manifest.deletedPaths);

      if (this.latestBackendStatus) {
        this.applyBackendStatus(this.latestBackendStatus);
      }

      return true;
    } catch {
      this.setButtonState(SyncButtonState.Error, "NoX Sync could not scan the local vault.");
      return false;
    }
  }

  private async handleManualSync(): Promise<void> {
    switch (this.buttonState) {
      case SyncButtonState.Synced:
        await this.refreshLocalAndBackendStatus();
        if (this.buttonState === SyncButtonState.Synced) {
          new Notice("NoX Sync: nothing to sync.");
        }
        return;

      case SyncButtonState.ServerUnreachable:
        await this.refreshBackendStatus();
        if (this.buttonState === SyncButtonState.ServerUnreachable) {
          new Notice("NoX Sync server is unreachable. Sync was not started.");
        }
        return;

      case SyncButtonState.AuthFailed:
        new Notice("Invalid NoX Sync API key. Check NoX Sync settings.");
        return;

      case SyncButtonState.BlockedRemote:
      case SyncButtonState.SyncingLocal:
        return;

      case SyncButtonState.Conflict:
        this.openConflictResolver();
        return;

      case SyncButtonState.Unknown:
        await this.refreshLocalAndBackendStatus();
        return;

      case SyncButtonState.LocalDirty:
      case SyncButtonState.RemoteDirty:
      case SyncButtonState.BothDirty:
        if (!(await this.refreshLocalAndBackendStatus())) {
          return;
        }
        if (
          this.buttonState === SyncButtonState.LocalDirty ||
          this.buttonState === SyncButtonState.RemoteDirty ||
          this.buttonState === SyncButtonState.BothDirty
        ) {
          await this.executeManualSync();
        }
        return;

      case SyncButtonState.Error:
        await this.refreshLocalAndBackendStatus();
        if (this.buttonState === SyncButtonState.Error) {
          new Notice("The previous NoX Sync sync failed.");
        }
        return;
    }
  }

  private async executeManualSync(): Promise<void> {
    if (!this.hasCredentials()) {
      this.setButtonState(SyncButtonState.AuthFailed);
      new Notice("Invalid NoX Sync API key. Check NoX Sync settings.");
      return;
    }

    let sessionId: string | null = null;
    let committed = false;

    try {
      this.clearManifestScanTimer();

      const begin = await this.beginSync();
      sessionId = begin.sessionId;
      this.setButtonState(SyncButtonState.SyncingLocal);

      const manifest = await this.createLocalManifest();
      await this.persistPendingDeletedPaths(manifest.deletedPaths);

      const plan = await this.submitManifest(begin.sessionId, manifest);
      if (plan.sessionId !== begin.sessionId) {
        throw new Error("Backend returned a sync plan for the wrong session.");
      }

      const conflicts = plan.actions.filter((action) => action.type === "conflict");
      if (conflicts.length > 0) {
        await this.persistPendingConflicts(conflicts);
        throw new SyncConflictError(conflicts.map((action) => action.path));
      }

      await this.executeSyncPlan(begin.sessionId, plan, manifest);

      const commit = await this.commitSync(begin.sessionId);
      committed = true;

      await this.applyCommittedSyncState(manifest, plan, commit.serverRevision);
      await this.refreshLocalAndBackendStatus();
      new Notice("NoX Sync complete.");
    } catch (error) {
      if (sessionId && !committed) {
        await this.abortSyncQuietly(sessionId, "Sync failed before commit.");
      }
      await this.handleSyncFailure(error);
    }
  }

  private async beginSync(): Promise<BeginSyncResponse> {
    return this.requestJSON<BeginSyncResponse>("/v1/sync/begin", "POST", {
      clientId: this.settings.clientId,
      clientName: this.settings.clientName,
      vaultId: this.settings.vaultId,
    });
  }

  private async submitManifest(sessionId: string, manifest: LocalManifest): Promise<SyncPlanResponse> {
    const request: ManifestRequest = {
      sessionId,
      clientId: this.settings.clientId,
      vaultId: this.settings.vaultId,
      lastKnownServerRevision: this.settings.lastKnownServerRevision,
      files: manifest.files,
      deletedPaths: manifest.deletedPaths,
    };

    return this.requestJSON<SyncPlanResponse>("/v1/sync/manifest", "POST", request);
  }

  private async executeSyncPlan(sessionId: string, plan: SyncPlanResponse, manifest: LocalManifest): Promise<void> {
    for (const action of plan.actions) {
      switch (action.type) {
        case "upload":
          await this.uploadPlannedFile(sessionId, action, manifest);
          break;
        case "download":
          await this.downloadPlannedFile(action, manifest);
          break;
        case "delete_local":
          await this.applyRemoteDelete(action, manifest);
          break;
        case "delete_remote":
          this.assertLocalPathStillAbsent(this.normalizedActionPath(action));
          break;
        case "none":
          break;
        case "conflict":
          throw new SyncConflictError([action.path]);
      }
    }
  }

  private async uploadPlannedFile(sessionId: string, action: PlanAction, manifest: LocalManifest): Promise<void> {
    const path = this.normalizedActionPath(action);
    const manifestFile = this.requireManifestFile(manifest, path);
    const expectedHash = action.expectedHash ?? manifestFile.hash;
    if (expectedHash !== manifestFile.hash) {
      throw new Error(`Upload plan hash does not match local manifest for ${path}.`);
    }

    const content = await this.readManifestFileContent(manifestFile);
    const query = new URLSearchParams({
      clientId: this.settings.clientId,
      path,
      hash: expectedHash,
      size: String(content.byteLength),
    });

    await this.requestUpload(`/v1/sync/upload/${encodeURIComponent(sessionId)}?${query.toString()}`, content);
  }

  private async downloadPlannedFile(action: PlanAction, manifest: LocalManifest): Promise<void> {
    const downloaded = await this.downloadRemoteFile(action);
    await this.writeDownloadedFile(action, manifest, downloaded);
  }

  private async downloadRemoteFile(action: PlanAction): Promise<DownloadedFile> {
    const path = this.normalizedActionPath(action);
    const query = new URLSearchParams({ path });
    const response = await this.requestBinary(`/v1/files/download?${query.toString()}`);
    const headerHash = responseHeader(response, "X-NoX-Sync-Hash");
    const expectedHash = action.remoteHash || headerHash;
    if (!expectedHash) {
      throw new Error(`Backend did not provide a hash for ${path}.`);
    }
    if (headerHash && headerHash !== expectedHash) {
      throw new Error(`Downloaded hash header does not match the sync plan for ${path}.`);
    }

    const content = response.arrayBuffer;
    const actualHash = await sha256Hex(content);
    if (actualHash !== expectedHash) {
      throw new Error(`Downloaded file failed SHA-256 verification for ${path}.`);
    }
    if (typeof action.size === "number" && action.size >= 0 && content.byteLength !== action.size) {
      throw new Error(`Downloaded file size does not match the sync plan for ${path}.`);
    }
    const headerRevision = numberFromHeader(responseHeader(response, "X-NoX-Sync-Revision"));
    if (headerRevision !== null && typeof action.revision === "number" && headerRevision !== action.revision) {
      throw new Error(`Downloaded file revision does not match the sync plan for ${path}.`);
    }

    return {
      content,
      hash: actualHash,
      revision: headerRevision ?? action.revision ?? 0,
    };
  }

  private async writeDownloadedFile(
    action: PlanAction,
    manifest: LocalManifest,
    downloaded: DownloadedFile,
  ): Promise<void> {
    const path = this.normalizedActionPath(action);
    const manifestFile = this.findManifestFile(manifest, path);
    const currentFile = await this.assertLocalPathStillMatchesManifest(path, manifestFile);

    await this.ensureParentFolders(path);
    if (currentFile) {
      await this.moveFileToSyncTrash(currentFile);
    }

    await this.app.vault.createBinary(path, downloaded.content);

    const written = this.getVaultFile(path);
    if (!written) {
      throw new Error(`Downloaded file was not written: ${path}.`);
    }

    const writtenHash = await this.hashVaultFile(written);
    if (writtenHash !== downloaded.hash) {
      throw new Error(`Written file failed SHA-256 verification for ${path}.`);
    }
  }

  private async applyRemoteDelete(action: PlanAction, manifest: LocalManifest): Promise<void> {
    const path = this.normalizedActionPath(action);
    const manifestFile = this.findManifestFile(manifest, path);
    const currentFile = await this.assertLocalPathStillMatchesManifest(path, manifestFile);

    if (currentFile) {
      await this.moveFileToSyncTrash(currentFile);
    }
  }

  private async commitSync(sessionId: string): Promise<CommitResponse> {
    return this.requestJSON<CommitResponse>("/v1/sync/commit", "POST", {
      sessionId,
      clientId: this.settings.clientId,
    });
  }

  private async abortSyncQuietly(sessionId: string, reason: string): Promise<void> {
    try {
      await this.requestJSON("/v1/sync/abort", "POST", {
        sessionId,
        clientId: this.settings.clientId,
        reason,
      });
    } catch {
      // The backend lock will expire if the abort cannot be delivered.
    }
  }

  private async requestJSON<T>(path: string, method: string, body?: unknown): Promise<T> {
    let response;
    try {
      response = await requestUrl({
        url: `${this.normalizedServerUrl()}${path}`,
        method,
        headers: {
          ...this.authHeaders(),
          "Content-Type": "application/json",
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch {
      throw new BackendRequestError(0, "SERVER_UNREACHABLE", "NoX Sync server is unreachable.");
    }

    ensureResponseOK(response);
    return response.json as T;
  }

  private async requestUpload(path: string, content: ArrayBuffer): Promise<void> {
    let response;
    try {
      response = await requestUrl({
        url: `${this.normalizedServerUrl()}${path}`,
        method: "PUT",
        headers: {
          ...this.authHeaders(),
          "Content-Type": "application/octet-stream",
        },
        body: content,
      });
    } catch {
      throw new BackendRequestError(0, "SERVER_UNREACHABLE", "NoX Sync server is unreachable.");
    }

    ensureResponseOK(response);
  }

  private async requestBinary(path: string) {
    let response;
    try {
      response = await requestUrl({
        url: `${this.normalizedServerUrl()}${path}`,
        method: "GET",
        headers: this.authHeaders(),
      });
    } catch {
      throw new BackendRequestError(0, "SERVER_UNREACHABLE", "NoX Sync server is unreachable.");
    }

    ensureResponseOK(response);
    return response;
  }

  private async handleSyncFailure(error: unknown): Promise<void> {
    if (error instanceof SyncConflictError) {
      this.setButtonState(SyncButtonState.Conflict);
      new Notice(`NoX Sync found ${error.conflictPaths.length} conflict(s).`);
      return;
    }

    if (error instanceof BackendRequestError) {
      if (error.code === "AUTH_REQUIRED" || error.code === "AUTH_FAILED" || error.status === 401) {
        this.setButtonState(SyncButtonState.AuthFailed);
        this.closeStatusStream();
        new Notice("NoX Sync authentication failed.");
        return;
      }

      if (error.code === "SYNC_LOCKED") {
        await this.refreshBackendStatus();
        new Notice("NoX Sync is already running on another device.");
        return;
      }

      if (error.code === "CONFLICT_DETECTED") {
        this.setButtonState(SyncButtonState.Conflict);
        new Notice("NoX Sync conflicts need to be resolved.");
        return;
      }

      if (error.code === "SERVER_UNREACHABLE") {
        this.setButtonState(SyncButtonState.ServerUnreachable);
        new Notice("NoX Sync server is unreachable. Sync was not completed.");
        return;
      }

      this.setButtonState(SyncButtonState.Error, error.message);
      new Notice(error.message);
      return;
    }

    const message = error instanceof Error ? error.message : "NoX Sync failed.";
    this.setButtonState(SyncButtonState.Error, message);
    new Notice(message);
  }

  private openConflictResolver(): void {
    if (this.settings.pendingConflicts.length === 0) {
      new Notice("NoX Sync has no stored conflicts to resolve.");
      void this.refreshLocalAndBackendStatus();
      return;
    }

    new ConflictResolutionModal(this.app, this).open();
  }

  async getConflictPreview(path: string): Promise<ConflictPreview> {
    const normalizedPath = normalizeRequiredPath(path);
    const detail = this.pendingConflictDetail(normalizedPath);
    const isMarkdown = isMarkdownPath(normalizedPath);

    let localHash = "";
    let localText: string | null = null;
    const localFile = this.getVaultFile(normalizedPath);
    if (localFile) {
      const localContent = await this.app.vault.readBinary(localFile);
      localHash = await sha256Hex(localContent);
      if (isMarkdown) {
        localText = textFromArrayBuffer(localContent);
      }
    }

    let remoteHash = detail.remoteHash ?? "";
    let remoteText: string | null = null;
    if (!detail.remoteDeleted) {
      const downloaded = await this.downloadConflictRemote(detail);
      remoteHash = downloaded.hash;
      if (!detail.remoteHash || detail.revision === 0) {
        detail.remoteHash = downloaded.hash;
        detail.revision = downloaded.revision || detail.revision;
        this.settings.pendingConflictDetails[normalizedPath] = detail;
        await this.saveSettings();
      }
      if (isMarkdown) {
        remoteText = textFromArrayBuffer(downloaded.content);
      }
    }

    return {
      detail,
      localText,
      remoteText,
      localHash,
      remoteHash,
      isMarkdown,
    };
  }

  async resolvePendingConflict(
    path: string,
    choice: ConflictResolutionChoice,
    mergedText = "",
  ): Promise<string> {
    const normalizedPath = normalizeRequiredPath(path);
    const detail = this.pendingConflictDetail(normalizedPath);

    switch (choice) {
      case "keep_local":
        await this.resolveByKeepingLocal(normalizedPath, detail);
        break;
      case "keep_remote":
        await this.resolveByKeepingRemote(normalizedPath, detail);
        break;
      case "keep_both":
        await this.resolveByKeepingBoth(normalizedPath, detail);
        break;
      case "manual_merge":
        await this.resolveByManualMerge(normalizedPath, detail, mergedText);
        break;
    }

    await this.clearResolvedConflict(normalizedPath);
    await this.refreshLocalManifestState();

    if (this.settings.pendingConflicts.length === 0) {
      return "Conflict resolved. Click sync to commit the resolved vault state.";
    }
    return "Conflict resolved.";
  }

  private pendingConflictDetail(path: string): PendingConflict {
    const detail = this.settings.pendingConflictDetails[path];
    if (detail) {
      return {
        ...detail,
        path,
        revision: detail.revision ?? 0,
        localDeleted: detail.localDeleted ?? false,
        remoteDeleted: detail.remoteDeleted ?? false,
      };
    }

    return {
      path,
      revision: this.settings.knownFileRevisions[path] ?? 0,
      localDeleted: this.getVaultFile(path) === null,
      remoteDeleted: false,
    };
  }

  private async resolveByKeepingLocal(path: string, detail: PendingConflict): Promise<void> {
    const localFile = this.getVaultFile(path);

    if (!localFile) {
      this.settings.pendingDeletedPaths = [...new Set([...this.settings.pendingDeletedPaths, path])].sort();
      this.settings.knownFileHashes[path] = detail.remoteHash ?? this.settings.knownFileHashes[path] ?? "";
      this.settings.knownFileRevisions[path] = detail.revision;
      this.localDirty = true;
      await this.saveSettings();
      return;
    }

    const localHash = await this.hashVaultFile(localFile);
    this.settings.knownFileHashes[path] = detail.remoteHash ?? this.settings.knownFileHashes[path] ?? localHash;
    this.settings.knownFileRevisions[path] = detail.revision;
    this.settings.pendingDeletedPaths = this.settings.pendingDeletedPaths.filter((deletedPath) => deletedPath !== path);
    this.localDirty = true;
    await this.saveSettings();
  }

  private async resolveByKeepingRemote(path: string, detail: PendingConflict): Promise<void> {
    if (detail.remoteDeleted) {
      const localFile = this.getVaultFile(path);
      if (localFile) {
        await this.moveFileToSyncTrash(localFile);
      }
      delete this.settings.knownFileHashes[path];
      delete this.settings.knownFileRevisions[path];
      this.settings.pendingDeletedPaths = this.settings.pendingDeletedPaths.filter((deletedPath) => deletedPath !== path);
      await this.saveSettings();
      return;
    }

    const downloaded = await this.downloadConflictRemote(detail);
    await this.replaceVaultFileWithContent(path, downloaded.content);
    this.settings.knownFileHashes[path] = downloaded.hash;
    this.settings.knownFileRevisions[path] = downloaded.revision || detail.revision;
    this.settings.pendingDeletedPaths = this.settings.pendingDeletedPaths.filter((deletedPath) => deletedPath !== path);
    await this.saveSettings();
  }

  private async resolveByKeepingBoth(path: string, detail: PendingConflict): Promise<void> {
    const localFile = this.getVaultFile(path);
    if (localFile) {
      await this.moveFileToConflictCopy(localFile);
    }

    if (detail.remoteDeleted) {
      delete this.settings.knownFileHashes[path];
      delete this.settings.knownFileRevisions[path];
      this.settings.pendingDeletedPaths = this.settings.pendingDeletedPaths.filter((deletedPath) => deletedPath !== path);
      this.localDirty = true;
      await this.saveSettings();
      return;
    }

    const downloaded = await this.downloadConflictRemote(detail);
    await this.ensureParentFolders(path);
    if (this.app.vault.getAbstractFileByPath(path) !== null) {
      throw new Error(`Cannot restore remote file because ${path} still exists.`);
    }
    await this.app.vault.createBinary(path, downloaded.content);
    this.settings.knownFileHashes[path] = downloaded.hash;
    this.settings.knownFileRevisions[path] = downloaded.revision || detail.revision;
    this.settings.pendingDeletedPaths = this.settings.pendingDeletedPaths.filter((deletedPath) => deletedPath !== path);
    this.localDirty = true;
    await this.saveSettings();
  }

  private async resolveByManualMerge(path: string, detail: PendingConflict, mergedText: string): Promise<void> {
    if (detail.remoteDeleted || !isMarkdownPath(path)) {
      throw new Error("Manual merge is only available when both sides have Markdown content.");
    }
    if (mergedText.trim() === "") {
      throw new Error("Manual merge content cannot be empty.");
    }

    const content = arrayBufferFromText(mergedText);
    await this.replaceVaultFileWithContent(path, content);
    this.settings.knownFileHashes[path] = detail.remoteHash ?? this.settings.knownFileHashes[path] ?? "";
    this.settings.knownFileRevisions[path] = detail.revision;
    this.settings.pendingDeletedPaths = this.settings.pendingDeletedPaths.filter((deletedPath) => deletedPath !== path);
    this.localDirty = true;
    await this.saveSettings();
  }

  private async downloadConflictRemote(detail: PendingConflict): Promise<DownloadedFile> {
    if (detail.remoteDeleted) {
      throw new Error("The remote side of this conflict is deleted.");
    }

    return this.downloadRemoteFile({
      type: "download",
      path: detail.path,
      remoteHash: detail.remoteHash,
      revision: detail.revision,
    });
  }

  private async replaceVaultFileWithContent(path: string, content: ArrayBuffer): Promise<void> {
    const existing = this.getVaultFile(path);
    await this.ensureParentFolders(path);
    if (existing) {
      await this.moveFileToSyncTrash(existing);
    }

    await this.app.vault.createBinary(path, content);
    const written = this.getVaultFile(path);
    if (!written) {
      throw new Error(`Resolved file was not written: ${path}.`);
    }
  }

  private async moveFileToConflictCopy(file: TFile): Promise<string> {
    const conflictPath = this.nextConflictCopyPath(file.path);
    await this.ensureParentFolders(conflictPath);
    await this.app.vault.rename(file, conflictPath);
    return conflictPath;
  }

  private nextConflictCopyPath(path: string): string {
    const clientName = slugForConflictName(this.settings.clientName || "client");
    const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
    const basePath = appendPathSuffix(path, `.sync-conflict.${clientName}.${timestamp}`);
    let candidate = basePath;

    while (this.app.vault.getAbstractFileByPath(candidate) !== null) {
      candidate = appendPathSuffix(basePath, `-${randomId().slice(0, 8)}`);
    }

    return candidate;
  }

  private async clearResolvedConflict(path: string): Promise<void> {
    this.settings.pendingConflicts = this.settings.pendingConflicts.filter((conflictPath) => conflictPath !== path);
    delete this.settings.pendingConflictDetails[path];
    await this.saveSettings();
  }

  private async applyCommittedSyncState(
    manifest: LocalManifest,
    plan: SyncPlanResponse,
    serverRevision: number,
  ): Promise<void> {
    const knownFileHashes = { ...this.settings.knownFileHashes };
    const knownFileRevisions = { ...this.settings.knownFileRevisions };

    for (const path of manifest.deletedPaths) {
      delete knownFileHashes[path];
      delete knownFileRevisions[path];
    }

    for (const action of plan.actions) {
      const path = this.normalizedActionPath(action);
      const manifestFile = this.findManifestFile(manifest, path);

      switch (action.type) {
        case "upload":
          if (!manifestFile || manifestFile.deleted) {
            throw new Error(`Upload action did not match a local manifest file for ${path}.`);
          }
          knownFileHashes[path] = action.expectedHash ?? manifestFile.hash;
          knownFileRevisions[path] = serverRevision;
          break;
        case "download":
          if (!action.remoteHash) {
            throw new Error(`Download action did not include a remote hash for ${path}.`);
          }
          knownFileHashes[path] = action.remoteHash;
          knownFileRevisions[path] = action.revision ?? serverRevision;
          break;
        case "delete_local":
        case "delete_remote":
          delete knownFileHashes[path];
          delete knownFileRevisions[path];
          break;
        case "none":
          if (manifestFile && !manifestFile.deleted) {
            knownFileHashes[path] = manifestFile.hash;
            knownFileRevisions[path] = action.revision ?? manifestFile.lastKnownRevision;
          } else {
            delete knownFileHashes[path];
            delete knownFileRevisions[path];
          }
          break;
        case "conflict":
          throw new SyncConflictError([path]);
      }
    }

    this.settings.knownFileHashes = removeEmptyHashEntries(knownFileHashes);
    this.settings.knownFileRevisions = knownFileRevisions;
    this.settings.lastKnownServerRevision = serverRevision;
    this.settings.pendingDeletedPaths = [];
    this.settings.pendingConflicts = [];
    this.localDirty = false;
    await this.saveSettings();
  }

  private async persistPendingConflicts(actions: PlanAction[]): Promise<void> {
    const paths = [...new Set(actions.map((action) => this.normalizedActionPath(action)))].sort();
    const details = { ...this.settings.pendingConflictDetails };
    for (const action of actions) {
      const path = this.normalizedActionPath(action);
      details[path] = {
        path,
        localHash: action.expectedHash,
        remoteHash: action.remoteHash,
        baseHash: action.baseHash,
        revision: action.revision ?? 0,
        localDeleted: action.expectedHash === undefined || action.expectedHash === "",
        remoteDeleted: action.remoteDeleted ?? false,
      };
    }

    this.settings.pendingConflicts = paths;
    this.settings.pendingConflictDetails = details;
    await this.saveSettings();
  }

  private normalizedActionPath(action: PlanAction): string {
    const path = normalizeVaultPath(action.path);
    if (!path) {
      throw new Error("Backend returned an invalid vault path.");
    }
    return path;
  }

  private requireManifestFile(manifest: LocalManifest, path: string): LocalManifestFile {
    const file = this.findManifestFile(manifest, path);
    if (!file) {
      throw new Error(`Sync plan references a local file that is not in the manifest: ${path}.`);
    }
    return file;
  }

  private findManifestFile(manifest: LocalManifest, path: string): LocalManifestFile | null {
    return manifest.files.find((file) => file.path === path) ?? null;
  }

  private async readManifestFileContent(file: LocalManifestFile): Promise<ArrayBuffer> {
    if (file.deleted) {
      throw new Error(`Cannot read deleted manifest file: ${file.path}.`);
    }

    const vaultFile = this.getVaultFile(file.path);
    if (!vaultFile) {
      throw new Error(`Local file disappeared during sync: ${file.path}.`);
    }

    const content = await this.app.vault.readBinary(vaultFile);
    const hash = await sha256Hex(content);
    if (hash !== file.hash) {
      throw new Error(`Local file changed during sync: ${file.path}.`);
    }
    return content;
  }

  private async assertLocalPathStillMatchesManifest(
    path: string,
    manifestFile: LocalManifestFile | null,
  ): Promise<TFile | null> {
    const current = this.app.vault.getAbstractFileByPath(path);
    if (current && !(current instanceof TFile)) {
      throw new Error(`Local path is not a file: ${path}.`);
    }

    if (!manifestFile) {
      if (current) {
        throw new Error(`Local file appeared during sync: ${path}.`);
      }
      return null;
    }

    if (manifestFile.deleted) {
      if (current) {
        throw new Error(`Local file appeared during sync: ${path}.`);
      }
      return null;
    }

    if (!current) {
      throw new Error(`Local file disappeared during sync: ${path}.`);
    }

    const hash = await this.hashVaultFile(current);
    if (hash !== manifestFile.hash) {
      throw new Error(`Local file changed during sync: ${path}.`);
    }

    return current;
  }

  private assertLocalPathStillAbsent(path: string): void {
    if (this.app.vault.getAbstractFileByPath(path) !== null) {
      throw new Error(`Local file appeared during sync: ${path}.`);
    }
  }

  private getVaultFile(path: string): TFile | null {
    const file = this.app.vault.getAbstractFileByPath(path);
    if (!file) {
      return null;
    }
    if (file instanceof TFile) {
      return file;
    }
    throw new Error(`Vault path is not a file: ${path}.`);
  }

  private async hashVaultFile(file: TFile): Promise<string> {
    return sha256Hex(await this.app.vault.readBinary(file));
  }

  private async ensureParentFolders(path: string): Promise<void> {
    const parent = parentPath(path);
    if (!parent) {
      return;
    }

    let current = "";
    for (const part of parent.split("/")) {
      current = current ? `${current}/${part}` : part;
      const existing = this.app.vault.getAbstractFileByPath(current);
      if (existing) {
        if (existing instanceof TFile) {
          throw new Error(`Cannot create folder because a file exists at ${current}.`);
        }
        continue;
      }
      await this.app.vault.createFolder(current);
    }
  }

  private async moveFileToSyncTrash(file: TFile): Promise<void> {
    const trashPath = this.nextSyncTrashPath(file.path);
    await this.ensureParentFolders(trashPath);
    await this.app.vault.rename(file, trashPath);
  }

  private nextSyncTrashPath(path: string): string {
    const timestamp = new Date().toISOString().replace(/[:.]/g, "-");
    const basePath = `${NOX_SYNC_TRASH_ROOT}/${timestamp}/${path}`;
    let candidate = basePath;

    while (this.app.vault.getAbstractFileByPath(candidate) !== null) {
      candidate = appendPathSuffix(basePath, `-${randomId().slice(0, 8)}`);
    }

    return candidate;
  }

  private setButtonState(state: SyncButtonState, detail = ""): void {
    this.buttonState = state;

    if (!this.ribbonButtonEl) {
      return;
    }

    this.ribbonButtonEl.removeClasses(STATE_CLASSES);
    this.ribbonButtonEl.addClass(`nox-sync-state-${state.toLowerCase()}`);
    this.ribbonButtonEl.setAttr("aria-disabled", String(isDisabledState(state)));
    this.ribbonButtonEl.setAttr("aria-label", tooltipForState(state, detail));
    this.ribbonButtonEl.setAttr("title", tooltipForState(state, detail));
    setIcon(this.ribbonButtonEl, iconForState(state));
  }

  private applyBackendStatus(status: BackendStatusResponse): void {
    this.latestBackendStatus = status;

    if (this.settings.pendingConflicts.length > 0) {
      this.setButtonState(SyncButtonState.Conflict);
      return;
    }

    const syncState = status.sync?.state ?? "IDLE";
    if (syncState === "SYNCING") {
      const state =
        status.sync.clientId === this.settings.clientId
          ? SyncButtonState.SyncingLocal
          : SyncButtonState.BlockedRemote;
      this.setButtonState(state, status.sync.clientName);
      return;
    }

    if (syncState === "FAILED") {
      this.setButtonState(SyncButtonState.Error, "The previous NoX Sync sync failed.");
      return;
    }

    if (syncState === "STALE_LOCK") {
      this.setButtonState(SyncButtonState.Error, "A previous sync lock became stale.");
      return;
    }

    const remoteDirty = status.serverRevision > this.settings.lastKnownServerRevision;
    const localDirty = this.hasLocalPendingState();

    if (localDirty && remoteDirty) {
      this.setButtonState(SyncButtonState.BothDirty);
    } else if (localDirty) {
      this.setButtonState(SyncButtonState.LocalDirty);
    } else if (remoteDirty) {
      this.setButtonState(SyncButtonState.RemoteDirty);
    } else {
      this.setButtonState(SyncButtonState.Synced);
    }
  }

  private hasLocalPendingState(): boolean {
    return this.localDirty || this.settings.pendingDeletedPaths.length > 0;
  }

  private async createLocalManifest(): Promise<LocalManifest> {
    const files: LocalManifestFile[] = [];
    const seenPaths = new Set<string>();

    for (const file of this.app.vault.getFiles()) {
      const normalizedPath = normalizeVaultPath(file.path);
      if (!normalizedPath || this.shouldExcludePath(normalizedPath)) {
        continue;
      }

      const content = await this.app.vault.readBinary(file);
      const hash = await sha256Hex(content);
      const size = file.stat?.size ?? content.byteLength;
      const lastKnownRevision = this.settings.knownFileRevisions[normalizedPath] ?? 0;

      seenPaths.add(normalizedPath);
      files.push({
        path: normalizedPath,
        hash,
        size,
        lastKnownRevision,
        deleted: false,
      });
    }

    const deletedPaths = Object.keys(this.settings.knownFileHashes)
      .map((path) => normalizeVaultPath(path))
      .filter((path): path is string => path !== null && !this.shouldExcludePath(path) && !seenPaths.has(path))
      .sort();

    for (const path of deletedPaths) {
      files.push({
        path,
        hash: this.settings.knownFileHashes[path] ?? "",
        size: 0,
        lastKnownRevision: this.settings.knownFileRevisions[path] ?? 0,
        deleted: true,
      });
    }

    files.sort((a, b) => a.path.localeCompare(b.path));

    return {
      generatedAt: new Date().toISOString(),
      files,
      deletedPaths,
    };
  }

  private manifestHasLocalChanges(manifest: LocalManifest): boolean {
    if (manifest.deletedPaths.length > 0) {
      return true;
    }

    return manifest.files.some((file) => this.settings.knownFileHashes[file.path] !== file.hash);
  }

  private async persistPendingDeletedPaths(deletedPaths: string[]): Promise<void> {
    const normalized = [...new Set(deletedPaths)].sort();
    if (arraysEqual(this.settings.pendingDeletedPaths, normalized)) {
      return;
    }

    this.settings.pendingDeletedPaths = normalized;
    await this.saveSettings();
  }

  private handleVaultChanged(file: TAbstractFile): void {
    const normalizedPath = normalizeVaultPath(file.path);
    if (!normalizedPath || this.shouldExcludePath(normalizedPath)) {
      return;
    }

    this.localDirty = true;
    this.applyLatestStatus();
    this.scheduleManifestScan();
  }

  private async handleVaultDeleted(file: TAbstractFile): Promise<void> {
    const normalizedPath = normalizeVaultPath(file.path);
    if (!normalizedPath || this.shouldExcludePath(normalizedPath)) {
      return;
    }

    if (this.settings.knownFileHashes[normalizedPath]) {
      const deletedPaths = [...new Set([...this.settings.pendingDeletedPaths, normalizedPath])].sort();
      this.settings.pendingDeletedPaths = deletedPaths;
      await this.saveSettings();
    }

    this.localDirty = true;
    this.applyLatestStatus();
    this.scheduleManifestScan();
  }

  private async handleVaultRenamed(file: TAbstractFile, oldPath: string): Promise<void> {
    await this.handleVaultDeleted({ path: oldPath } as TAbstractFile);
    this.handleVaultChanged(file);
  }

  private scheduleManifestScan(): void {
    this.clearManifestScanTimer();
    this.manifestScanTimer = window.setTimeout(() => {
      this.manifestScanTimer = null;
      void this.refreshLocalManifestState();
    }, 1500);
  }

  private clearManifestScanTimer(): void {
    if (this.manifestScanTimer !== null) {
      window.clearTimeout(this.manifestScanTimer);
      this.manifestScanTimer = null;
    }
  }

  private applyLatestStatus(): void {
    if (this.latestBackendStatus) {
      this.applyBackendStatus(this.latestBackendStatus);
    } else if (this.localDirty) {
      this.setButtonState(SyncButtonState.LocalDirty);
    }
  }

  private shouldExcludePath(path: string): boolean {
    if (isPluginInternalPath(path) || isNoxSyncTrashPath(path)) {
      return true;
    }

    return !this.settings.syncHiddenFiles && isHiddenPath(path);
  }

  private connectStatusStream(): void {
    this.closeStatusStream();
    this.clearReconnectTimer();

    if (!this.hasCredentials() || typeof EventSource === "undefined") {
      return;
    }

    const url = `${this.normalizedServerUrl()}/v1/sync/events?api_key=${encodeURIComponent(
      this.settings.apiKey.trim(),
    )}`;
    const events = new EventSource(url);
    this.statusEvents = events;

    events.addEventListener("status", (event: MessageEvent<string>) => {
      try {
        this.applyBackendStatus(JSON.parse(event.data) as BackendStatusResponse);
      } catch {
        this.setButtonState(SyncButtonState.Error, "NoX Sync received an invalid status update.");
      }
    });

    events.onerror = () => {
      this.closeStatusStream();
      void this.refreshBackendStatus();
      this.scheduleStatusReconnect();
    };
  }

  private closeStatusStream(): void {
    if (this.statusEvents) {
      this.statusEvents.close();
      this.statusEvents = null;
    }
  }

  private scheduleStatusReconnect(): void {
    this.clearReconnectTimer();
    if (!this.hasCredentials()) {
      return;
    }

    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.connectStatusStream();
    }, 5000);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private clearSettingsRefreshTimer(): void {
    if (this.settingsRefreshTimer !== null) {
      window.clearTimeout(this.settingsRefreshTimer);
      this.settingsRefreshTimer = null;
    }
  }

  private hasCredentials(): boolean {
    return this.settings.serverUrl.trim() !== "" && this.settings.apiKey.trim().startsWith("noxsync_");
  }

  private normalizedServerUrl(): string {
    return this.settings.serverUrl.trim().replace(/\/+$/, "");
  }

  private authHeaders(): Record<string, string> {
    return {
      Authorization: `Bearer ${this.settings.apiKey.trim()}`,
    };
  }
}

class ConflictResolutionModal extends Modal {
  private selectedPath: string | null = null;

  constructor(
    app: App,
    private readonly plugin: NoxSyncPlugin,
  ) {
    super(app);
  }

  onOpen(): void {
    this.selectedPath = this.plugin.settings.pendingConflicts[0] ?? null;
    this.render();
  }

  onClose(): void {
    this.contentEl.empty();
  }

  private render(): void {
    const { contentEl } = this;
    contentEl.empty();
    contentEl.addClass("nox-sync-conflict-modal");

    contentEl.createEl("h2", { text: "NoX Sync Conflicts" });

    const conflicts = this.plugin.settings.pendingConflicts;
    if (conflicts.length === 0) {
      contentEl.createEl("p", { text: "No conflicts are pending." });
      return;
    }

    if (!this.selectedPath || !conflicts.includes(this.selectedPath)) {
      this.selectedPath = conflicts[0];
    }

    const layout = contentEl.createEl("div", { cls: "nox-sync-conflict-layout" });
    const list = layout.createEl("div", { cls: "nox-sync-conflict-list" });
    const detail = layout.createEl("div", { cls: "nox-sync-conflict-detail" });

    for (const path of conflicts) {
      const item = list.createEl("button", {
        text: path,
        cls: path === this.selectedPath ? "nox-sync-conflict-list-item is-active" : "nox-sync-conflict-list-item",
      });
      item.type = "button";
      item.onclick = () => {
        this.selectedPath = path;
        this.render();
      };
    }

    void this.renderDetail(detail, this.selectedPath);
  }

  private async renderDetail(container: HTMLElement, path: string): Promise<void> {
    container.empty();
    container.createEl("p", { text: "Loading conflict details..." });

    try {
      const preview = await this.plugin.getConflictPreview(path);
      if (this.selectedPath !== path) {
        return;
      }

      container.empty();
      container.createEl("h3", { text: path });

      const meta = container.createEl("div", { cls: "nox-sync-conflict-meta" });
      meta.createEl("span", {
        text: preview.detail.localDeleted ? "Local: deleted" : `Local: ${shortHash(preview.localHash)}`,
      });
      meta.createEl("span", {
        text: preview.detail.remoteDeleted ? "Remote: deleted" : `Remote: ${shortHash(preview.remoteHash)}`,
      });

      let mergeInput: HTMLTextAreaElement | null = null;
      if (preview.isMarkdown) {
        const columns = container.createEl("div", { cls: "nox-sync-conflict-columns" });
        this.renderReadonlyText(columns, "Local version", preview.localText ?? "(deleted)");
        this.renderReadonlyText(columns, "Remote version", preview.remoteText ?? "(deleted)");

        if (!preview.detail.localDeleted && !preview.detail.remoteDeleted) {
          const mergeWrap = container.createEl("div", { cls: "nox-sync-conflict-merge" });
          mergeWrap.createEl("h4", { text: "Manual merge" });
          mergeInput = mergeWrap.createEl("textarea", { cls: "nox-sync-conflict-textarea" });
          mergeInput.value = preview.localText ?? "";
        }
      } else {
        container.createEl("p", {
          text: "This is a binary or non-Markdown conflict. No text merge is available.",
          cls: "nox-sync-conflict-note",
        });
      }

      const actions = container.createEl("div", { cls: "nox-sync-conflict-actions" });
      this.renderActionButton(actions, "Keep local", path, "keep_local");
      this.renderActionButton(actions, "Keep remote", path, "keep_remote");
      this.renderActionButton(actions, "Keep both", path, "keep_both");
      if (mergeInput) {
        this.renderActionButton(actions, "Save manual merge", path, "manual_merge", () => mergeInput?.value ?? "");
      }
    } catch (error) {
      container.empty();
      const message = error instanceof Error ? error.message : "Failed to load conflict details.";
      container.createEl("p", { text: message, cls: "nox-sync-conflict-error" });
    }
  }

  private renderReadonlyText(container: HTMLElement, label: string, value: string): void {
    const wrap = container.createEl("div", { cls: "nox-sync-conflict-version" });
    wrap.createEl("h4", { text: label });
    const textarea = wrap.createEl("textarea", { cls: "nox-sync-conflict-textarea" });
    textarea.readOnly = true;
    textarea.value = value;
  }

  private renderActionButton(
    container: HTMLElement,
    label: string,
    path: string,
    choice: ConflictResolutionChoice,
    value?: () => string,
  ): void {
    const button = container.createEl("button", { text: label });
    button.type = "button";
    button.onclick = () => {
      void this.resolve(path, choice, value?.() ?? "");
    };
  }

  private async resolve(path: string, choice: ConflictResolutionChoice, mergedText: string): Promise<void> {
    try {
      const message = await this.plugin.resolvePendingConflict(path, choice, mergedText);
      new Notice(`NoX Sync: ${message}`);

      if (this.plugin.settings.pendingConflicts.length === 0) {
        this.close();
        return;
      }

      this.selectedPath = this.plugin.settings.pendingConflicts[0];
      this.render();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Conflict resolution failed.";
      new Notice(`NoX Sync: ${message}`);
    }
  }
}

class NoxSyncSettingTab extends PluginSettingTab {
  plugin: NoxSyncPlugin;

  constructor(app: App, plugin: NoxSyncPlugin) {
    super(app, plugin);
    this.plugin = plugin;
  }

  display(): void {
    const { containerEl } = this;
    containerEl.empty();

    containerEl.createEl("h2", { text: "NoX Sync" });

    new Setting(containerEl)
      .setName("Server URL")
      .setDesc("Backend URL, for example http://localhost:8080.")
      .addText((text) =>
        text
          .setPlaceholder("http://localhost:8080")
          .setValue(this.plugin.settings.serverUrl)
          .onChange(async (value) => {
            this.plugin.settings.serverUrl = value.trim();
            await this.plugin.saveSettings();
            this.plugin.queueSettingsRefresh();
          }),
      );

    new Setting(containerEl)
      .setName("API key")
      .setDesc("Reusable noxsync_ API key from the backend admin page.")
      .addText((text) => {
        text.inputEl.type = "password";
        text
          .setPlaceholder("noxsync_...")
          .setValue(this.plugin.settings.apiKey)
          .onChange(async (value) => {
            this.plugin.settings.apiKey = value.trim();
            await this.plugin.saveSettings();
            this.plugin.queueSettingsRefresh();
          });
      });

    new Setting(containerEl)
      .setName("Client name")
      .setDesc("Readable name shown when this device owns the sync lock.")
      .addText((text) =>
        text
          .setPlaceholder("Windows PC")
          .setValue(this.plugin.settings.clientName)
          .onChange(async (value) => {
            this.plugin.settings.clientName = value.trim();
            await this.plugin.saveSettings();
          }),
      );

    new Setting(containerEl)
      .setName("Vault ID")
      .setDesc("Local vault identity used for sync tracking.")
      .addText((text) =>
        text
          .setPlaceholder("vault_...")
          .setValue(this.plugin.settings.vaultId)
          .onChange(async (value) => {
            this.plugin.settings.vaultId = value.trim();
            await this.plugin.saveSettings();
          }),
      );

    new Setting(containerEl)
      .setName("Sync hidden files")
      .setDesc("Includes dot-prefixed vault files, while still excluding NoX Sync plugin data.")
      .addToggle((toggle) =>
        toggle.setValue(this.plugin.settings.syncHiddenFiles).onChange(async (value) => {
          this.plugin.settings.syncHiddenFiles = value;
          await this.plugin.saveSettings();
          this.plugin.queueSettingsRefresh();
        }),
      );

    new Setting(containerEl)
      .setName("Test connection")
      .setDesc("Checks that the backend is reachable and the API key is valid.")
      .addButton((button) =>
        button
          .setButtonText("Test connection")
          .setCta()
          .onClick(() => {
            void this.plugin.testConnection();
          }),
      );
  }
}

function tooltipForState(state: SyncButtonState, detail = ""): string {
  switch (state) {
    case SyncButtonState.Unknown:
      return "Checking NoX Sync status...";
    case SyncButtonState.Synced:
      return "Vault synced.";
    case SyncButtonState.LocalDirty:
      return "Local changes pending sync.";
    case SyncButtonState.RemoteDirty:
      return "Remote changes available. Click to sync.";
    case SyncButtonState.BothDirty:
      return "Local and remote changes detected. Sync may require conflict resolution.";
    case SyncButtonState.SyncingLocal:
      return "Syncing vault...";
    case SyncButtonState.BlockedRemote:
      return detail ? `Sync in progress on ${detail}.` : "Sync in progress on another device.";
    case SyncButtonState.ServerUnreachable:
      return "NoX Sync server unavailable.";
    case SyncButtonState.Conflict:
      return "Conflicts pending.";
    case SyncButtonState.Error:
      return detail || "The previous NoX Sync sync failed.";
    case SyncButtonState.AuthFailed:
      return "Authentication failed. Check NoX Sync settings.";
  }
}

function iconForState(state: SyncButtonState): string {
  switch (state) {
    case SyncButtonState.Unknown:
      return "refresh-cw";
    case SyncButtonState.Synced:
      return "check-circle";
    case SyncButtonState.LocalDirty:
      return "upload-cloud";
    case SyncButtonState.RemoteDirty:
      return "download-cloud";
    case SyncButtonState.BothDirty:
      return "alert-triangle";
    case SyncButtonState.SyncingLocal:
      return "refresh-cw";
    case SyncButtonState.BlockedRemote:
      return "lock";
    case SyncButtonState.ServerUnreachable:
      return "wifi-off";
    case SyncButtonState.Conflict:
      return "alert-octagon";
    case SyncButtonState.Error:
      return "x-circle";
    case SyncButtonState.AuthFailed:
      return "key-round";
  }
}

function isDisabledState(state: SyncButtonState): boolean {
  return [
    SyncButtonState.Unknown,
    SyncButtonState.SyncingLocal,
    SyncButtonState.BlockedRemote,
    SyncButtonState.AuthFailed,
  ].includes(state);
}

function randomId(): string {
  const webCrypto = globalThis.crypto;

  if (webCrypto) {
    if (typeof webCrypto.randomUUID === "function") {
      return webCrypto.randomUUID().replaceAll("-", "");
    }

    const bytes = new Uint8Array(16);
    webCrypto.getRandomValues(bytes);
    return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
  }

  return `${Date.now().toString(16)}${Math.random().toString(16).slice(2)}`;
}

function normalizeVaultPath(rawPath: string): string | null {
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

function normalizeRequiredPath(rawPath: string): string {
  const path = normalizeVaultPath(rawPath);
  if (!path) {
    throw new Error("Invalid conflict path.");
  }
  return path;
}

function isMarkdownPath(path: string): boolean {
  const lowerPath = path.toLowerCase();
  return lowerPath.endsWith(".md") || lowerPath.endsWith(".markdown");
}

function isPluginInternalPath(path: string): boolean {
  return path === ".obsidian/plugins/nox-sync" || path.startsWith(".obsidian/plugins/nox-sync/");
}

function isNoxSyncTrashPath(path: string): boolean {
  return path === NOX_SYNC_TRASH_ROOT || path.startsWith(`${NOX_SYNC_TRASH_ROOT}/`);
}

function isHiddenPath(path: string): boolean {
  return path.split("/").some((part) => part.startsWith("."));
}

async function sha256Hex(content: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", content);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function textFromArrayBuffer(content: ArrayBuffer): string {
  return new TextDecoder("utf-8").decode(content);
}

function arrayBufferFromText(value: string): ArrayBuffer {
  const bytes = new TextEncoder().encode(value);
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
}

function slugForConflictName(value: string): string {
  const slug = value
    .trim()
    .replace(/[^A-Za-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return slug || "client";
}

function shortHash(value: string): string {
  return value ? value.slice(0, 12) : "unknown";
}

function ensureResponseOK(response: { status: number; json: unknown }): void {
  if (response.status >= 200 && response.status < 300) {
    return;
  }

  const backendError = isBackendErrorResponse(response.json) ? response.json : {};
  throw new BackendRequestError(
    response.status,
    backendError.code ?? "HTTP_ERROR",
    backendError.message ?? `NoX Sync backend returned HTTP ${response.status}.`,
  );
}

function isBackendErrorResponse(value: unknown): value is BackendErrorResponse {
  return typeof value === "object" && value !== null;
}

function responseHeader(response: { headers?: Record<string, string> }, name: string): string {
  const headers = response.headers ?? {};
  const lowerName = name.toLowerCase();
  for (const [key, value] of Object.entries(headers)) {
    if (key.toLowerCase() === lowerName) {
      return value;
    }
  }
  return "";
}

function numberFromHeader(value: string): number | null {
  if (!value) {
    return null;
  }

  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function removeEmptyHashEntries(hashes: Record<string, string>): Record<string, string> {
  return Object.fromEntries(Object.entries(hashes).filter(([, hash]) => hash !== ""));
}

function parentPath(path: string): string {
  const index = path.lastIndexOf("/");
  return index === -1 ? "" : path.slice(0, index);
}

function appendPathSuffix(path: string, suffix: string): string {
  const slashIndex = path.lastIndexOf("/");
  const directory = slashIndex === -1 ? "" : path.slice(0, slashIndex + 1);
  const filename = slashIndex === -1 ? path : path.slice(slashIndex + 1);
  const dotIndex = filename.lastIndexOf(".");

  if (dotIndex <= 0) {
    return `${directory}${filename}${suffix}`;
  }

  return `${directory}${filename.slice(0, dotIndex)}${suffix}${filename.slice(dotIndex)}`;
}

function arraysEqual(left: string[], right: string[]): boolean {
  if (left.length !== right.length) {
    return false;
  }

  return left.every((value, index) => value === right[index]);
}

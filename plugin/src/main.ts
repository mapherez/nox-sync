import { App, Notice, Plugin, PluginSettingTab, Setting, requestUrl, setIcon } from "obsidian";

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
};

const STATE_CLASSES = Object.values(SyncButtonState).map(
  (state) => `nox-sync-state-${state.toLowerCase()}`,
);

export default class NoxSyncPlugin extends Plugin {
  settings: NoxSyncSettings = { ...DEFAULT_SETTINGS };
  private ribbonButtonEl: HTMLElement | null = null;
  private buttonState = SyncButtonState.Unknown;
  private statusEvents: EventSource | null = null;
  private reconnectTimer: number | null = null;
  private settingsRefreshTimer: number | null = null;

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
    this.setButtonState(SyncButtonState.Unknown);
    await this.refreshBackendStatus();
    this.connectStatusStream();
  }

  onunload(): void {
    this.closeStatusStream();
    this.clearReconnectTimer();
    this.clearSettingsRefreshTimer();
  }

  async loadSettings(): Promise<void> {
    this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
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
      void this.refreshBackendStatus().then((ok) => {
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

  private async handleManualSync(): Promise<void> {
    switch (this.buttonState) {
      case SyncButtonState.Synced:
        await this.refreshBackendStatus();
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
        new Notice("NoX Sync conflicts need to be resolved.");
        return;

      case SyncButtonState.Unknown:
        await this.refreshBackendStatus();
        return;

      case SyncButtonState.LocalDirty:
      case SyncButtonState.RemoteDirty:
      case SyncButtonState.BothDirty:
        await this.refreshBackendStatus();
        if (
          this.buttonState === SyncButtonState.LocalDirty ||
          this.buttonState === SyncButtonState.RemoteDirty ||
          this.buttonState === SyncButtonState.BothDirty
        ) {
          new Notice("NoX Sync manual sync flow will be implemented in a later milestone.");
        }
        return;

      case SyncButtonState.Error:
        await this.refreshBackendStatus();
        if (this.buttonState === SyncButtonState.Error) {
          new Notice("The previous NoX Sync sync failed.");
        }
        return;
    }
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
    return this.settings.pendingDeletedPaths.length > 0;
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

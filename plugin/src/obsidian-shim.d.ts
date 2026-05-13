declare module "obsidian" {
  export class App {
    vault: Vault;
  }

  export class Plugin {
    app: App;
    loadData(): Promise<unknown>;
    saveData(data: unknown): Promise<void>;
    addRibbonIcon(icon: string, title: string, callback: (evt: MouseEvent) => unknown): HTMLElement;
    addCommand(command: { id: string; name: string; callback: () => unknown }): void;
    addSettingTab(tab: PluginSettingTab): void;
    registerEvent(eventRef: EventRef): void;
  }

  export class TAbstractFile {
    path: string;
  }

  export class TFile extends TAbstractFile {
    stat: {
      ctime: number;
      mtime: number;
      size: number;
    };
  }

  export interface EventRef {}

  export interface Vault {
    getName(): string;
    getFiles(): TFile[];
    getAbstractFileByPath(path: string): TAbstractFile | null;
    readBinary(file: TFile): Promise<ArrayBuffer>;
    createBinary(path: string, data: ArrayBuffer): Promise<TFile>;
    createFolder(path: string): Promise<void>;
    rename(file: TAbstractFile, newPath: string): Promise<void>;
    on(name: "create", callback: (file: TAbstractFile) => unknown): EventRef;
    on(name: "modify", callback: (file: TFile) => unknown): EventRef;
    on(name: "delete", callback: (file: TAbstractFile) => unknown): EventRef;
    on(name: "rename", callback: (file: TAbstractFile, oldPath: string) => unknown): EventRef;
  }

  export class PluginSettingTab {
    app: App;
    plugin: Plugin;
    containerEl: HTMLElement;
    constructor(app: App, plugin: Plugin);
    display(): void;
  }

  export class Modal {
    app: App;
    contentEl: HTMLElement;
    constructor(app: App);
    open(): void;
    close(): void;
    onOpen(): void;
    onClose(): void;
  }

  export class Setting {
    constructor(containerEl: HTMLElement);
    setName(name: string): this;
    setDesc(desc: string): this;
    addText(callback: (component: TextComponent) => unknown): this;
    addToggle(callback: (component: ToggleComponent) => unknown): this;
    addButton(callback: (component: ButtonComponent) => unknown): this;
  }

  export class TextComponent {
    inputEl: HTMLInputElement;
    setPlaceholder(placeholder: string): this;
    setValue(value: string): this;
    onChange(callback: (value: string) => unknown): this;
  }

  export class ButtonComponent {
    setButtonText(text: string): this;
    setCta(): this;
    onClick(callback: () => unknown): this;
  }

  export class ToggleComponent {
    setValue(value: boolean): this;
    onChange(callback: (value: boolean) => unknown): this;
  }

  export class Notice {
    constructor(message: string, timeout?: number);
  }

  export function setIcon(parent: HTMLElement, iconId: string): void;

  export interface RequestUrlParam {
    url: string;
    method?: string;
    headers?: Record<string, string>;
    body?: string | ArrayBuffer;
  }

  export interface RequestUrlResponse {
    status: number;
    json: unknown;
    text: string;
    arrayBuffer: ArrayBuffer;
    headers: Record<string, string>;
  }

  export function requestUrl(params: RequestUrlParam): Promise<RequestUrlResponse>;
}

interface HTMLElement {
  addClass(className: string): void;
  removeClasses(classNames: string[]): void;
  setAttr(name: string, value: string): void;
  empty(): void;
  createEl<K extends keyof HTMLElementTagNameMap>(
    tagName: K,
    options?: { text?: string; cls?: string },
  ): HTMLElementTagNameMap[K];
}

declare module "obsidian" {
  export class App {
    vault: {
      getName(): string;
    };
  }

  export class Plugin {
    app: App;
    loadData(): Promise<unknown>;
    saveData(data: unknown): Promise<void>;
    addRibbonIcon(icon: string, title: string, callback: (evt: MouseEvent) => unknown): HTMLElement;
    addCommand(command: { id: string; name: string; callback: () => unknown }): void;
    addSettingTab(tab: PluginSettingTab): void;
  }

  export class PluginSettingTab {
    app: App;
    plugin: Plugin;
    containerEl: HTMLElement;
    constructor(app: App, plugin: Plugin);
    display(): void;
  }

  export class Setting {
    constructor(containerEl: HTMLElement);
    setName(name: string): this;
    setDesc(desc: string): this;
    addText(callback: (component: TextComponent) => unknown): this;
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

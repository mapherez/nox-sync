# NoX Sync - an [Obsidian](https://obsidian.md/) plugin

[![CI](https://github.com/mapherez/nox-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/ci.yml)
[![CodeQL](https://github.com/mapherez/nox-sync/actions/workflows/codeql.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/mapherez/nox-sync/badge)](https://securityscorecards.dev/viewer/?uri=github.com/mapherez/nox-sync)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

Made with ❤️ by [Mapherez](https://github.com/mapherez) - If you enjoy NoX Sync, please consider [buying me a beer 🍺](https://buymeacoffee.com/mapherez)

## What is NoX Sync?

NoX Sync is an Obsidian plugin for manually synchronizing vaults with [NoX Backend](https://github.com/mapherez/nox-backend), an independent server project. This repository contains only the plugin and its development, tests, release configuration, and documentation.

The plugin is one client of NoX Backend. It connects through the backend's HTTP API using a **Server URL** and **API key** that you configure in Obsidian. A running NoX Backend instance is an external runtime dependency; it is not built or distributed by this repository.

For server installation, hosting, administration, backups, or API documentation, see the [NoX Backend repository](https://github.com/mapherez/nox-backend).

Synchronization is explicitly triggered by the user rather than running continuously in the background.

## Features

- Manual sync from the Obsidian ribbon button or `NoX Sync: Sync vault` command.
- Server URL, API key, and Client name settings with a **Test connection** action.
- Remote vault management from plugin settings: create, select, delete, restore, permanently delete, and display cloud size.
- Manifest-based synchronization with SHA-256 validation for uploads and downloads.
- Explicit Markdown and binary conflict handling.
- Safe local replacement and delete behavior through `.nox-sync-trash/`.
- Local trash size display and clear-trash action.
- Optional synchronization of hidden vault files.
- Plugin-local settings, credentials, sync state, and trash excluded from sync.
- Settings shortcut from the ribbon when no remote vault is selected.

## Install From Release

Download these three assets from a [NoX Sync plugin release](https://github.com/mapherez/nox-sync/releases):

```text
main.js
manifest.json
styles.css
```

Create this folder inside each Obsidian vault where you want to use NoX Sync:

```text
<vault>/.obsidian/plugins/nox-sync/
```

Put the three files in that folder, then enable NoX Sync from Obsidian's Community Plugins settings. The GitHub source code zip is not the installable plugin package.

## Connect To NoX Backend

You need a reachable NoX Backend instance and your API key. Obtain the **Server URL** and **API key** from the backend dashboard or your server administrator. If you need to set up a server first, follow the [NoX Backend documentation](https://github.com/mapherez/nox-backend).

In Obsidian:

1. Open NoX Sync settings.
2. Enter the **Server URL**, for example `https://sync.example.com`. Use the server's base URL without adding `/v1` or `/vault-dashboard`.
3. Paste your **API key**.
4. Set a readable **Client name**, such as `Laptop` or `Desktop`.
5. Click **Test connection**.
6. In **Backend vault**, create or select the remote vault for this local vault.
7. Use the ribbon button or `NoX Sync: Sync vault` command to sync manually.

To sync another device, install the plugin there and select the same remote vault using the same user's credentials. Each device must perform its own manual sync. If your API key changes, update it on each device.

You can assign a shortcut to `NoX Sync: Sync vault` in Obsidian's Hotkeys settings. For the full first-run flow, see the [User setup guide](docs/user-setup.md).

## Build From Source

The plugin source, tests, dependencies, and build configuration live in `plugin/`. CI uses Node.js 22.

```bash
cd plugin
npm ci
npm run typecheck
npm run test
npm run build
```

The installable files are written to `plugin/dist/`: `main.js`, `manifest.json`, and `styles.css`. Copy those files into your vault's `.obsidian/plugins/nox-sync/` folder to test the plugin in Obsidian.

The build and unit tests do not require a running backend. Testing actual synchronization in Obsidian requires access to a separately managed NoX Backend instance.

## Plugin Releases And Versions

The release workflow builds and checks the plugin, validates version compatibility metadata, generates artifact attestations, and creates a draft GitHub release containing `main.js`, `manifest.json`, and `styles.css`.

The release tag matches the version in `plugin/manifest.json`. To update the plugin version everywhere, run from `plugin/`:

```bash
npm run version:set
```

The script shows the current version and asks for the new `MAJOR.MINOR.PATCH` version. Press Enter to cancel, or pass a version directly with `npm run version:set -- 1.0.2`. It updates `plugin/package.json`, both root version entries in `plugin/package-lock.json`, the root and plugin `manifest.json` files, and both `versions.json` files. Previous compatibility entries and `minAppVersion` are preserved.

Run `npm run build` afterwards to regenerate the release files in `plugin/dist/`. The root manifest and versions files are plugin metadata and remain part of this repository.

## Documentation

- [User setup guide](docs/user-setup.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Security policy](SECURITY.md)
- [NoX Backend: server setup, operation, and API documentation](https://github.com/mapherez/nox-backend)

## Security And Quality

GitHub Actions runs plugin type checks, unit tests, and builds. CodeQL scans JavaScript/TypeScript, Dependabot tracks plugin npm dependencies and GitHub Actions, and OpenSSF Scorecard checks repository and supply-chain security.

Report plugin vulnerabilities privately as described in the [Security policy](SECURITY.md). Report backend vulnerabilities through the [NoX Backend project](https://github.com/mapherez/nox-backend).

## Safety Model

The plugin relies on NoX Backend for remote vault ownership, synchronization locks, sessions, and commits. Files are not considered synced until content hashes are verified and the backend commit succeeds. Conflicts are explicit, and normal sync does not silently overwrite conflicting changes.

Local files replaced or deleted during sync are preserved in `.nox-sync-trash/`. This folder is excluded from sync; clearing it permanently removes those local safety copies.

## Obsidian Policy Disclosures

- Account requirement: Obtain an API key for a NoX Backend instance. Account setup is managed by that independent server; the plugin authenticates with the API key.
- Network use: The plugin sends vault manifests, file content, sync status requests, and vault management requests to the Server URL configured by the user. The optional support link opens Buy Me a Coffee only when clicked.
- Payment: No payment is required for the plugin. The Buy Me a Coffee link is optional.
- Telemetry: The plugin does not include client-side telemetry or analytics.
- Ads: The plugin does not load dynamic ads. The settings page includes an optional static support link.
- File access: The plugin uses Obsidian's vault APIs for files inside the currently opened vault, including `.nox-sync-trash/`. It does not access files outside the vault.
- Updates: The plugin does not include a self-update mechanism. Updates are installed through Obsidian/GitHub release files.

## License

NoX Sync is released under the [GNU General Public License v3.0](LICENSE). You can use, copy, modify, and redistribute it, but distributed modified versions must remain open-source under the GPL. The software is provided without warranty.

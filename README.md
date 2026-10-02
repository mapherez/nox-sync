# NoX Sync

[![CI](https://github.com/mapherez/nox-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/ci.yml)
[![CodeQL](https://github.com/mapherez/nox-sync/actions/workflows/codeql.yml/badge.svg)](https://github.com/mapherez/nox-sync/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/mapherez/nox-sync/badge)](https://securityscorecards.dev/viewer/?uri=github.com/mapherez/nox-sync)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

A manual, self-hosted synchronization plugin for [Obsidian](https://obsidian.md/).

> [!IMPORTANT]
> **NoX Sync requires [NoX Backend](https://github.com/mapherez/nox-backend).**
>
> This repository contains the Obsidian plugin only. Installing the plugin by itself is not enough: you must also have access to a running NoX Backend instance.
>
> NoX Backend is a separate self-hosted service responsible for remote storage, authentication, vault management, synchronization state and the API used by the plugin.

## How it works

NoX Sync connects Obsidian to a NoX Backend server using:

- a **Server URL**;
- an **API key**;
- a **Client name** identifying the current device.

Synchronization is triggered manually from Obsidian. The plugin compares the local vault with its remote vault, plans the required changes and transfers files through the backend API.

The plugin and backend are developed and released independently:

- **NoX Sync** — Obsidian client;
- **[NoX Backend](https://github.com/mapherez/nox-backend)** — self-hosted server.

## Features

- Manual synchronization from the ribbon or command palette.
- Multiple devices connected to the same remote vault.
- Remote vault creation and selection from plugin settings.
- Upload, download and delete synchronization.
- SHA-256 content verification.
- Markdown and binary conflict handling.
- Backend-enforced synchronization locks.
- Safe local file replacement and deletion through `.nox-sync-trash/`.
- Local trash size display and manual cleanup.
- Optional synchronization of hidden vault files.
- Connection testing directly from the settings page.
- No client-side telemetry or analytics.

## Requirements

You need:

- Obsidian `1.13.0` or newer;
- a running [NoX Backend](https://github.com/mapherez/nox-backend) instance;
- a Server URL for that instance;
- a valid API key.

If you do not already have a backend, follow the setup instructions in the [NoX Backend repository](https://github.com/mapherez/nox-backend).

## Installation

Download these three files from the latest [NoX Sync release](https://github.com/mapherez/nox-sync/releases):

```text
main.js
manifest.json
styles.css
```

Create the plugin directory inside your vault:

```text
<vault>/.obsidian/plugins/nox-sync/
```

Copy the three files into that directory, then enable **NoX Sync** from Obsidian's Community Plugins settings.

The GitHub source archive is not the installable plugin package.

## Configuration

Open **Settings → NoX Sync** in Obsidian.

Configure:

### Server URL

Enter the base URL of your NoX Backend instance:

```text
https://sync.example.com
```

Do not append `/v1`, `/vault-dashboard` or another API path.

### API key

Copy your API key from the NoX Backend dashboard and paste it into the plugin settings.

### Client name

Give the current device a readable name, for example:

```text
Desktop
Laptop
Phone
```

Then click **Test connection**.

Once connected, create or select a remote vault.

## Synchronizing

Use either:

- the NoX Sync ribbon button; or
- the `NoX Sync: Sync vault` command from the command palette.

You can assign your own keyboard shortcut from Obsidian's Hotkeys settings.

Synchronization is intentionally manual. NoX Sync does not continuously monitor or synchronize your vault in the background.

## Multiple devices

To synchronize the same vault across several devices:

1. Install NoX Sync on each device.
2. Configure the same NoX Backend instance.
3. Authenticate with the appropriate API key.
4. Select the same remote vault.
5. Synchronize manually on each device.

Each device keeps its own local state and Client name.

## Conflicts and local safety

NoX Sync does not silently overwrite conflicting changes.

When files must be replaced or removed locally during synchronization, recoverable copies are placed in:

```text
.nox-sync-trash/
```

This directory is excluded from synchronization.

The trash can be inspected or cleared from the plugin settings.

## NoX Backend

NoX Sync does not contain or distribute its server.

Server installation, Docker configuration, authentication, storage, backups, API documentation and administration belong to the separate project:

**[github.com/mapherez/nox-backend](https://github.com/mapherez/nox-backend)**

The two projects communicate only through the backend HTTP API.

## Development

The plugin source lives in `plugin/`.

```bash
cd plugin
npm ci
npm run typecheck
npm run test
npm run build
```

Production assets are generated in:

```text
plugin/dist/
```

containing:

```text
main.js
manifest.json
styles.css
```

The build and unit tests do not require a running backend.

Testing real synchronization requires access to a NoX Backend instance.

## Versions and releases

Plugin version metadata is maintained in the package, manifests and `versions.json` files.

To update the plugin version:

```bash
cd plugin
npm run version:set
```

Or provide the version directly:

```bash
npm run version:set -- 1.0.2
```

After changing the version:

```bash
npm run build
```

The release workflow validates the plugin, builds the production assets and creates a draft GitHub release containing:

```text
main.js
manifest.json
styles.css
```

NoX Sync and NoX Backend have independent release cycles and version numbers.

## Documentation

- [User setup guide](docs/user-setup.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Security policy](SECURITY.md)
- [NoX Backend](https://github.com/mapherez/nox-backend)

## Privacy and network access

NoX Sync communicates only with the Server URL configured by the user.

The plugin sends the information required to manage and synchronize the selected vault, including file manifests and file contents.

NoX Sync does not include client-side telemetry or analytics.

The optional support link opens Buy Me a Coffee only when explicitly clicked.

## Security

API keys should be treated as credentials.

Use HTTPS when exposing NoX Backend over a network you do not fully control.

Plugin security issues should be reported according to [SECURITY.md](SECURITY.md).

Backend vulnerabilities should be reported through the [NoX Backend repository](https://github.com/mapherez/nox-backend).

## Support

If you find NoX Sync useful, you can support its development through [Buy Me a Coffee](https://buymeacoffee.com/mapherez).

## License

NoX Sync is licensed under the [GNU General Public License v3.0](LICENSE).

# Security Policy

## Supported Versions

Security fixes are provided for the latest public release line.

| Version | Supported |
| --- | --- |
| `0.1.x` | Yes |

## Reporting A Vulnerability

Please do not open a public issue for a suspected vulnerability.

Use GitHub private vulnerability reporting if it is available for this repository:

```text
https://github.com/mapherez/nox-sync/security/advisories/new
```

If private reporting is not available, contact the maintainer through the GitHub profile and include only enough public detail to establish contact. Avoid posting exploit details, private server URLs, API keys, vault contents, logs with secrets, or personal data in public issues.

Helpful reports include:

- affected version or commit
- whether the issue affects the Obsidian plugin, backend, Docker image, dashboard, or GitHub workflows
- reproduction steps
- expected impact
- relevant logs with secrets removed

## Scope

In scope:

- unauthorized access to another user's vaults
- API key or session handling issues
- sync data corruption caused by backend or plugin logic
- path traversal or filesystem access outside intended vault/backend data paths
- unsafe handling of uploaded or downloaded file content
- vulnerable release, Docker, or GitHub Actions configuration

Out of scope:

- vulnerabilities in a user's own hosting provider, reverse proxy, DNS, or Google account
- compromised machines or Obsidian installations
- social engineering
- denial-of-service reports without a practical security impact
- issues requiring already-stolen API keys, Google accounts, or server access

## Security Model Summary

NoX Sync is self-hosted. The plugin sends data only to the Server URL configured by the user. The backend dashboard uses Google OAuth for login, and the plugin uses per-user `noxsync_` API keys for sync. Users should protect backend `/data` backups and API keys as sensitive data.

# Security Policy

## Supported Versions

Security fixes are provided for the latest public release of the NoX Sync Obsidian plugin. Use the latest plugin release when reporting an issue; older releases do not receive separate security fixes.

## Reporting A Vulnerability

Please do not open a public issue for a suspected vulnerability.

Use [GitHub private vulnerability reporting](https://github.com/mapherez/nox-sync/security/advisories/new) if it is available for this repository.

If private reporting is not available, contact the maintainer through the GitHub profile and include only enough public detail to establish contact. Avoid posting exploit details, private server URLs, API keys, vault contents, logs with secrets, or personal data in public issues.

Helpful reports include:

- Affected plugin version or commit.
- Whether the issue affects credentials, HTTP communication, local vault access, sync integrity, dependencies, or plugin release workflows.
- Reproduction steps.
- Expected impact.
- Relevant logs with secrets removed.

## Scope

This policy covers the Obsidian plugin, its dependencies, and its build and release configuration in this repository, including:

- Exposure or unsafe handling of the plugin's API key and local settings.
- Unsafe handling of the configured Server URL or authenticated HTTP requests.
- Unintended disclosure of vault contents by the plugin.
- Sync data corruption caused by plugin logic.
- Path traversal or plugin filesystem access outside the intended local vault.
- Unsafe handling of downloaded file content by the plugin.
- Vulnerable plugin dependencies, release artifacts, or GitHub Actions configuration.

[NoX Backend](https://github.com/mapherez/nox-backend) is an independent project. Server authorization, server-side API behavior, dashboard authentication, backend storage, and server deployment vulnerabilities should be reported through that project's security reporting process.

Other issues outside this policy include:

- Vulnerabilities in a user's hosting provider, reverse proxy, DNS, or account provider.
- Compromised machines or Obsidian installations.
- Social engineering.
- Denial-of-service reports without a practical security impact.
- Issues requiring already-stolen API keys, accounts, or server access, without a further plugin vulnerability.

## Security Model Summary

NoX Sync is an Obsidian client of NoX Backend. The plugin communicates with the Server URL configured by the user over HTTP and authenticates with the user's API key. Configure a server you trust and use HTTPS for remote connections.

Protect API keys and plugin-local settings as sensitive data. The plugin's own settings, credentials, sync state, and `.nox-sync-trash/` are excluded from synchronization.

The plugin verifies content hashes and uses the backend's synchronization and commit API. Local replacement and delete operations preserve safety copies in `.nox-sync-trash/`; clearing that folder permanently removes those copies. Backend security and backup guidance belongs to the [NoX Backend project](https://github.com/mapherez/nox-backend).

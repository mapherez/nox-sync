# Multi-User Multi-Vault Plan

## Goal

Implement NoX Sync vNext as a multi-user, multi-vault backend on the `multi-vault` branch.

The backend supports Google login for the web dashboard, admin-managed allowlisted users, one reusable API key per user for plugin access, and multiple remote vaults per user. The plugin authenticates with the user API key, fetches that user's backend vaults, and lets the currently opened Obsidian vault choose or create the remote vault it syncs with.

## Confirmed Product Decisions

- Access model: Google login with allowlist.
- Bootstrap: admin emails come from `NOX_SYNC_ADMIN_EMAILS`.
- User management: basic admin UI to add/remove/disable users.
- Regular users: see only their own dashboard, API key, and vaults.
- API key scope: one active reusable API key per user.
- Vault creation: plugin creates remote vaults.
- Dashboard vault download: zip archive.
- Dashboard vault deletion: soft delete.
- Migration from `0.1.0`: fresh start; no automatic preservation of old single-vault data.
- Session duration: 14 days.
- OAuth implementation: use maintained Go OAuth/Google ID-token libraries.
- Google users are stored by stable Google `sub`; email is used only for allowlist/display.
- The plugin continues to use API keys, not Google OAuth.
- No vault sharing between users in this phase.

## Current Implementation Phase

Phase 1 implementation complete locally. Next phase is manual OAuth/dashboard validation and two-user/two-vault remote testing.

## Status Checklist

- [x] Create root-level plan/status file.
- [x] Add multi-user/multi-vault backend schema migration.
- [x] Add auth config, Google OAuth flow, web sessions, login/logout.
- [x] Add admin allowlist bootstrap and dashboard user management.
- [x] Replace singleton API key logic with user-scoped API keys.
- [x] Add user-scoped vault list/create/delete/download.
- [x] Scope sync planning, locking, uploads, downloads, commit, abort, stale-lock cleanup, and SSE by user plus vault.
- [x] Add plugin vault list/create/select UI.
- [x] Store plugin sync state per selected remote vault.
- [x] Update README/docs for Google OAuth, admin bootstrap, dashboard, and plugin vault selection.
- [x] Run backend and plugin regression tests.

## Open Issues

- Google OAuth credentials and public callback URL must be supplied by deployment through environment variables.
- Existing `0.1.0` data is intentionally not migrated on this branch.
- Manual Google OAuth login, dashboard admin actions, and two-user/two-vault validation still need to be performed against a real deployment.
- Slow/remote sync testing remains required after local regression tests.

## Test Status

- Backend tests: `go test ./...` passing.
- Plugin tests: `npm run typecheck`, `npm run test`, and `npm run build` passing.
- Manual two-user/two-vault validation: pending.

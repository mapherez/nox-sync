import assert from "node:assert/strict";
import test from "node:test";

import {
  SyncButtonState,
  classifyBackendError,
  isFolderAlreadyExistsError,
  isHiddenPath,
  isMarkdownPath,
  isNoxSyncTrashPath,
  isPluginInternalPath,
  normalizeRequiredPath,
  normalizeVaultPath,
  syncActionProgress,
} from "../src/sync-core";

test("normalizeVaultPath normalizes relative vault paths", () => {
  assert.equal(normalizeVaultPath(" Notes\\Project.md "), "Notes/Project.md");
  assert.equal(normalizeVaultPath("/Notes//Today.md"), "Notes/Today.md");
  assert.equal(normalizeVaultPath("Notes/./Today.md"), "Notes/Today.md");
});

test("normalizeVaultPath rejects empty and escaping paths", () => {
  assert.equal(normalizeVaultPath(""), null);
  assert.equal(normalizeVaultPath("."), null);
  assert.equal(normalizeVaultPath("../outside.md"), null);
  assert.equal(normalizeVaultPath("Notes/../outside.md"), null);
  assert.throws(() => normalizeRequiredPath("../outside.md"), /Invalid conflict path/);
});

test("path predicates identify internal, trash, hidden, and Markdown paths", () => {
  assert.equal(isPluginInternalPath(".obsidian/plugins/nox-sync/data.json"), true);
  assert.equal(isPluginInternalPath(".obsidian/plugins/other/data.json"), false);
  assert.equal(isNoxSyncTrashPath(".nox-sync-trash/2026-05-14/Note.md"), true);
  assert.equal(isHiddenPath("Notes/.private.md"), true);
  assert.equal(isHiddenPath("Notes/Public.md"), false);
  assert.equal(isMarkdownPath("Notes/Readme.MD"), true);
  assert.equal(isMarkdownPath("attachments/image.png"), false);
});

test("syncActionProgress clamps file action progress into the action range", () => {
  assert.equal(syncActionProgress(0, 4), 30);
  assert.equal(syncActionProgress(2, 4), 60);
  assert.equal(syncActionProgress(4, 4), 90);
  assert.equal(syncActionProgress(10, 4), 90);
  assert.equal(syncActionProgress(0, 0), 80);
});

test("isFolderAlreadyExistsError identifies Obsidian folder collisions", () => {
  assert.equal(isFolderAlreadyExistsError(new Error("Folder already exists")), true);
  assert.equal(isFolderAlreadyExistsError(new Error("folder already exists: .nox-sync-trash")), true);
  assert.equal(isFolderAlreadyExistsError("Folder already exists"), true);
  assert.equal(isFolderAlreadyExistsError({ message: "Folder already exists" }), true);
  assert.equal(isFolderAlreadyExistsError(new Error("File already exists")), false);
  assert.equal(isFolderAlreadyExistsError({ message: 409 }), false);
});

test("classifyBackendError maps backend errors to plugin states", () => {
  assert.deepEqual(classifyBackendError({ status: 401, code: "AUTH_FAILED", message: "bad key" }), {
    kind: "auth",
    state: SyncButtonState.AuthFailed,
    detail: "NoX Sync authentication failed.",
    notice: "NoX Sync authentication failed.",
    refreshBackendStatus: false,
    closeStatusStream: true,
  });

  assert.equal(
    classifyBackendError({ status: 409, code: "SYNC_LOCKED", message: "locked" }).state,
    SyncButtonState.BlockedRemote,
  );
  assert.equal(
    classifyBackendError({ status: 409, code: "SYNC_SESSION_STALE", message: "stale" }).state,
    SyncButtonState.Error,
  );
  assert.equal(
    classifyBackendError({ status: 400, code: "HASH_MISMATCH", message: "hash mismatch" }).detail,
    "NoX Sync stopped because a file failed hash validation.",
  );
  assert.equal(
    classifyBackendError({ status: 0, code: "SERVER_UNREACHABLE", message: "offline" }).state,
    SyncButtonState.ServerUnreachable,
  );
});

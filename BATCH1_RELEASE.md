# Filebrowser 3.2.0 — transfer reliability batch

Built from your tested custom 3.1.0 archive version 3 (SHA-256 b02b7807753bfdd5eee2e2ac042af2caa74ab7ec87003f76a2c319b3b2f0cd3a). Dependencies remain at the upgraded 3.1.0 versions. This release changes application behavior and includes the rebuilt embedded frontend and native Linux amd64/arm64 executables.

## Existing features improved

- Uploads, editor saves, office saves, conversion output, extraction copies and archive creation stage file contents privately on the destination filesystem. Failed streaming, sync, scan or validation preserves the previous committed file. Destination reservations and version checks reject conflicting publication. New-file publication uses no-replace rename, with a hard-link fallback; it never deletes the previous destination to make room.
- Upload sessions have random IDs, owner identity, durable offsets, streaming SHA-256 checkpoints, retention, capacity checks and quotas. Restart reconciliation removes unacknowledged tails and resumes scan processing. A separate local journal sits beside the main database at `<database>.uploads.db`.
- The transfer panel retains outcomes (all unfinished sessions plus the latest 200 completed entries), supports per-file pause/resume/cancel, reselecting the original file after reload, retrying transient scan failures and clearing finished history. Current credentials are read for each tus request, and resume fingerprints include account and destination. Single-file parallel chunks stay disabled.
- Receiving the last byte queues bounded processing. The file becomes ready only after scan policy and publication succeed. Scan outcomes are clean, infected, skipped, disabled, pending, unknown or error. Empty/unrecognized responses cannot count as clean.
- Existing manual scan, archive and conversion status endpoints enforce stored owner/admin identity. Their existing in-memory queues remain unchanged; shared durable background jobs and prepared folder downloads are the next batch.

## Settings and defaults

In Settings → Global, the upload section now contains recovery and quota controls; the ClamAV section contains scan controls. Settings are stored in the application database, not in the server JSON.

| Setting | Default | Meaning |
|---|---:|---|
| `tus.retentionHours` | 72 | Upload recovery and completed-history retention; 1–720 hours |
| `tus.concurrentUploads` | 5 | Browser file concurrency; 1–32 |
| `tus.maxActiveUploads` | 1000 | Unfinished uploads across this instance; 1–10000 |
| `tus.maxPendingBytes` | 0 | Total declared bytes in unfinished uploads; 0 is unlimited |
| `tus.reserveBytes` | 67108864 | Keep 64 MiB free on each destination filesystem |
| `clamav.automaticMaxBytes` | 1900000000 | Automatic scan limit; -1 is unlimited; 0 normalizes to this default |
| `clamav.oversizePolicy` | `skip` | `skip`: publish with warning; `scan`: send anyway; `block`: retain unpublished |
| `clamav.scanWorkers` | 2 | Concurrent upload processing workers; 1–8 |
| `clamav.scanTimeout` | `30m` | Upload scan deadline; 1s–24h |

The existing 10 MiB chunk and retry settings remain. The oversize default preserves previous publication policy while exposing an explicit skipped result. Disabled scanning is displayed as disabled. A blocked or transiently failed scan retains staged bytes for retry or cancellation. Infected payloads cannot be retried into publication; cancel them. Conflict with a newer destination requires a fresh upload after reviewing that file.

## How recovery works

Pause stops the current chunk and preserves the last acknowledged offset. Resume uses the server's authoritative offset. After a page/browser restart, sign in and choose Select original file for the saved transfer. Size and modification time must match; you must supply the same original contents. A browser cannot continue transmitting after it closes.

The server computes SHA-256 while receiving bytes without reading the whole file into memory. API clients may supply a hex `sha256` in base64-encoded Upload-Metadata to require an expected digest. The browser does not prehash large files; the computed server digest is available from the upload status API.

POST creation remains `/api/tus/<destination>?override=true|false`. Its Location is `/api/uploads/<id>` (plus configured base URL). HEAD/PATCH/DELETE use that URL. GET `/api/uploads` lists your sessions; GET the ID returns status. POST `/<id>/pause`, `/resume`, `/retry` controls recovery. DELETE cancels private staging; DELETE with `?dismiss=true` clears terminal history without deleting the published file. Cancellation after successful publication returns a conflict instead of pretending to undo a save.

The frontend build now clears previous generated assets before producing the new bundle, preventing old hashed scripts from accumulating in native executables.

The frontend build now clears previous generated assets before producing the new bundle, preventing old hashed scripts from accumulating in native executables.

## Native installation and backups

Use [NATIVE_INSTALL.md](NATIVE_INSTALL.md) to upgrade your existing working test installation as a service operator with sudo. The helper leaves `/mnt/data` and its mounts in place. It saves the old binary, database and configuration before replacement, including an existing upload journal. Immediate startup failure restores the old binary/database and journal; if the old version had no journal, rollback removes the new one.

State-only backups intentionally omit the journal and all served data, so a test clone gets its own upload namespace. Full cold backups include the journal together with `/mnt/data` and staged files. When explicitly permitted external symlinks hold data/staging outside that tree, include their targets with `--extra`. When explicitly permitted external symlinks hold data/staging outside that tree, include their targets with `--extra`. Use the updated helper included here for backups after this upgrade. Never copy a live journal by itself or reuse a production journal on a state-only clone sharing mounted folders.

## Compatibility and limits

- Finish or cancel active 3.1.0 transfers before upgrading. Old process-memory upload state cannot be converted into the new durable sessions. Reload the browser after upgrading so frontend and backend use the same protocol.
- This is a single-writer native server design. `redisCacheUrl` is retained for configuration compatibility but does not coordinate these durable uploads. Do not run several writers against one database/journal. A test server sharing production mounts must use a dedicated test folder and filenames.
- The reserved `.filebrowser-staging` directory is hidden and denied through user APIs, including symlink aliases. Existing content using that exact reserved name becomes unavailable through the app. Temporary files consume additional disk space while an old destination still exists.
- Replacements preserve owner/group, mode, Linux POSIX access ACLs and user xattrs where supported. Atomic replacement creates a new inode and does not update other hard links. A destination that is itself a symlink is rejected for replacement. Permission/metadata failures leave the previous file intact.
- Linux amd64 runs and failure recovery are tested locally. ARM64 is cross-built; ARM execution, actual NAS/ZFS ACL behavior, reverse proxy limits and live ClamAV/ConvertX/Collabora/Erugo services still require checks on your test server. File-level atomicity is provided; extraction or folder copying is not one atomic transaction.
- External processes do not participate in the in-process reservation table. Version checks detect ordinary changes but are not a distributed compare-and-swap guarantee. Durability depends on the filesystem honoring sync and rename.

Validation results and repeatable scripts are in `validation/results/batch1/`, `validation/batch1-transfer.py`, and the Go/frontend/native tests. See `BATCH1_VALIDATION.md` for exact outcomes and environmental limits.

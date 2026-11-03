> **Current fork documentation:** For File Browser **3.3.1**, use [README](README.md) and [installation/upgrade](docs/INSTALL_3.3.1.md), plus [integrations](docs/INTEGRATIONS_3.3.1.md). Material below is retained as historical/underlying reference and may describe earlier versions.

# Filebrowser 3.3.0 — existing-feature enhancements

Historical 3.3.0 baseline report. The 3.3.1 correction restores the original directory renderer while retaining server pagination; it supersedes references below to viewport virtualization. Read DIRECTORY_UI_RELEASE.md and DIRECTORY_UI_VALIDATION.md for the current directory behavior and checks.

This release builds on 3.2.0 and implements all eleven areas in the enhancement report. It includes the complete source, embedded frontend, and static Linux amd64/arm64 executables. It uses the existing native database, configuration, service and mounted data paths.

| Area | Implemented behavior | Where to use it |
|---|---|---|
| Background jobs | One persistent journal for scans, archive creation/extraction, conversions, prepared downloads and Erugo shares. Bounded workers, durable progress, cancellation, retries, crash recovery, and owner/admin access. A separate index worker keeps directory navigation responsive. | Background jobs panel; `/api/jobs` |
| ConvertX | Separate connection, upload, processing and download budgets. Persisted upstream job IDs resume polling after restart. Duplicate active submissions are coalesced, retries with the same explicit idempotency key reuse the stored result, and ambiguous submissions require review. Atomic output publication checks the submitted destination version. | Existing Convert action; timeouts in About / System |
| Collabora / WOPI | Persistent expiring office leases share the upload/mutation conflict policy. Saves check the locked version, publish atomically, journal save intents and recover after interrupted publication. Uploads, editor saves, rename, move and delete respect the lease. | Existing office action |
| Archive extraction | Expanded-byte, entry-count, depth, time and destination free-space budgets. Per-file publication is atomic. Published entries are recorded and verified during restart recovery. | Existing Extract action; limits in About / System |
| Erugo | Regular files keep exact-length streaming. Folder ZIPs become persistent private artifacts, then transfer as durable jobs with byte progress and cancellation. Capacity is bounded. Upstream uncertainty is recorded instead of automatically creating a second share. | Existing Share action; Background jobs |
| Large directories | Disk-backed snapshot indexes built in bounded batches, server-side sorting/filtering and pages, viewport virtualization for list/grid views, and selection retained across pages. Refresh forces a new snapshot. | Directory page controls |
| Previews / editor | Viewers, prompts and locales load on demand. Large CSV/log/text previews fetch metadata and bounded windows, without downloading the entire text first. CSV pages preserve quoted multiline records. TIFF-family decoding runs in a cancellable worker with encoded-size, pixel and time caps. Server-resized previews use native image rendering. | Open a file normally; Edit as text retains existing editor limits |
| EPUB position | Position and font size are stored separately by account and book path. The legacy shared position is migrated only to the first book opened after upgrade. Numeric/string locations and corrupt stored values are handled. | Existing EPUB reader |
| Authentication | Persistent revocable sessions, device listing, single/all-session sign-out and credential-change invalidation. Renewal is coordinated and cannot restore an account after logout/account switching. Expired access tokens can renew only through the renewal endpoint while their persistent session remains valid. Current permissions are reloaded for requests. | About / System → Signed-in sessions |
| Integration diagnostics | Saved checks, configuration-change detection, advertised versions when available, capabilities/contract details, schema checking and secret-redacted errors. ConvertX checks health JSON and target schemas; Collabora exposes discovered extensions. | About / System → Integration diagnostics |
| Version/build visibility | About / System shows application/build, Go runtime, operating system, architecture and enabled integrations. Administrators can inspect backend package versions and edit resource limits. | Settings → About / System |

## Defaults and practical boundaries

- Two action workers, plus one directory index worker. Action workers are configurable from 1–8 and take effect after a service restart. Default active queue limit: 1,000; retained job history: up to 10,000 entries / seven days.
- Extraction/source packaging: 100 GiB, 100,000 entries, depth 64, one hour; destination free-space reserve: 1 GiB. These limits are configurable. A cancelled extraction keeps files already published. Recovery resumes only entries whose recorded versions still match; external changes require review.
- Prepared ZIP storage: 100 GiB per instance, with aggregate capacity accounting. Artifacts expire after 24 hours and are cleaned on subsequent preparation. Downloads support authenticated GET/HEAD, byte ranges and a SHA-256 ETag. Keep the application database directory on storage with enough capacity for prepared ZIPs.
- Previous contents: up to five versions per file, seven days, within 100 GiB per containing directory. Versions live in the protected `.filebrowser-staging/history` directory beside the file. Expiry is enforced on access; cleanup also runs when a new version is saved. If recovery storage is exhausted, replacement fails while preserving the destination. Restore through the file information dialog.
- Large text previews: at most 256 KiB / 2,000 rows per UI page; the API caps a request at 1 MiB. A single CSV record exceeding the window is rejected clearly. Windows are version-checked. Existing editor/ordinary preview size protections remain.
- Image worker: 32 MiB encoded input, 32 million pixels, at most 64 image directories, 15-second decoding budget. Download unsupported/oversized images for desktop viewing.
- Directory pages default to 200 entries, maximum 500. Each index is capped at one million entries / 128 MiB metadata; a bounded set of snapshots is kept. Snapshots expire after ten minutes and are invalidated when directory metadata changes. Explicit Refresh bypasses reuse. Name filtering applies before pagination. Existing folder-picker and conflict-check callers retain their original complete-listing API behavior; normal directory navigation uses the paged API. Selection across pages is held for the current directory; changing directory or reloading clears it.
- ConvertX: connection/TLS 15 seconds; upload, processing and output download two hours each. The original `timeout` remains the individual API-response/header budget (default two minutes). These controls do not change limits imposed by ConvertX, proxies or storage.
- Access tokens retain the configured lifetime. Persistent sessions last at most 30 days, with at most 50 live sessions per account and 10,000 records per instance. Upgrading from legacy tokens preserves valid tokens until their original expiry; sign in again afterwards. Revocation stops new requests and renewal; it cannot retract bytes already sent in an open response.

Erugo's current integration is an exact-length POST, not a resumable byte-upload protocol. A folder artifact survives a retry; an upstream transfer restarts from byte zero. If Filebrowser loses the upstream response during submission, check Erugo/ConvertX first. The job panel provides **I checked: retry the submission** and **Close after review**. Retrying after manual confirmation uses the same job key. Automatic retry never assumes that an unacknowledged external submission failed. No claim is made that an upstream service honors the optional idempotency header unless it advertises that capability.

Office leases coordinate writes made through this Filebrowser instance. Other applications writing directly to the filesystem do not participate in those leases. Recorded file versions and publication digests detect changes and preserve conflicts for review. Existing protections against final symlink destinations and private staging aliases remain.

## API compatibility

Existing scan/archive-create/ConvertX job status routes still return their established response shapes and enforce ownership. Use a stable `Idempotency-Key` header when retrying an external-action request after losing its response; the bundled frontend assigns an operation key. A deliberate later operation receives a new key.

New common routes: `GET /api/jobs`, `GET /api/jobs/{id}`, `POST /api/jobs/{id}?action=cancel|retry|review_retry|dismiss`.

Archive extraction and Erugo sharing now return HTTP 202 with `jobId` by default; clients poll `/api/jobs/{id}` until `succeeded` and consume `result`. The bundled frontend handles this. Direct callers can retain synchronous behavior with `?sync=true`. Prepared downloads use `POST /api/downloads` and `GET|HEAD /api/downloads/{jobId}` after successful preparation. New session, history, diagnostics, text-window and system routes require normal authentication; administrative diagnostics/settings remain admin-only.

ConvertX requires its File Browser integration API (`/api/health`, `/api/conversions`, `/api/convert`, `/api/jobs/{id}`); a reachable HTML home page is insufficient. Its credentials are not forwarded to foreign-origin output URLs or redirects. No new third-party dependencies were added in 3.3.0; the pinned versions from 3.2.0 remain.

## Native upgrade and backup

Follow `NATIVE_INSTALL.md` as **the service operator** on **the staging host**. An upgrade uses the existing `/root/filebrowser.db`, `/etc/filebrowser.json`, environment, systemd drop-ins and mounted `/mnt/data`. It does not restore or rearrange data. Immediate startup failure triggers the installer's binary/database/journal rollback.

The existing `/root/filebrowser.db.uploads.db` sidecar now also stores jobs, session revocations, office leases, directory indexes and diagnostics. Keep it paired with the main database for full cold backups. Full backups now also include its `.artifacts` directory; previous file versions are included with data. State-only clones omit the journal/artifacts and incomplete-operation namespace so an independent test instance cannot resume production jobs against shared files. Include `/etc/filebrowser-test.json` using `--extra` when backing up the test host.

Use a dedicated test directory when the test host and production expose the same mounts. An application rollback restores metadata/executable snapshots, not files or external actions performed after the upgrade.

## Validation

See `FULL_ENHANCEMENTS_VALIDATION.md` for exact checks and evidence. The native transfer test uploaded and fully downloaded **5 GiB + 123 bytes**, forced a process kill/restart during upload, verified the resumed offset and full SHA-256, tested a byte range, and verified private staging. Integration behavior is checked with local protocol fixtures. Real ConvertX, Erugo, Collabora, ClamAV, NAS mounts/ACLs and the user's systemd service were not contacted or changed. ARM64 is cross-built and inspected, not run on ARM hardware.

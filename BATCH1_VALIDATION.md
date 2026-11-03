# Filebrowser 3.2.0 batch 1 validation

2 November 2026 (Europe/Bucharest). Checks used isolated temporary data and generated files. No live installation, mounted user data or external integration service was changed.

| Check | Result | Evidence |
|---|---|---|
| Full Go suite with race detector | Pass | `validation/results/batch1/go-race-final.log` |
| Go vet and GolangCI-Lint 2.15.0 | Pass; zero lint findings | `go-vet-final.log`, `go-lint-final.log` |
| Frontend lint, Vue/TypeScript checks, production build | Pass | `frontend-lint.log`, `frontend-build.log`, `frontend-config-final.log` |
| Frontend unit tests | 42 pass across 10 files | `frontend-test.log` |
| Native installer/restore fixtures | 30 pass | `native-install.log` |
| Backup-toolkit regression fixtures | 8 pass | `backup-toolkit.log` |
| Linux x86-64 executable | Executed; version and checksum verified | `native-builds.json`, installer manifest |
| Linux ARM64 executable | Cross-built; ELF architecture and CGO-disabled metadata verified | `native-builds.json`; execution not tested |
| Actual native upload/download | 5,368,709,243 bytes each, exact SHA-256 match | `native-final.log` |
| Abrupt server restart during transfer | Resumed the persisted offset; previous destination stayed intact until publication | `native-final.log`, Go recovery tests |
| Production browser transfers | Pause/resume, reload/reselect/resume, reserved filename characters, cancel, clear history, zero page errors | `native-final.log` |
| Existing video, EPUB and sanitized math preview | Each browser check passed | `browser-media.log`, `browser-epub.log`, `browser-math.log` |
| Linux metadata | Mode, user xattrs, default POSIX ACL inheritance and replacement ACL preservation checked | `files/atomic_linux_test.go`, Go suite |

The final native transfer used a database initialized by the previous 3.1.0 executable. It uploaded and downloaded 5 GiB + 123 bytes, killed the server after acknowledged chunks, restarted it with the same journal, checked HEAD offset, finished publication, verified a byte range and compared the entire download to an independent streaming SHA-256.

- SHA-256: `a6f00bba8ff9c3102bf332c3c24bca5c7e28e0526c24f393f44579d7851fe264`.
- Peak native child-process RSS: 30,716 KiB (30.0 MiB), sampled after the native transfer and before the browser phase. This is the maximum across initializer/server child processes, not a NAS benchmark.
- Scan policy for this large native test: disabled, correctly reported as disabled. Clean, infected, unknown, error and oversized skip/block behavior were tested against local HTTP fixtures; live scanner performance was not measured.

Fault tests cover invalid creation metadata, interrupted bodies, failed sync, durable offsets/hash state, unacknowledged tails, owner checks, cancellation without published-file delete permission, safe expiry, transient scan retry after restart, asynchronous final PATCH, publication checkpoint recovery, shared destination reservations and non-destructive history clearing. The actual router is tested at root and under `/fb` to prevent upload-list redirects from breaking browser recovery. Older unfinished sessions remain visible even when completed history exceeds 200 entries. Recovery through explicitly permitted external symlinks is also covered. Recovery through explicitly permitted external symlinks is also covered.

Browser checks used Chrome Headless Shell 156.0.8078.80. This execution environment requires `--no-zygote --single-process`; component checks ran in separate invocations to avoid reusing isolated contexts in that mode. The existing media fixture now verifies that keyboard speed controls increase and decrease the rate, rather than assuming six presses must produce exactly 3× playback. Application media behavior was not changed. Vite emits an existing table-markup warning in the Users view; production compilation passes.

## Reproduce

Use Go 1.27.3, Node 26.13.0 and pnpm 12.16.1. From the source root:

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run lint
pnpm --dir frontend run test
pnpm --dir frontend run build
go test -race -timeout=90s ./...
go vet ./...
golangci-lint run --timeout=5m
python3 -m unittest discover -s native-install -p 'test_*.py'
FILEBROWSER_TEST_BYTES=5368709243 python3 validation/batch1-transfer.py
```

Set `FILEBROWSER_BASELINE_BINARY` to a saved 3.1.0 executable to repeat database-upgrade coverage. The default initializer is the packaged executable. For browser transfer checks, also set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`, put the required Node version on PATH, and set `FILEBROWSER_BROWSER_ARGS` to a JSON array when environment-specific flags are needed. See `frontend/tests/batch1-app.mjs`. The normal browser component command is `pnpm --dir frontend run test:browser`.

Actual Jailmaker/ZFS/NAS mounts, NFSv4 ACLs, reverse proxy limits, real systemd startup on your machine, ARM64 runtime and live ClamAV/ConvertX/Collabora/Erugo endpoints remain deployment checks on `a staging host`. Use a dedicated test directory when mounts are shared with production. New shared durable job queues, prepared folder downloads and folder-level transactions are outside this batch.

# Validation — 2026-11-01

The updated custom source was built and tested with Node 26.13.0, pnpm 12.16.1, Go 1.27.3, and Python 3.15.0 on Linux amd64.

| Check | Result |
|---|---|
| `pnpm install --frozen-lockfile` | Pass, strict peer checking and versioned patches applied |
| `task build` | Pass: Vue SFC checking, native TypeScript 7 module checking, configuration checking, modern/legacy frontend bundles, and backend executable |
| `pnpm test` | 36 tests pass across 9 files |
| `pnpm run test:browser` | 3 tests pass using Playwright 1.64.1 and Chromium headless shell 156.0.8078.4 |
| Production browser smoke | Login and file listing pass using the built binary and embedded production assets |
| TUS upload/resume and raw download | 67,108,987 bytes uploaded and downloaded; offset recovery, HTTP 206 byte range, length, and SHA-256 match |
| Existing-database smoke | Database initialized by a binary built from the original dependencies; upgraded server opens it and passes login, listing, and the transfer checks |
| Frontend lint | Pass |
| `go test -race ./...` | Pass across the backend suite |
| `go vet ./...` | Pass |
| golangci-lint 2.15.0 | Pass, zero issues |
| `go mod verify` | All modules verified |
| Cross-compilation | 12 targets pass: Linux amd64/arm64/armv6/armv7/386/riscv64; Windows amd64/arm64; macOS amd64/arm64; FreeBSD amd64; OpenBSD amd64 |
| `pnpm audit --json` | Zero known vulnerabilities at all severities |
| `govulncheck ./...` | No reachable vulnerable symbols; one module-only advisory described below |
| GoReleaser 2.19 configuration check | Pass; existing Docker configuration has a future-deprecation notice |
| `mkdocs build --strict` | Pass with all dependencies installed from the hash-locked requirements |
| CI/Compose/Task/pnpm/release YAML | Parsed successfully; release job prerequisites exist |

The Chromium tests cover actual video playback, keyboard pause/seek, 3× playback speed, caption toggling, fullscreen/speed controls, EPUB rendering and CFI reporting, and sanitized math rendering. Unit tests cover the xmldom EPUB fallback, math sanitization, Pinia/router integration, media URL compatibility, and the existing upload/copy/move conflict rules. The transfer helper starts an isolated temporary server; it never uses an existing application's database.

The Go scanner reports **GO-2026-5932** for the unmaintained `golang.org/x/crypto/openpgp` package because its parent module is in the graph. This application does not import that package, and the finding has no package or function trace. The latest x/crypto module is retained for the maintained crypto packages used by the app; no affected symbol is reachable in this scan. See [the Go advisory](https://pkg.go.dev/vuln/GO-2026-5932). This is distinct from a claim that the application has no architectural security issues.

Expected frontend build messages remain for Go-template-generated reCAPTCHA/custom-CSS URLs and large bundles. The server supplies those URLs at runtime; production browser login and asset loading pass. Existing broad locale bundles and legacy-browser output remain, so this change does not claim a bundle-size optimization.

## Reproduce

```sh
task build
cd frontend
pnpm run lint
pnpm test
pnpm exec playwright install --with-deps chromium
pnpm run test:browser
pnpm audit
cd ..
go test -race ./...
go vet ./...
go mod verify
python3 validation/smoke-transfer.py
```

For a production-browser smoke run, set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` to a local compatible Chromium executable before running the Python helper. To exercise an old database, set `FILEBROWSER_BASELINE_BINARY` to a binary built from the original source. Test credentials and data are temporary and removed afterward.

Recorded command output is in `validation/results/`; cross-compilation details are in `validation/results/cross-build.json`. Cross-compilation verifies compilation, not execution on those operating systems.

## Not executed here

Docker image builds and runtime container checks were unavailable because this environment has no Docker daemon. Base tags and manifest digests were verified against their registries, and the source Dockerfile includes the required patch before its frozen dependency install. Real Erugo, ConvertX, ClamAV, Collabora, and Redis deployments were not available for live integration testing. The configured service versions and credentials must be checked in staging. This upgrade was not deployed or published.

## Packaged native installer

The archive also includes ready-built Linux amd64 and arm64 executables. The amd64 executable reports `v3.1.0/source-archive` and passed the production-browser and old-database transfer smoke run. ARM64 was cross-compiled and identified as a static AArch64 ELF executable, without ARM runtime testing. Five installer tests cover fresh backup restore with the new executable, upgrading an already restored test instance, stopped-service state, and rollback of the binary/database after a startup failure. See `validation/results/native-install/`.

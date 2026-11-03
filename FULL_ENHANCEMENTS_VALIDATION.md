# Filebrowser 3.3.0 validation

Historical 3.3.0 baseline report. The 3.3.1 correction restores the original directory renderer while retaining server pagination; it supersedes references below to viewport virtualization. Read DIRECTORY_UI_RELEASE.md and DIRECTORY_UI_VALIDATION.md for the current directory behavior and checks.

Validation was performed in an isolated Linux workspace, using Go 1.27.3, Node 26.13.0, pnpm 12.16.1 and Chrome Headless Shell 156.0.8078.80. No live NAS, production installation or external integration was modified.

| Check | Result / scope |
|---|---|
| Go race suite | `go test -race ./...`; all packages pass |
| Go vet / GolangCI | `go vet ./...`; GolangCI 2.15.0 reports zero issues |
| Frontend checks | Vue, native TypeScript and configuration checks, ESLint and production build pass |
| Frontend unit tests | 46 tests, including EPUB key isolation/migration and operation keys on HTTP LAN origins |
| New backend regression cases | 20 cases covering journal recovery, deduplication/idempotency, owner-only status, atomic publication, persistent office leases/save versions, session revocation/renewal, directory pages, archive budgets, resumable prepared downloads, CSV boundaries, bounded workers and external integration contracts |
| Native upgrade / restore | 30 installer fixtures pass, including rollback and upload-journal preservation |
| Backup toolkit | Eight migration fixtures pass; full backups include prepared artifacts; state-only clones omit the operation namespace |
| Native browser flow | Real login; paged/virtualized directory; cross-page selection; server filtering; bounded CSV/log loading; TIFF worker; previous contents; durable ZIP/range/ETag; System/session/diagnostic UI; no runtime errors |
| Viewer compatibility | Video.js playback, captions/speed/fullscreen; Vue Reader EPUB rendering and CFI callbacks; sanitized browser math preview all pass |
| Large-file native transfer | 5,368,709,243 bytes uploaded and fully downloaded; forced process kill/restart during upload, acknowledged offset recovery, SHA-256 equality, byte-range and private-staging checks pass |
| Executables | Static Linux amd64 and arm64 binaries, embedded production frontend, version and source checksum recorded in manifests |

The large-transfer digest is `a6f00bba8ff9c3102bf332c3c24bca5c7e28e0526c24f393f44579d7851fe264`. Exact run duration and backend peak RSS are in `validation/results/full-enhancements/large-transfer.json`.

Raw evidence is under `validation/results/full-enhancements`. Source hashes are in `SOURCE_MANIFEST.json`; executable hashes are in `native-install/binaries.json`. The About/System source checksum matches the source/asset manifest. Validation logs and native binaries are excluded from that source identity to avoid recursive build inputs.

Reproduction commands, after installing the pinned frontend/toolchain dependencies:

```bash
go test -race ./...
go vet ./...
golangci-lint run --timeout 5m
pnpm --dir frontend test
pnpm --dir frontend lint
pnpm --dir frontend build
python3 -m unittest discover -s native-install -v
FILEBROWSER_TEST_BYTES=5368709243 python3 validation/batch1-transfer.py
# Set PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH to an installed Chromium executable.
python3 validation/full-app-browser.py
```

The protocol fixtures validate the implemented ConvertX, Erugo, scanner and WOPI contracts. Live service versions/capabilities, real mounted-filesystem ACL behavior and systemd on a staging host still require the user's normal test installation. ARM64 was cross-built and inspected; it was not executed on ARM hardware. The development-only EPUB fixture emitted a ResizeObserver notification while its rendering/CFI test passed; the production app browser flow had no runtime errors.

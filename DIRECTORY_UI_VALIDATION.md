# Filebrowser 3.3.1 validation

Validated on 2026-11-03 against disposable local data and an isolated native Linux amd64 instance. No user's installation, mounted data or external integration was used.

| Check | Result |
| --- | --- |
| Baseline comparison | Verified and extracted the complete 3.2.0 and 3.3.0 release archives; compared directory components, listing styles, stores, APIs, navigation and affected viewers/prompts. The changes are described in DIRECTORY_UI_RELEASE.md. |
| Frontend type checks and production build | Vue, native TypeScript and Vite configuration checks pass. A clean production build includes the restored renderer and lazy viewers. |
| Frontend tests | 56 tests in 14 files pass. New tests cover debounced filtering, abort/late-response handling, immediate sorting/persistence, off-page selection clearing, navigation isolation, coordination with automatic refreshes, file-only drop fallback, complete directory batches, inaccessible entries and empty drops. |
| Frontend lint | Passes without warnings. |
| Go race suite | `go test -race ./...` passes. Mixed directory tests cover 251 folders and 257 files, seven pages per sort/direction, section boundaries, total counts, order, duplicates and omissions. |
| Go static checks | `go vet ./...` passes; GolangCI-Lint 2.15.0 reports zero issues. |
| Installer fixtures | 30 native installer/migration tests pass, including preservation, rollback and existing mounted-data scenarios. The installer selects and verifies version 3.3.1. |
| Real application browser flow | Login, bounded paging, cross-page selection and clearing, return to a file on a later page, original folder/file sections, card styling, live filter/clear, immediate sort, input keyboard isolation, right-click menu/rename, internal drag/move, native file drop into a folder, all three view modes, mouse/keyboard column sorting, mobile layout and menu bounds pass. |
| Existing enhancement smoke | Bounded CSV/log windows, TIFF worker decoding, previous contents, prepared ZIP/range/ETag, About/System, sessions, diagnostics and limits UI pass. No browser runtime errors. |
| Viewer compatibility | Video.js playback/captions/speed controls, EPUB rendering/CFI and sanitized browser math tests pass. |
| Transfer regression | 67,108,987 bytes uploaded and downloaded with matching SHA-256, forced process restart, offset recovery, HTTP range and protected staging checks. The same source also passes production UI pause/resume, reload/reselect/resume, reserved filename, cancellation and history clearing without deleting files. |
| Native release | Static Linux amd64 and arm64 executables built from the source identity in SOURCE_MANIFEST.json. Native execution/browser checks run on amd64; arm64 is cross-compiled. Archive integrity, source entries, embedded build identity and both binary checksums are verified before delivery. |

The explicit no-worker fixture mode now also suppresses its directory index worker. This prevents a test's manual index execution from racing an unintended background execution; normal application startup still starts the dedicated index worker.

The transfer above is a targeted patch regression, rather than a new claim that every storage, proxy or integration configuration was exercised. The preceding release's multi-gigabyte validation remains in its historical report. No live ConvertX, Collabora, Erugo or ClamAV service was contacted for this directory correction.

Logs for this patch are under `validation/results/directory-ui-3.3.1/`. Older logs and FULL_ENHANCEMENTS_* reports describe earlier releases and are retained as history. The former separate viewport virtualizer is superseded by the original progressive renderer within bounded server pages.

## Compared archives

- 3.2.0: `79c1ea16e8297ea69dde7cf3645d785bfe6b8d4dc966e6464bee22c0270f4cbc`
- 3.3.0: `63bd6df9a8578f2dc25f2a85f6b728ba6433c8ee4509c5c12ced29262d5912ee`

The packaged build uses Go 1.27.3, Node 26.13.0 and pnpm 12.16.1. Package pins and lockfiles are retained from 3.3.0. This patch changes no package versions or persistent database formats.

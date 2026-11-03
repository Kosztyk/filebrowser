# Kosztyk File Browser — fork changelog

## 3.3.1 (2026-11)

- Restored folder-first listing, original list/grid/gallery cards, context menus and drag-and-drop after the 3.3.0 virtual-list regression.
- Preserved server-side pagination (200 entries by default, up to 500), instant filtering, sorting and selection across pages; corrected keyboard and navigation behavior.
- Carried forward the 3.3.0 durable jobs, resumable uploads, prepared downloads, version history, integration diagnostics, revocable sessions, file viewers and service integrations without intentionally changing their contracts.
- Retired the obsolete v18.1 deploy helper that targeted `master`, replacing it with a guarded 3.3.1 release-branch publisher.
- Updated deployment guides to distinguish this fork from archived upstream, specify a clean production upgrade path and provide credential-free integration examples.

## 3.3.0

See [FULL_ENHANCEMENTS_RELEASE.md](FULL_ENHANCEMENTS_RELEASE.md) for details, operational limits and API compatibility. Highlights: persisted jobs; version history; prepared folder downloads; bounded archive extraction; atomic writes; office leases; resilient ConvertX operations; Erugo share jobs; indexed listings; bounded previews; per-book EPUB reading state; session management; integration diagnostics.

## 3.2.0 / 3.1.0

Historical dependency/feature updates: [BATCH1_RELEASE.md](BATCH1_RELEASE.md), [UPGRADE_NOTES.md](UPGRADE_NOTES.md) and [DEPENDENCY_CHANGES.md](DEPENDENCY_CHANGES.md). Existing Erugo reverse shares, ClamAV, ConvertX, Collabora and archive actions are carried into 3.3.1.

**Version designation:** `VERSION` is `3.3.1`; the packaged Linux binaries report `v3.3.1/directory-fix`. This is the fork's version, not upstream's published version.

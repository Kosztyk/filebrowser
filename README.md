# File Browser 3.3.1 — Kosztyk advanced fork

[![Source](https://img.shields.io/badge/source-Kosztyk%2Ffilebrowser-blue)](https://github.com/Kosztyk/filebrowser)
![Version](https://img.shields.io/badge/version-3.3.1-informational)

This is a **custom-maintained fork**, not an official upstream File Browser release or a claim that upstream development has resumed. It preserves the original browser-based file manager, administrator controls and native File Browser shares while adding integrated file services and safer long-running operations. The upstream File Browser project was archived in September 2026; this fork must be assessed and maintained separately.

![File Browser](branding/banner.png)

## What's new in 3.3.1

**3.3.1 corrects the directory UI regressions in 3.3.0:** folders appear before files; list, grid and gallery styling, thumbnails and permission-aware right-click menus work with paginated directories. Search-as-you-type, sorting, paging, selection persistence, keyboard actions, drop targets and folder totals have been corrected. See [3.3.1 UI release notes](DIRECTORY_UI_RELEASE.md).

The 3.3.x line also adds:

| Area | Features |
|---|---|
| **Durable operations** | Background jobs for scanning, archive operations, conversion, prepared downloads and Erugo sharing; cancellation, retry/review and recovery after restart. |
| **Transfers & recovery** | TUS resumable uploads, conflict handling, atomic publication, version history / restore, resumable ranged downloads of prepared folders and upload/session durability. |
| **Archive tools** | Browse ZIP/other supported archives, preview/download individual contents, extract selected entries or entire archives, and apply extraction resource limits. |
| **Erugo** | Share files/folders, download bandwidth limits, reverse shares / file requests with upload speed limits, long-upload handling, native-sharing fallback. |
| **ConvertX** | Convert selected files from toolbar or context menu, choose supported formats, track server-side conversion jobs, avoid accidental overwrites. |
| **ClamAV** | Automatic scans on uploads plus manual scans using a compatible HTTP REST scanning API; configurable size/oversize policy; infected files are rejected. |
| **Collabora Online** | Open/edit office documents via WOPI, callback host configuration, expiring office leases and conflict-aware save handling. |
| **Large folders & previews** | Bounded indexed/paginated listings, bounded text/CSV preview windows, safer image decoding, EPUB reader progress. |
| **Security & administration** | Persistent revocable application sessions, per-device sign-out, integration diagnostics, package/runtime visibility and resource limits. |

**Important limits:** An Erugo outbound upload is *not* resumable mid-request; a prepared folder can survive a retry, but the upload to Erugo starts again. Application version history is not a replacement for a filesystem backup. Existing tokens and external sessions have different lifetimes; validate your own deployment. See [FULL_ENHANCEMENTS_RELEASE.md](FULL_ENHANCEMENTS_RELEASE.md).

## Install or upgrade

**Fresh installation (from this repository):** follow [Installation — Docker or Linux systemd](docs/INSTALL_3.3.1.md). The Docker build uses `Dockerfile.source`, not the release-image `Dockerfile` that expects an already-built executable. Run `docker compose build` or build the native frontend first, using the documented Node 26 / pnpm 12 / Go 1.27 toolchain.

```bash
docker run -d \
  --name filebrowser \
  --restart unless-stopped \
  -p 8000:80 \
  -v filebrowser_data:/srv \
  -v filebrowser_db:/database \
  -v filebrowser_config:/config \
  kosztyk/filebrowser:3.3.1
```

### Default credentials

**Default user:** `admin`

**Find initial password:**

```bash
docker logs filebrowser 2>&1 | grep "initialized with randomly generated password"
```

The command returns:

User 'admin' initialized with randomly generated password: XXXXXXXXXXXX

### Reset password

Stop File Browser before resetting its password:

```bash
docker stop filebrowser
```

Then run:

```bash
docker run --rm \
  -v filebrowser_db:/database \
  -v filebrowser_config:/config \
  --entrypoint /bin/filebrowser \
  kosztyk/filebrowser:3.3.1 \
  --config=/config/settings.json users update admin \
  --password 'YourNewSecurePassword123!'
```

Restart File Browser:

```bash
docker start filebrowser
```


**Upgrade:** follow [3.3.1 upgrade and rollback](docs/INSTALL_3.3.1.md#upgrade-an-existing-installation). Preserve your **database, upload journal, journal artifacts, configuration, environment, systemd unit and data mounts**. Do not replace a running binary before backups or migrate a production database using the test-machine-only installer.

**Integrations:** see [Integration setup, API checks and troubleshooting](docs/INTEGRATIONS_3.3.1.md) and the [sanitized six-service Compose example](deploy/examples/compose.integrations.yaml). A matching [.env.example](deploy/examples/.env.example) contains placeholders, no real passwords, IP addresses or API tokens. The individual vendor containers are optional; File Browser works without them.

Quick links: [Release changes](CHANGELOG_CUSTOM.md) · [Detailed enhancements](FULL_ENHANCEMENTS_RELEASE.md) · [3.3.1 directory corrections](DIRECTORY_UI_RELEASE.md) · [Upstream-compatible documentation](docs/README.md) · [Security](SECURITY.md) · [Contributing](CONTRIBUTING.md).

## Build requirements

- Node.js **26.13.0**, pnpm **12.16.1** for the frontend.
- Go **1.27.x** for the backend (module requirement Go 1.27).
- Docker with BuildKit when using `Dockerfile.source`, or a supported Linux host for native systemd installation.

```bash
# Linux/macOS with the matching prerequisites installed
cd frontend
pnpm install --frozen-lockfile
pnpm run build
cd ..
CGO_ENABLED=0 go build -buildvcs=false -trimpath \
  -ldflags='-s -w -X github.com/filebrowser/filebrowser/v2/version.Version=3.3.1' \
  -o filebrowser .
./filebrowser version
```

Prebuilt Linux amd64/arm64 binaries are provided in the uploaded 3.3.1 distribution archive; they are not produced by a regular source checkout and should not be mistaken for a signed GitHub release. The bundled `native-install/install_test.py` targets a **specific test-machine layout**; it is not a generic production migration script.

## Security model

File Browser manages files with the permissions of its service account. Use a reverse proxy with TLS and independent authentication for remote access; keep the optional command runner disabled; avoid `root` and privileged containers; restrict access to ClamAV, ConvertX, Erugo and Collabora APIs. Persistent session revocation is an addition in this fork, but it does not retract already-transmitted data and does not imply independent security auditing. Do not reuse example credentials. Consult [SECURITY.md](SECURITY.md) and upstream [security advisories](https://github.com/filebrowser/filebrowser/security/advisories).

## License and provenance

Apache License 2.0, copyright File Browser contributors. This fork incorporates the original [File Browser](https://github.com/filebrowser/filebrowser) project, which is archived, with additional custom work. Upstream documentation and changelog are retained for historical reference; fork-specific release notes appear above.

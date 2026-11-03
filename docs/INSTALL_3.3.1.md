# File Browser 3.3.1 installation, upgrade and rollback

These procedures apply to **Kosztyk/filebrowser** custom builds, not the archived upstream image. **Do not deploy to production before a backup and a test upgrade on a copy of the real database.** Verify the service's current paths (`systemctl cat filebrowser`, `filebrowser config cat`) rather than assuming `/root/filebrowser.db` or `/srv`.

## Fresh install — Docker Compose

1. Clone this repository at a reviewed 3.3.1 commit, or extract the provided complete 3.3.1 source archive. Docker BuildKit must be enabled.
2. Create writable `data`, `database`, `config` folders, owned by the container's UID 1000 (or use named volumes). **Do not** import production data into an empty anonymous volume by accident.
3. Create a deployment `compose.yaml`:

```yaml
services:
  filebrowser:
    build:
      context: .
      dockerfile: Dockerfile.source
    image: kosztyk/filebrowser:3.3.1
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:80" # expose through your TLS reverse proxy
    environment:
      FB_ADDRESS: "0.0.0.0"
      FB_PORT: "80"
      FB_ROOT: "/srv"
      FB_DATABASE: "/database/filebrowser.db"
    volumes:
      - ./data:/srv
      - ./database:/database
      - ./config:/config
```

```bash
mkdir -p data database config
sudo chown -R 1000:1000 data database config

docker compose build filebrowser
docker compose up -d filebrowser
docker compose logs --tail=100 filebrowser
```

Confirm the binding, persistent storage and initial administrator account creation according to the CLI/startup output. For a publicly accessible service, configure TLS and authentication in a reverse proxy before opening firewall access. For the integrated optional services, see [INTEGRATIONS_3.3.1.md](INTEGRATIONS_3.3.1.md).

## Fresh install — Linux native (systemd)

Build from source with Node **26.13.0**, pnpm **12.16.1** and Go **1.27.x**:

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm run build
cd ..
CGO_ENABLED=0 go build -buildvcs=false -trimpath \
  -ldflags='-s -w -X github.com/filebrowser/filebrowser/v2/version.Version=3.3.1' \
  -o filebrowser .
./filebrowser version
```

Install binary as root and run the service as a dedicated unprivileged user. Illustrative, fresh-only unit (choose your own data/db directories and confirm CLI flags against `filebrowser --help`):

```ini
[Unit]
Description=File Browser custom fork
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=filebrowser
Group=filebrowser
WorkingDirectory=/var/lib/filebrowser
ExecStart=/usr/local/bin/filebrowser --address 127.0.0.1 --port 8080 --root /srv/files --database /var/lib/filebrowser/filebrowser.db
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/filebrowser /srv/files

[Install]
WantedBy=multi-user.target
```

Create `filebrowser` user and writable directories first, then `sudo install -m 0755 ./filebrowser /usr/local/bin/filebrowser`, install the unit at `/etc/systemd/system/filebrowser.service`, `sudo systemctl daemon-reload && sudo systemctl enable --now filebrowser`. Do not create a second database on an existing deployment.

## Upgrade an existing installation

**Inventory:** check `systemctl cat filebrowser`, `systemctl show filebrowser -p Environment -p EnvironmentFiles`, your existing CLI flags or Docker Compose, `filebrowser config cat`, and the actual main Bolt database path. `filebrowser.db.uploads.db` contains durable transfers, jobs, sessions and office leases; its adjacent `.artifacts` directory holds prepared private downloads. Keep the main database, this sidecar, artifacts, config, credentials, mounted file data and staging/history together in a consistent backup. Let active jobs finish; stop the service for a cold backup.

**Native upgrade (generic, reviewed path examples — adapt before execution):**

```bash
# Set to paths discovered from your CURRENT unit, not guessed values.
DB=/var/lib/filebrowser/filebrowser.db
BIN=/usr/local/bin/filebrowser
BACKUP=/var/backups/filebrowser/$(date -u +%Y%m%dT%H%M%SZ)

sudo systemctl stop filebrowser
sudo install -d -m 0700 "$BACKUP"
sudo cp -a "$BIN" "$BACKUP/filebrowser.previous"
sudo cp -a "$DB" "$BACKUP/"
[ ! -f "${DB}.uploads.db" ] || sudo cp -a "${DB}.uploads.db" "$BACKUP/"
[ ! -d "${DB}.uploads.db.artifacts" ] || sudo cp -a "${DB}.uploads.db.artifacts" "$BACKUP/"
# ALSO back up all systemd config/env, source data tree and .filebrowser-staging
# to your regular filesystem backup before continuing.

# From the extracted or checked-out 3.3.1 source, after compiling as above:
sudo install -m 0755 ./filebrowser "$BIN"
sudo systemctl start filebrowser
sudo systemctl --no-pager --full status filebrowser
sudo journalctl -u filebrowser -n 100 --no-pager
"$BIN" version
```

Expected: `File Browser v3.3.1/...`. Hard-refresh the web UI; test login, folder listing, upload/resume, a version restore, a file download and enabled integrations with harmless test files. **Do not** blindly run `native-install/install_test.py` on your production host: it assumes `/root/filebrowser.db`, `/etc/filebrowser.json` and a specific native service.

**Docker upgrade:** back up existing bind mounts/volumes and all DB sidecars, keep unchanged mounts, settings and root directory, then build the new source image with `docker compose build filebrowser` and recreate only that service with `docker compose up -d --no-deps filebrowser`. **Never** replace an existing container with the sample volumes until paths match.

## Rollback

Stop the upgraded service, restore the previous binary **and all** database/journal/artifacts/config files from the same cold-backup snapshot, then restart. An older binary may not be compatible with a database altered by a newer version; restoring only the binary is unsafe. Restore the file tree if newer writes altered it, and review external Erugo/ConvertX actions separately (those cannot be rolled back by restoring the local DB). Validate with logs and a private smoke test.

## Source publishing

The generated `publish-3.3.1.sh` script provided beside the prepared source archive pushes to `release/3.3.1`, never force-pushes or overwrites `master`, and checks the expected remote HEAD before publishing. It requires `git` plus local GitHub authentication (credential manager, SSH key or `gh auth login`). Merge the proposed PR only after CI and a staging test.

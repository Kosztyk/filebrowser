#!/usr/bin/env bash
set -Eeuo pipefail

# File Browser v18.1 advanced deploy + GitHub sync
#
# Usage:
#   bash deploy-filebrowser-v18.1-and-github.sh [/path/to/filebrowser-source]
#
# Default source directory:
#   /root/filebrowser-master
#
# Optional environment overrides:
#   APP_DIR=/root/filebrowser-master
#   VERSION=2.63.23-advanced-v18.1
#   REPO_URL=https://github.com/Kosztyk/filebrowser.git
#   BRANCH=master
#   SERVICE=filebrowser
#   INSTALL_BIN=/usr/local/bin/filebrowser
#   COMMIT_MESSAGE='Update File Browser advanced build to v18.1'
#   GIT_NAME=Kosztyk
#   GIT_EMAIL=36381705+Kosztyk@users.noreply.github.com
#   GITHUB_BACKUP=1        # 1=create backup branch before updating master
#   FILEBROWSER_DB=/path/to/filebrowser.db   # optional database backup

APP_DIR="${1:-${APP_DIR:-/root/filebrowser-master}}"
VERSION="${VERSION:-2.63.23-advanced-v18.1}"
REPO_URL="${REPO_URL:-https://github.com/Kosztyk/filebrowser.git}"
BRANCH="${BRANCH:-master}"
SERVICE="${SERVICE:-filebrowser}"
INSTALL_BIN="${INSTALL_BIN:-/usr/local/bin/filebrowser}"
COMMIT_MESSAGE="${COMMIT_MESSAGE:-Update File Browser advanced build to v18.1}"
GIT_NAME="${GIT_NAME:-Kosztyk}"
GIT_EMAIL="${GIT_EMAIL:-36381705+Kosztyk@users.noreply.github.com}"
GITHUB_BACKUP="${GITHUB_BACKUP:-1}"
FILEBROWSER_DB="${FILEBROWSER_DB:-}"
STAMP="$(date +%Y%m%d-%H%M%S)"

if [[ $EUID -eq 0 ]]; then
  SUDO=()
else
  SUDO=(sudo)
fi

TMP_ROOT="$(mktemp -d /tmp/filebrowser-v18.1-deploy.XXXXXX)"
GITHUB_CLONE="$TMP_ROOT/github"
NEW_BINARY="$APP_DIR/filebrowser"
BACKUP_BINARY="${INSTALL_BIN}.backup-${STAMP}"
BACKUP_DB=""
LOCAL_DEPLOYED=0
OLD_BINARY_EXISTED=0

cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

log() {
  printf '\n\033[1;34m==> %s\033[0m\n' "$*"
}

fail() {
  printf '\n\033[1;31mERROR: %s\033[0m\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "Required command not found: $1"
}

version_ge() {
  # version_ge ACTUAL MINIMUM
  local actual="$1" minimum="$2"
  [[ "$(printf '%s\n%s\n' "$minimum" "$actual" | sort -V | head -n1)" == "$minimum" ]]
}

rollback_local() {
  if [[ "$LOCAL_DEPLOYED" -ne 1 ]]; then
    return 0
  fi

  printf '\n\033[1;33mRolling back local File Browser installation...\033[0m\n' >&2
  "${SUDO[@]}" systemctl stop "$SERVICE" || true

  if [[ "$OLD_BINARY_EXISTED" -eq 1 && -f "$BACKUP_BINARY" ]]; then
    "${SUDO[@]}" install -m 0755 "$BACKUP_BINARY" "$INSTALL_BIN"
    "${SUDO[@]}" systemctl start "$SERVICE" || true
  else
    "${SUDO[@]}" rm -f "$INSTALL_BIN"
  fi

  LOCAL_DEPLOYED=0
}

log "Checking required tools"
for c in go node pnpm git rsync systemctl sort; do
  need_cmd "$c"
done

if command -v gh >/dev/null 2>&1; then
  if gh auth status -h github.com >/dev/null 2>&1; then
    log "Using authenticated GitHub CLI credentials"
    gh auth setup-git >/dev/null 2>&1 || true
  fi
fi

APP_DIR="$(cd "$APP_DIR" 2>/dev/null && pwd)" || fail "Source directory does not exist: $APP_DIR"

log "Validating source tree: $APP_DIR"
for f in go.mod main.go cmd/cmd.go frontend/package.json frontend/pnpm-lock.yaml frontend/src/main.ts frontend/public/index.html; do
  [[ -f "$APP_DIR/$f" ]] || fail "Missing required source file: $f"
done

grep -qx 'module github.com/filebrowser/filebrowser/v2' "$APP_DIR/go.mod" \
  || fail "Unexpected Go module in $APP_DIR/go.mod"

grep -q 'src="../src/main.ts"' "$APP_DIR/frontend/public/index.html" \
  || fail "Expected v18.1 Vite production entry fix is missing from frontend/public/index.html"

# Custom-feature sanity checks. These are intentionally lightweight guardrails,
# not replacements for the build/tests below.
for f in \
  http/archive.go \
  http/clamav.go \
  http/collabora.go \
  http/convertx.go \
  http/erugo.go \
  http/wopi.go \
  settings/clamav.go \
  settings/convertx.go \
  settings/erugo.go \
  ERUGO_BANDWIDTH_LIMITS.md; do
  [[ -f "$APP_DIR/$f" ]] || fail "Custom feature file missing: $f"
done

GO_VERSION="$(go env GOVERSION | sed 's/^go//')"
NODE_VERSION="$(node --version | sed 's/^v//')"
PNPM_VERSION="$(pnpm --version)"

printf 'Go:   %s\nNode: %s\npnpm: %s\n' "$GO_VERSION" "$NODE_VERSION" "$PNPM_VERSION"
version_ge "$GO_VERSION" "1.25.0" || fail "Go >= 1.25.0 is required; found $GO_VERSION"
version_ge "$NODE_VERSION" "24.0.0" || fail "Node >= 24.0.0 is required; found $NODE_VERSION"
version_ge "$PNPM_VERSION" "10.0.0" || fail "pnpm >= 10.0.0 is required; found $PNPM_VERSION"

log "Checking GitHub repository access"
git clone --quiet --branch "$BRANCH" --single-branch "$REPO_URL" "$GITHUB_CLONE" \
  || fail "Could not clone $REPO_URL branch $BRANCH"

git -C "$GITHUB_CLONE" config user.name "$GIT_NAME"
git -C "$GITHUB_CLONE" config user.email "$GIT_EMAIL"

# A dry-run push checks that the local machine has usable push credentials
# before we spend time building or stop the service.
git -C "$GITHUB_CLONE" push --dry-run origin "HEAD:refs/heads/$BRANCH" >/dev/null \
  || fail "GitHub push authentication failed. Configure git/gh credentials and rerun."

ORIGINAL_GITHUB_HEAD="$(git -C "$GITHUB_CLONE" rev-parse HEAD)"
printf 'GitHub %s currently at %s\n' "$BRANCH" "$ORIGINAL_GITHUB_HEAD"

log "Building frontend"
cd "$APP_DIR/frontend"
rm -rf dist
pnpm install --frozen-lockfile
pnpm run build
[[ -f dist/public/index.html || -f dist/index.html ]] \
  || fail "Frontend build completed without an expected index.html in frontend/dist"

log "Running Go tests"
cd "$APP_DIR"
env GOWORK=off GO111MODULE=on GOTOOLCHAIN=auto go test ./...

log "Building File Browser $VERSION"
rm -f "$NEW_BINARY"
env GOWORK=off GO111MODULE=on GOTOOLCHAIN=auto \
  go build \
    -ldflags="-s -w -X github.com/filebrowser/filebrowser/v2/version.Version=$VERSION -X github.com/filebrowser/filebrowser/v2/version.CommitSHA=local" \
    -o "$NEW_BINARY" .

[[ -x "$NEW_BINARY" ]] || fail "Go build did not create $NEW_BINARY"

log "Verifying newly built binary"
"$NEW_BINARY" version

log "Preparing exact source update for GitHub"
# Mirror the audited source into a fresh clone of master. Build outputs, local
# databases and dependency directories are deliberately not committed.
rsync -a --delete \
  --exclude='.git/' \
  --exclude='node_modules/' \
  --exclude='frontend/node_modules/' \
  --exclude='frontend/dist/' \
  --exclude='/filebrowser' \
  --exclude='/filebrowser.exe' \
  --exclude='*.db' \
  --exclude='*.bak' \
  --exclude='.DS_Store' \
  --exclude='.env.local' \
  --exclude='.env.*.local' \
  "$APP_DIR/" "$GITHUB_CLONE/"

cd "$GITHUB_CLONE"
git add -A

GITHUB_HAS_CHANGES=1
if git diff --cached --quiet; then
  GITHUB_HAS_CHANGES=0
  echo "GitHub master already contains the same tracked source; no source commit is needed."
else
  echo
  echo "GitHub changes to be committed:"
  git status --short
  git commit -m "$COMMIT_MESSAGE"
  NEW_GITHUB_HEAD="$(git rev-parse HEAD)"
  printf 'Prepared commit: %s\n' "$NEW_GITHUB_HEAD"
fi

if [[ "$GITHUB_HAS_CHANGES" -eq 1 && "$GITHUB_BACKUP" == "1" ]]; then
  BACKUP_BRANCH="backup/pre-v18.1-${STAMP}"
  log "Creating GitHub backup branch: $BACKUP_BRANCH"
  git push --quiet origin "$ORIGINAL_GITHUB_HEAD:refs/heads/$BACKUP_BRANCH" \
    || fail "Could not create GitHub backup branch; local deployment has NOT started."
  echo "Created GitHub backup branch: $BACKUP_BRANCH"
fi

log "Backing up current local installation"
if [[ -f "$INSTALL_BIN" ]]; then
  OLD_BINARY_EXISTED=1
  "${SUDO[@]}" cp -a "$INSTALL_BIN" "$BACKUP_BINARY"
  echo "Binary backup: $BACKUP_BINARY"
fi

if [[ -n "$FILEBROWSER_DB" ]]; then
  if [[ -f "$FILEBROWSER_DB" ]]; then
    BACKUP_DB="${FILEBROWSER_DB}.backup-${STAMP}"
    "${SUDO[@]}" cp -a "$FILEBROWSER_DB" "$BACKUP_DB"
    echo "Database backup: $BACKUP_DB"
  else
    fail "FILEBROWSER_DB was specified but does not exist: $FILEBROWSER_DB"
  fi
fi

log "Deploying locally"
"${SUDO[@]}" systemctl stop "$SERVICE"
"${SUDO[@]}" install -m 0755 "$NEW_BINARY" "$INSTALL_BIN"
LOCAL_DEPLOYED=1

if ! "${SUDO[@]}" systemctl start "$SERVICE"; then
  rollback_local
  fail "The new File Browser service could not be started; previous binary restored."
fi

sleep 2
if ! "${SUDO[@]}" systemctl is-active --quiet "$SERVICE"; then
  "${SUDO[@]}" journalctl -u "$SERVICE" -n 100 --no-pager || true
  rollback_local
  fail "The new File Browser service did not remain active; previous binary restored."
fi

log "Verifying installed version"
"$INSTALL_BIN" version

if [[ "$GITHUB_HAS_CHANGES" -eq 1 ]]; then
  log "Updating GitHub $BRANCH"
  # Fetch immediately before push to avoid overwriting a concurrent update.
  git fetch --quiet origin "$BRANCH"
  REMOTE_NOW="$(git rev-parse "origin/$BRANCH")"
  if [[ "$REMOTE_NOW" != "$ORIGINAL_GITHUB_HEAD" ]]; then
    rollback_local
    fail "GitHub $BRANCH changed while this script was running ($ORIGINAL_GITHUB_HEAD -> $REMOTE_NOW). Local deployment was rolled back. Rerun against the new head."
  fi

  if ! git push origin "HEAD:refs/heads/$BRANCH"; then
    rollback_local
    fail "GitHub push failed. Local deployment was rolled back to keep local/GitHub versions consistent."
  fi

  FINAL_GITHUB_HEAD="$(git rev-parse HEAD)"
  echo "GitHub updated to: $FINAL_GITHUB_HEAD"
else
  echo "No GitHub push required."
fi

log "Final service status"
"${SUDO[@]}" systemctl --no-pager --full status "$SERVICE"

log "Recent File Browser logs"
"${SUDO[@]}" journalctl -u "$SERVICE" -n 60 --no-pager || true

printf '\n\033[1;32mSUCCESS\033[0m\n'
printf 'Local binary: %s\n' "$INSTALL_BIN"
printf 'Version:      %s\n' "$VERSION"
printf 'GitHub repo:  %s\n' "$REPO_URL"
printf 'GitHub branch:%s\n' "$BRANCH"
if [[ "$OLD_BINARY_EXISTED" -eq 1 ]]; then
  printf 'Rollback bin: %s\n' "$BACKUP_BINARY"
fi
if [[ -n "$BACKUP_DB" ]]; then
  printf 'Database bak: %s\n' "$BACKUP_DB"
fi

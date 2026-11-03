#!/usr/bin/env bash
# Non-destructive release branch push of the complete 3.3.1 SOURCE tree.
# Requirements: git + rsync + GitHub HTTPS or SSH credentials. Does NOT require gh.
set -euo pipefail
SOURCE_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_URL="${FILEBROWSER_GIT_REMOTE:-https://github.com/Kosztyk/filebrowser.git}"
BRANCH="${FILEBROWSER_RELEASE_BRANCH:-release/3.3.1}"
EXPECTED_MASTER="73fe38ea56003f22d2d02cd52083a8ce73a24bb5"

for c in git rsync mktemp; do command -v "$c" >/dev/null || { echo "ERROR: $c is required" >&2; exit 1; }; done
[ "$(tr -d '\r\n' < "$SOURCE_DIR/VERSION")" = 3.3.1 ] || { echo 'ERROR: expected VERSION=3.3.1' >&2; exit 1; }
[ -f "$SOURCE_DIR/go.mod" ] && [ -f "$SOURCE_DIR/frontend/package.json" ] || { echo 'ERROR: incomplete source directory' >&2; exit 1; }
[ -f "$SOURCE_DIR/docs/INTEGRATIONS_3.3.1.md" ] || { echo 'ERROR: missing prepared documentation' >&2; exit 1; }

TMP="$(mktemp -d "${TMPDIR:-/tmp}/filebrowser-release.XXXXXXXX")"
trap 'rm -rf -- "$TMP"' EXIT
printf 'Cloning %s into isolated temporary worktree...\n' "$REPO_URL"
git clone --quiet --no-single-branch "$REPO_URL" "$TMP/repo"
cd "$TMP/repo"
MASTER_HEAD="$(git rev-parse refs/remotes/origin/master)"
if [ "$MASTER_HEAD" != "$EXPECTED_MASTER" ] && [ "${ALLOW_MASTER_ADVANCE:-0}" != 1 ]; then
  printf 'ERROR: remote master changed since preparation (%s). Re-review changes before pushing.\n' "$MASTER_HEAD" >&2
  printf 'If reviewed, set ALLOW_MASTER_ADVANCE=1 and rerun.\n' >&2
  exit 1
fi
if git show-ref --verify --quiet "refs/remotes/origin/$BRANCH"; then
  git switch --quiet --create "$BRANCH" --track "origin/$BRANCH"
else
  git switch --quiet --create "$BRANCH" origin/master
fi
# Exact 3.3.1 source snapshot; never change master or any production directories.
rsync -a --delete \
  --exclude='/.git/' \
  --exclude='/.env' --exclude='/.env.*' \
  --exclude='/frontend/node_modules/' \
  --exclude='/frontend/dist/' \
  --exclude='/native-install/linux-amd64/filebrowser' \
  --exclude='/native-install/linux-arm64/filebrowser' \
  "$SOURCE_DIR/" ./
# Regenerate built frontend at build time; prebuilt native binaries only live in the ZIP distribution.
git add --all
git diff --cached --check
if git diff --cached --quiet; then
  printf 'No release changes to push. Remote branch is already current.\n'
  exit 0
fi
if ! git config user.name >/dev/null || ! git config user.email >/dev/null; then
  git config user.name "${GIT_AUTHOR_NAME:-Kosztyk}"
  git config user.email "${GIT_AUTHOR_EMAIL:-36381705+Kosztyk@users.noreply.github.com}"
fi
printf '\nProposed release changes:\n'
git diff --cached --stat | tail -n 25
printf '\nCreating release commit...\n'
git commit --quiet -m 'Release File Browser 3.3.1 advanced fork with complete source and documentation'
git push --set-upstream origin "HEAD:refs/heads/$BRANCH"
printf '\nPushed %s. Review pull request (do NOT merge without CI/staging tests):\n' "$BRANCH"
printf 'https://github.com/Kosztyk/filebrowser/compare/master...%s?expand=1\n' "$BRANCH"

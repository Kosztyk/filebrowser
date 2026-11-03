#!/usr/bin/env python3
"""Backup/restore Kosz's native systemd Filebrowser installation (Python 3.8+)."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile

SYSTEM_ROOT = Path("/")
SERVICE = "filebrowser.service"
DATABASE = "/root/filebrowser.db"
UPLOAD_JOURNAL = DATABASE + ".uploads.db"
DATA = "/mnt/data"
META = "filebrowser-migration-metadata"
FIXED = [DATABASE, "/etc/filebrowser.json", "/etc/filebrowser.env",
         "/etc/systemd/system/filebrowser.service", "/usr/local/bin/filebrowser"]


def fail(message):
    raise RuntimeError(message)


def local(logical):
    return SYSTEM_ROOT / logical.lstrip("/")


def sha256(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(4 * 1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def systemctl(*arguments, check=True):
    return subprocess.run(["systemctl", *arguments], check=check,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)


def active():
    return systemctl("is-active", "--quiet", SERVICE, check=False).returncode == 0


def normalized(path):
    if not isinstance(path, str) or not path.startswith("/"):
        fail("Backup paths must be absolute")
    if ".." in Path(path).parts or path == "/":
        fail("Refusing an unsafe or whole-system backup path: " + path)
    return "/" + str(Path(path)).lstrip("/")


def backup(output_dir, state_only=False, extras=()):
    config = json.loads(local("/etc/filebrowser.json").read_text())
    config = {key.lower(): value for key, value in config.items()}
    if config.get("database") != DATABASE or config.get("root") != DATA:
        fail("The configured paths have changed; expected /root/filebrowser.db and /mnt/data")
    for name in FIXED:
        if not local(name).is_file():
            fail("Required file missing: " + name)
    if local(DATABASE).stat().st_size == 0:
        fail("The database is empty")
    if not local(DATA).is_dir():
        fail("Served-files directory missing: " + DATA)

    # Read effective path overrides from the running process, without sourcing
    # the environment file or printing any credentials.
    runtime = {}
    pid = systemctl("show", "--property=MainPID", "--value", SERVICE, check=False).stdout.strip()
    if pid.isdigit() and int(pid) > 0:
        try:
            runtime = dict(entry.split(b"=", 1) for entry in
                           Path("/proc", pid, "environ").read_bytes().split(b"\0") if b"=" in entry)
        except FileNotFoundError:
            pass
    for key, expected in ((b"FB_DATABASE", DATABASE), (b"FB_ROOT", DATA)):
        if runtime.get(key) and runtime[key].decode() != expected:
            fail("The running service overrides a backup path; review " + key.decode())

    output = Path(output_dir).resolve()
    roots = list(FIXED)
    if not state_only:
        roots.append(DATA)
        if local(UPLOAD_JOURNAL).exists():
            if local(UPLOAD_JOURNAL).is_symlink() or not local(UPLOAD_JOURNAL).is_file():
                fail("Upload journal must be a regular file")
            roots.append(UPLOAD_JOURNAL)
            if local(UPLOAD_JOURNAL+".artifacts").exists():
                if local(UPLOAD_JOURNAL+".artifacts").is_symlink() or not local(UPLOAD_JOURNAL+".artifacts").is_dir():
                    fail("Invalid artifact directory")
                roots.append(UPLOAD_JOURNAL+".artifacts")
    dropins = "/etc/systemd/system/filebrowser.service.d"
    if local(dropins).exists():
        roots.append(dropins)
    # Include TLS files explicitly referenced by the JSON, when present.
    for key in ("cert", "key"):
        value = runtime.get(("FB_" + key.upper()).encode())
        value = value.decode() if value else config.get(key)
        if value:
            roots.append("/" + str(value).lstrip("/"))
    roots.extend(normalized(name) for name in extras)
    if state_only:
        journal=local(UPLOAD_JOURNAL).resolve()
        if any(local(name).resolve()==journal or local(name).resolve() in journal.parents for name in roots):
            fail("State-only backups must omit the upload journal so clones have independent sessions")
    # Preserve critical symlinks and their actual targets, including a symlinked
    # /mnt/data. Links within the served tree are preserved without following
    # external targets; add those targets with --extra if needed.
    for name in list(roots):
        item = local(name)
        if not item.exists():
            fail("Backup path missing: " + name)
        if item.is_symlink():
            target = item.resolve(strict=True)
            try:
                roots.append("/" + str(target.relative_to(SYSTEM_ROOT.resolve())))
            except ValueError:
                fail("A symlink target is outside the source filesystem: " + name)
    roots = list(dict.fromkeys(normalized(name) for name in roots))
    for name in roots:
        item = local(name).resolve()
        if item.is_dir() and (output == item or item in output.parents):
            fail("The backup destination must be outside " + name)

    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    # Use a conservative uncompressed estimate before stopping the service.
    # This avoids filling a small OS disk with a backup of a large data mount.
    sizes = subprocess.check_output([
        "du", "--summarize", "--apparent-size", "--block-size=1", "--",
        *[str(local(name)) for name in roots],
    ])
    estimate = sum(int(line.split(b"\t", 1)[0]) for line in sizes.splitlines())
    required = estimate + estimate // 20 + 64 * 1024 * 1024
    available = shutil.disk_usage(output).free
    if available < required:
        fail("Insufficient free space at " + str(output) +
             "; conservative requirement %.2f GiB, available %.2f GiB. Choose a larger backup disk or use --state-only." %
             (required / 1024 ** 3, available / 1024 ** 3))
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    mode = "state" if state_only else "full"
    archive = output / ("filebrowser-" + mode + "-" + stamp + ".tar.gz")
    partial = Path(str(archive) + ".partial")
    if archive.exists() or partial.exists():
        fail("A backup with this timestamp already exists; try again in a second")
    was_active = active()
    enabled = systemctl("is-enabled", SERVICE, check=False).stdout.strip()
    print("Backing up state only." if state_only else "Backing up application state and all files under /mnt/data.", flush=True)
    print("Filebrowser will be stopped while the archive is written.", flush=True)
    try:
        systemctl("stop", SERVICE)
        if active():
            fail("The service did not stop; no backup was created")
        with tempfile.TemporaryDirectory(prefix="filebrowser-backup-") as temporary:
            metadata_dir = Path(temporary) / META
            metadata_dir.mkdir(mode=0o700)
            manifest = {
                "format": 1, "created_utc": stamp, "system": platform.system(),
                "architecture": platform.machine(), "includes_data": not state_only,
                "database": DATABASE, "data_root": DATA, "archive_roots": roots,
                "service_enabled": enabled,
                "file_sha256": {name:sha256(local(name)) for name in roots if local(name).is_file()},
            }
            (metadata_dir / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
            subprocess.run([
                "tar", "--create", "--gzip", "--file", str(partial),
                "--acls", "--xattrs", "--numeric-owner", "--sparse",
                "--directory", temporary, META,
                "--directory", str(SYSTEM_ROOT), "--",
                *[name.lstrip("/") for name in roots],
            ], check=True)
        partial.replace(archive)
    finally:
        if was_active:
            systemctl("start", SERVICE)
            if not active():
                fail("Backup finished or failed, but the original service could not be restarted; check journalctl -u filebrowser")
            print("Original Filebrowser service restarted.", flush=True)
    # These checks happen after restarting production, avoiding extra downtime.
    subprocess.run(["tar", "--list", "--gzip", "--file", str(archive)],
                   check=True, stdout=subprocess.DEVNULL)
    sidecar = Path(str(archive) + ".sha256")
    sidecar.write_text(sha256(archive) + "  " + archive.name + "\n")
    (output / "latest-backup.txt").write_text(str(archive) + "\n")
    print("Verified backup: " + str(archive))
    print("Checksum file: " + str(sidecar))
    if state_only:
        print("File contents are NOT included. Copy test files separately into /mnt/data on the new machine.")
    return archive


def read_manifest(archive):
    with tarfile.open(archive, mode="r|gz") as stream:
        for entry in stream:
            if entry.name == META + "/manifest.json":
                if not entry.isfile() or entry.size > 1024 * 1024:
                    fail("Invalid backup manifest")
                return json.load(stream.extractfile(entry))
            if not entry.name.startswith(META):
                break
    fail("This is not a backup made by this tool")


def restore(archive, binary=None, keep_existing_data=False):
    archive = Path(archive).resolve(strict=True)
    sidecar = Path(str(archive) + ".sha256")
    if not sidecar.is_file():
        fail("Copy the .sha256 file alongside the archive before restoring")
    expected = sidecar.read_text().split()[0]
    if not re.fullmatch(r"[0-9a-f]{64}", expected) or sha256(archive) != expected:
        fail("Archive checksum does not match; nothing was restored")
    manifest = read_manifest(archive)
    if manifest.get("format") != 1 or manifest.get("database") != DATABASE or manifest.get("data_root") != DATA:
        fail("Unexpected backup format or paths")
    if manifest.get("system") != platform.system():
        fail("Restore this backup onto another Linux machine")
    replacement = Path(binary).resolve(strict=True) if binary else None
    if replacement is None and manifest.get("architecture") != platform.machine():
        fail("The saved executable is for another CPU architecture. Build the upgraded binary for this machine and use --binary /path/to/filebrowser")
    if replacement and (not replacement.is_file() or not os.access(replacement, os.X_OK)):
        fail("The replacement executable must be an executable regular file")
    roots = [normalized(name) for name in manifest.get("archive_roots", [])]
    if not all(name in roots for name in FIXED):
        fail("The backup is missing required application files")
    protected_data = local(DATA).resolve()

    def overlaps_data(path):
        resolved = path.resolve()
        return (resolved == protected_data or protected_data in resolved.parents or
                resolved in protected_data.parents)

    if keep_existing_data:
        if manifest.get("includes_data") is not False:
            fail("--keep-existing-data requires a state-only backup")
        if local(DATA).is_symlink() or not local(DATA).is_dir():
            fail("--keep-existing-data requires an existing directory at " + DATA)
        if any(overlaps_data(local(name)) for name in roots):
            fail("The archive records paths that could change existing data; nothing was restored")
    if keep_existing_data and any(local(name).resolve()==local(UPLOAD_JOURNAL).resolve() or local(name).resolve() in local(UPLOAD_JOURNAL).resolve().parents for name in roots):
        fail("A state-only clone must not restore another instance's upload journal")
    check_roots=list(dict.fromkeys((roots if keep_existing_data else roots+[DATA])+[UPLOAD_JOURNAL]))
    for name in check_roots:
        destination = local(name)
        if destination.is_symlink() or (destination.exists() and
                (not destination.is_dir() or any(destination.iterdir()))):
            hint = ""
            if name == DATA and not destination.is_symlink() and destination.is_dir() and manifest.get("includes_data") is False:
                hint = "; use --keep-existing-data with this state-only backup to preserve the existing directory and its mounts"
            fail("Refusing to overwrite an existing installation or data at " + name + hint)
    # Validate member names/links before any extraction. The archive checksum
    # and this allowlist also catch accidental use of a different tar archive.
    allowed = [name.lstrip("/") for name in roots] + [META]
    with tarfile.open(archive, mode="r|gz") as stream:
        for entry in stream:
            name = entry.name.rstrip("/")
            if name.startswith("/") or ".." in Path(name).parts:
                fail("Unsafe archive member")
            if not any(name == base or name.startswith(base + "/") for base in allowed):
                fail("An archive member falls outside the recorded backup paths")
            if keep_existing_data and (entry.issym() or entry.islnk()):
                target = Path(entry.linkname)
                if entry.issym() and not target.is_absolute():
                    target = Path(name).parent / target
                if overlaps_data(local(str(target))):
                    fail("An archive link could change existing data; nothing was restored")
    subprocess.run([
        "tar", "--extract", "--gzip", "--file", str(archive),
        "--directory", str(SYSTEM_ROOT), "--same-owner", "--same-permissions",
        "--numeric-owner", "--acls", "--xattrs", "--exclude=" + META,
    ], check=True)
    for name, digest in manifest["file_sha256"].items():
        if sha256(local(normalized(name))) != digest:
            fail("Restored file verification failed: " + name)
    if replacement:
        destination = local("/usr/local/bin/filebrowser")
        # Replace the canonical executable itself, even if the backup used a symlink.
        with tempfile.NamedTemporaryFile(dir=destination.parent, delete=False) as output:
            temporary_binary = Path(output.name)
            with replacement.open("rb") as input_file:
                shutil.copyfileobj(input_file, output)
        temporary_binary.chmod(0o755)
        temporary_binary.replace(destination)
    if not keep_existing_data:
        local(DATA).mkdir(parents=True, exist_ok=True)
    systemctl("daemon-reload")
    print("Restore verified. Database, configuration, environment and executable are in their original paths.")
    if keep_existing_data:
        print("Existing /mnt/data and its mounted folders were kept unchanged.")
    elif not manifest.get("includes_data"):
        print("This was a state-only backup; /mnt/data needs independent test files.")
    print("The service has not been started. On this TEST machine, run:")
    print("  systemctl enable --now filebrowser")
    print("  systemctl status filebrowser --no-pager")
    print("Review copied external-service URLs before exercising integrations.")


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    b = commands.add_parser("backup")
    b.add_argument("output_dir", nargs="?", default="/var/backups/filebrowser")
    b.add_argument("--state-only", action="store_true", help="omit /mnt/data contents")
    b.add_argument("--extra", action="append", default=[], metavar="ABSOLUTE_PATH")
    r = commands.add_parser("restore")
    r.add_argument("archive")
    r.add_argument("--binary", help="use an already-built executable for the new machine")
    r.add_argument("--keep-existing-data", action="store_true",
                   help="restore state only while preserving existing /mnt/data and its mounts")
    args = parser.parse_args()
    if os.geteuid() != 0:
        fail("Run this command as root or with sudo")
    for program in ("tar", "du", "systemctl"):
        if not shutil.which(program):
            fail("Required command is missing: " + program)
    if args.command == "backup":
        backup(args.output_dir, args.state_only, args.extra)
    else:
        restore(args.archive, args.binary, keep_existing_data=args.keep_existing_data)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, subprocess.CalledProcessError, tarfile.TarError) as error:
        print("ERROR: " + str(error), file=sys.stderr)
        sys.exit(1)

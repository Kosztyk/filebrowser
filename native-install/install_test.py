#!/usr/bin/env python3
"""Install the packaged Filebrowser 3.3.1 on the NEW native Linux test machine."""
import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile

import filebrowser_migrate as migration

PACKAGE = Path(__file__).resolve().parent


def selected_binary():
    if platform.system() != "Linux":
        raise RuntimeError("Use this installer on the new Linux machine")
    arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine().lower())
    if not arch:
        raise RuntimeError("Packaged executables support Linux amd64 and arm64; build from source for this CPU")
    name = "linux-" + arch + "/filebrowser"
    binary = PACKAGE / name
    manifest = json.loads((PACKAGE / "binaries.json").read_text())
    if manifest.get("version") != "3.3.1" or migration.sha256(binary) != manifest["binaries"].get(name):
        raise RuntimeError("The packaged executable checksum is invalid")
    # Python's ZIP extractor does not restore executable mode automatically.
    binary.chmod(0o755)
    version = subprocess.check_output([str(binary), "version"], text=True).strip()
    if "v3.3.1/" not in version:
        raise RuntimeError("The packaged executable has an unexpected version")
    print("Packaged executable: " + version, flush=True)
    return binary


def install_binary(source):
    destination = migration.local("/usr/local/bin/filebrowser")
    with tempfile.NamedTemporaryFile(dir=destination.parent, delete=False) as temporary:
        staged = Path(temporary.name)
        with source.open("rb") as original:
            shutil.copyfileobj(original, temporary)
        temporary.flush()
        os.fsync(temporary.fileno())
    staged.chmod(0o755)
    staged.replace(destination)


def upgrade_existing(binary):
    for name in migration.FIXED:
        if not migration.local(name).is_file():
            raise RuntimeError("Restore your backup first; missing " + name)
    if migration.local(migration.DATABASE).stat().st_size == 0:
        raise RuntimeError("The database is empty; restore it before upgrading")
    directory = migration.local("/var/backups/filebrowser-upgrade")
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    rollback = Path(tempfile.mkdtemp(prefix="before-3.3.1-", dir=directory))
    was_active = migration.active()
    saved = False
    replacing = False
    try:
        migration.systemctl("stop", migration.SERVICE)
        if migration.active():
            raise RuntimeError("Filebrowser did not stop")
        for name in migration.FIXED:
            output = rollback / name.lstrip("/")
            output.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(migration.local(name), output)
        journal=migration.local(migration.UPLOAD_JOURNAL)
        if journal.exists():
            if journal.is_symlink() or not journal.is_file():
                raise RuntimeError("Upload journal must be a regular file")
            shutil.copy2(journal,rollback/"root/filebrowser.db.uploads.db")
        saved = True
        replacing = True
        install_binary(binary)
        if was_active:
            migration.systemctl("start", migration.SERVICE)
            if not migration.active():
                raise RuntimeError("The upgraded service did not start")
    except BaseException:
        if saved and replacing:
            migration.systemctl("stop", migration.SERVICE)
            shutil.copy2(rollback / "root/filebrowser.db", migration.local(migration.DATABASE))
            journal=migration.local(migration.UPLOAD_JOURNAL)
            saved_journal=rollback/"root/filebrowser.db.uploads.db"
            if saved_journal.exists():
                shutil.copy2(saved_journal,journal)
            elif journal.exists():
                journal.unlink()
            install_binary(rollback / "usr/local/bin/filebrowser")
        if was_active:
            migration.systemctl("start", migration.SERVICE)
        raise
    print("Upgraded executable installed. Rollback copies: " + str(rollback))
    if not was_active:
        print("The service remains stopped. Start it with systemctl start filebrowser when ready.")


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    restore = commands.add_parser("restore", help="restore an old backup and install 3.3.1 before first start")
    restore.add_argument("backup_archive")
    restore.add_argument("--keep-existing-data", action="store_true",
                         help="state-only backup: leave existing /mnt/data and mounted folders unchanged")
    commands.add_parser("upgrade", help="upgrade a backup already restored on this TEST machine")
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise RuntimeError("Run as root or with sudo on the new TEST machine")
    binary = selected_binary()
    if args.command == "restore":
        migration.restore(args.backup_archive, str(binary), keep_existing_data=args.keep_existing_data)
        print("Filebrowser 3.3.1 is installed at /usr/local/bin/filebrowser.")
        print("The restored original executable was replaced before the service was started.")
    else:
        upgrade_existing(binary)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, subprocess.CalledProcessError) as error:
        print("ERROR: " + str(error), file=sys.stderr)
        sys.exit(1)

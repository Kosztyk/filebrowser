> **Test-host-only installer.** This helper assumes a specific restored Linux test machine. For fresh installation and generic production upgrades see [docs/INSTALL_3.3.1.md](docs/INSTALL_3.3.1.md).

# Upgrade your working test machine to Filebrowser 3.3.1

Your 3.1.0, 3.2.0 or 3.3.0 native test installation is already working. Run this upgrade on **the staging host with an unprivileged user** after copying FileBrowser_3.3.1_GitHub_Ready.zip into `$HOME/filebrowser-migration`. Let existing transfers finish first.

```bash
sudo -v
cd "$HOME/filebrowser-migration" &&
python3 -m zipfile -e FileBrowser_3.3.1_GitHub_Ready.zip . &&
cd filebrowser-3.3.1 &&
sudo bash install-native-test.sh upgrade &&
/usr/local/bin/filebrowser version &&
sudo systemctl status filebrowser --no-pager
```

Expect `File Browser v3.3.1/directory-fix`. The helper keeps the current configuration, environment, systemd overrides and mounted data. It restarts an already running service and leaves a stopped service stopped. If you intentionally had it stopped, start it with `sudo systemctl start filebrowser`. Reload the browser after the upgrade. This patch restores the original directory layout and actions; see DIRECTORY_UI_RELEASE.md.

The helper saves rollback copies under `/var/backups/filebrowser-upgrade/before-3.3.1-*`, including the main database, previous executable and an existing upload journal. It handles rollback on immediate startup failure. Check login and a transfer in a dedicated test folder after installation. There is no need to repeat the restore for this already installed machine.

Your TCP listener override and `/etc/filebrowser-test.json` remain in use. The upgrade does not rename, remove, unmount or replace `/mnt/data`. If mounts are shared with production, changes inside them affect the same underlying files; use dedicated test files.

## Fresh-machine restore reference

# Restore your backup directly into Filebrowser 3.3.1

Use this procedure on the **new native Linux test machine**. This archive now contains ready-built Linux amd64 and arm64 executables, the restore helper, and an installer. It also retains the full source. Go, Node.js, Docker, and frontend builds are not required for this installation route. Linux, Python 3.8+, GNU tar, and systemd are required.

You need these three files on the new machine:

1. This updated `FileBrowser_3.3.1_GitHub_Ready.zip`.
2. The `filebrowser-full-TIMESTAMP.tar.gz` or `filebrowser-state-TIMESTAMP.tar.gz` made from your old machine.
3. Its matching `.tar.gz.sha256` file, in the same directory as the backup.

The old-machine backup was created using `Filebrowser_Bare_Metal_Backup_Restore.zip`. That toolkit creates the backup; this upgraded application archive installs the new version and restores the old data. If you have not made the backup, use that toolkit first. A state-only backup retains users, credentials and settings but omits the file contents.

## Fresh test machine

Log in as `kosz`, copy the three files into `$HOME/filebrowser-migration`, and replace the backup filename below with the actual one you created. The account needs sudo access:

```bash
cd "$HOME/filebrowser-migration"
python3 -m zipfile -e FileBrowser_3.3.1_GitHub_Ready.zip .
cd filebrowser-3.3.1

sudo bash install-native-test.sh restore "$HOME/filebrowser-migration/filebrowser-full-TIMESTAMP.tar.gz"
```

The installer selects and verifies the executable for this CPU, verifies the backup, restores your old database/configuration/environment/service/files, and installs **3.3.1** at `/usr/local/bin/filebrowser`. It refuses to overwrite an existing installation. It does not start the old executable or create a new database.

## State-only backup with existing files or mounted folders

If `/mnt/data` already contains files or mounted subdirectories on the new test server, leave them in place. Restore the state-only backup with:

```bash
cd "$HOME/filebrowser-migration/filebrowser-3.3.1"
sudo bash install-native-test.sh restore \
  "$HOME/filebrowser-migration/filebrowser-state-TIMESTAMP.tar.gz" \
  --keep-existing-data
```

This explicit option restores the database, settings, service and upgraded executable while preserving `/mnt/data`, its contents, permissions and mounts. The service remains stopped. It requires a state-only manifest and rejects full backups, recorded paths overlapping the existing data, and archive links into that data before extraction. Existing application files are still protected against overwrite. Do not rename or unmount the existing data tree. Mounted storage can also be used by the original server; use independent test files when testing writes or deletions.

## Restored application paths

The restored paths are:

```text
/usr/local/bin/filebrowser                         upgraded 3.3.1 executable
/root/filebrowser.db                              existing users/settings/shares
/etc/filebrowser.json                             existing configuration
/etc/filebrowser.env                              existing credentials/environment
/etc/systemd/system/filebrowser.service           existing native service
/mnt/data                                        restored files, for a full backup
```

Verify the installed version:

```bash
/usr/local/bin/filebrowser version
```

It must report `v3.3.1/directory-fix`. Check `/etc/filebrowser.json` for a specific listen address tied to the old machine; adjust that address on this test host if necessary. Review copied external-service URLs before exercising integrations; use independent files and an independent upload journal.

Start the new version:

```bash
sudo systemctl enable --now filebrowser
sudo systemctl is-active filebrowser
sudo journalctl -u filebrowser -n 50 --no-pager
```

Open the new machine's IP using the original configured port/base path. Your existing account credentials come from the restored database. With a state-only backup, add independent sample files to `/mnt/data` before testing file operations. For a large full backup, checksum verification and extraction can take considerable time.

## If you already restored the backup on the test machine

Extract the updated application archive there, then run as `kosz`:

```bash
cd "$HOME/filebrowser-migration/filebrowser-3.3.1"
sudo bash install-native-test.sh upgrade
/usr/local/bin/filebrowser version
sudo systemctl is-active filebrowser
```

This command stops the test service, saves the current database/executable/configuration/environment/unit under `/var/backups/filebrowser-upgrade/before-3.3.1-*`, and replaces the executable with 3.3.1. It restarts a previously running test service and leaves a previously stopped service stopped. If replacement or the immediate startup check fails, it attempts to restore the saved binary/database and restart the previous version. The helper checks service state; inspect the journal and test login to verify application health. Rollback copies do not undo file operations or external-service actions.

Run these install commands only on the new test machine. Your original machine is used to create the backup and can keep running its original installation.

## Validation

The packaged amd64 executable was tested against a temporary database and production embedded frontend with browser login, listing, TUS resume, ranged download, and a 64 MiB checksum-verified transfer. The arm64 executable was cross-compiled and its ELF architecture verified; it was not executed on ARM hardware. Native installer tests use isolated filesystem fixtures and simulated systemd calls. Results and binary hashes are in `validation/results/native-install/` and `native-install/binaries.json`.

The upload journal also contains jobs, sessions, office locks and diagnostics in 3.3.1. Full cold backups include its `.artifacts` directory. State-only clones omit both. See FULL_ENHANCEMENTS_RELEASE.md for defaults, API compatibility, recovery and the new UI controls.

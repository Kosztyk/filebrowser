import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import tarfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import install_test as installer
import filebrowser_migrate as migration


class NativeInstallerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        for name in migration.FIXED:
            p = self.root / name.lstrip("/")
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_bytes(("original:" + name).encode())
        (self.root / "etc/filebrowser.json").write_text(json.dumps({"database": migration.DATABASE, "root": migration.DATA}))
        (self.root / "mnt/data").mkdir(parents=True)
        (self.root / "mnt/data/sample.txt").write_text("existing file")
        self.new = self.root / "new-filebrowser"
        self.new.write_bytes(b"new executable fixture")
        self.new.chmod(0o755)
        self.running = True
        self.calls = []

    def control(self, *arguments, **kwargs):
        self.calls.append(arguments)
        if arguments[0] == "stop":
            self.running = False
        elif arguments[0] == "start":
            self.running = True
        return subprocess.CompletedProcess(arguments, 0, "", "")

    def upgrade(self):
        with patch.object(migration, "SYSTEM_ROOT", self.root), \
                patch.object(migration, "systemctl", self.control), \
                patch.object(migration, "active", lambda: self.running), \
                contextlib.redirect_stdout(io.StringIO()):
            installer.upgrade_existing(self.new)

    def test_upgrade_preserves_database_and_restarts_running_test_service(self):
        original = (self.root / "root/filebrowser.db").read_bytes()
        self.upgrade()
        self.assertEqual((self.root / "usr/local/bin/filebrowser").read_bytes(), self.new.read_bytes())
        self.assertEqual((self.root / "root/filebrowser.db").read_bytes(), original)
        self.assertTrue(self.running)
        rollback = next((self.root / "var/backups/filebrowser-upgrade").iterdir())
        self.assertEqual((rollback / "root/filebrowser.db").read_bytes(), original)
        self.assertEqual((rollback / "usr/local/bin/filebrowser").read_bytes(), b"original:/usr/local/bin/filebrowser")

    def test_stopped_service_remains_stopped(self):
        self.running = False
        self.upgrade()
        self.assertFalse(self.running)

    def test_replacement_failure_restarts_original(self):
        with patch.object(installer, "install_binary", side_effect=[OSError("simulated failure"), None]):
            with self.assertRaises(OSError):
                self.upgrade()
        self.assertTrue(self.running)
        self.assertEqual((self.root / "usr/local/bin/filebrowser").read_bytes(), b"original:/usr/local/bin/filebrowser")

    def test_fresh_restore_uses_the_new_packaged_executable(self):
        with patch.object(migration, "SYSTEM_ROOT", self.root), \
                patch.object(migration, "systemctl", self.control), \
                patch.object(migration, "active", lambda: self.running), \
                contextlib.redirect_stdout(io.StringIO()):
            archive = migration.backup(self.root / "archives")
        target = self.root / "restored"
        target.mkdir()
        self.calls.clear()
        with patch.object(installer, "selected_binary", return_value=self.new), \
                patch.object(sys, "argv", ["install_test.py", "restore", str(archive)]), \
                patch.object(migration, "SYSTEM_ROOT", target), \
                patch.object(migration, "systemctl", self.control), \
                patch.object(os, "geteuid", return_value=0), \
                contextlib.redirect_stdout(io.StringIO()):
            installer.main()
        self.assertEqual((target / "usr/local/bin/filebrowser").read_bytes(), self.new.read_bytes())
        self.assertEqual((target / "root/filebrowser.db").read_bytes(), b"original:/root/filebrowser.db")
        self.assertEqual((target / "mnt/data/sample.txt").read_text(), "existing file")
        self.assertNotIn(("start", migration.SERVICE), self.calls)

    def test_failed_start_restores_the_previous_binary_and_database(self):
        original_db = (self.root / "root/filebrowser.db").read_bytes()
        starts = []
        def fail_first_start(*arguments, **kwargs):
            if arguments[0] == "start" and not starts:
                starts.append(True)
                (self.root / "root/filebrowser.db").write_bytes(b"changed by failed new startup")
                raise subprocess.CalledProcessError(1, ["systemctl", "start"])
            return self.control(*arguments, **kwargs)
        with patch.object(migration, "SYSTEM_ROOT", self.root), \
                patch.object(migration, "systemctl", fail_first_start), \
                patch.object(migration, "active", lambda: self.running), \
                contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaises(subprocess.CalledProcessError):
                installer.upgrade_existing(self.new)
        self.assertEqual((self.root / "usr/local/bin/filebrowser").read_bytes(), b"original:/usr/local/bin/filebrowser")
        self.assertEqual((self.root / "root/filebrowser.db").read_bytes(), original_db)
        self.assertTrue(self.running)

    def state_archive(self, state_only=True, extras=()):
        with patch.object(migration, "SYSTEM_ROOT", self.root), \
                patch.object(migration, "systemctl", self.control), \
                patch.object(migration, "active", lambda: self.running), \
                contextlib.redirect_stdout(io.StringIO()):
            return migration.backup(self.root / "archives", state_only=state_only, extras=extras)

    def populated_target(self):
        target = self.root / "restored"
        data = target / "mnt/data"
        (data / "mounted-folder").mkdir(parents=True)
        (data / "mounted-folder/keep.txt").write_bytes(b"existing mounted data")
        data.chmod(0o751)
        return target

    def restore_archive(self, archive, target, keep=True):
        arguments = ["install_test.py", "restore", str(archive)]
        if keep:
            arguments.append("--keep-existing-data")
        with patch.object(installer, "selected_binary", return_value=self.new), \
                patch.object(sys, "argv", arguments), \
                patch.object(migration, "SYSTEM_ROOT", target), \
                patch.object(migration, "systemctl", self.control), \
                patch.object(os, "geteuid", return_value=0), \
                contextlib.redirect_stdout(io.StringIO()):
            installer.main()

    def rewrite_archive(self, archive, edit_manifest, extra=None):
        with tarfile.open(archive, "r:gz") as stream:
            members = [(member, stream.extractfile(member).read() if member.isfile() else None)
                       for member in stream]
        staged = archive.with_suffix(".rewritten")
        with tarfile.open(staged, "w:gz") as stream:
            for member, contents in members:
                if member.name == migration.META + "/manifest.json":
                    manifest = json.loads(contents)
                    edit_manifest(manifest)
                    contents = json.dumps(manifest).encode()
                    member.size = len(contents)
                stream.addfile(member, io.BytesIO(contents) if contents is not None else None)
            if extra is not None:
                stream.addfile(extra)
        staged.replace(archive)
        Path(str(archive) + ".sha256").write_text(migration.sha256(archive) + "  " + archive.name + "\n")

    def test_keep_existing_state_restore_never_enumerates_or_creates_data(self):
        archive = self.state_archive()
        target = self.populated_target()
        data = target / "mnt/data"
        before = data.stat()
        mkdir, iterdir = Path.mkdir, Path.iterdir
        def guarded_mkdir(path, *args, **kwargs):
            if path == data:
                raise AssertionError("Existing data mkdir must not be called")
            return mkdir(path, *args, **kwargs)
        def guarded_iterdir(path):
            if path == data:
                raise AssertionError("Existing mounted data must not be enumerated")
            return iterdir(path)
        self.calls.clear()
        with patch.object(Path, "mkdir", guarded_mkdir), patch.object(Path, "iterdir", guarded_iterdir):
            self.restore_archive(archive, target)
        after = data.stat()
        for attribute in ("st_ino", "st_mode", "st_mtime_ns", "st_ctime_ns"):
            self.assertEqual(getattr(before, attribute), getattr(after, attribute))
        self.assertEqual((data / "mounted-folder/keep.txt").read_bytes(), b"existing mounted data")
        self.assertEqual((target / "root/filebrowser.db").read_bytes(), b"original:/root/filebrowser.db")
        self.assertEqual((target / "usr/local/bin/filebrowser").read_bytes(), self.new.read_bytes())
        self.assertEqual(self.calls, [("daemon-reload",)])

    def test_state_restore_still_refuses_existing_data_without_explicit_flag(self):
        archive = self.state_archive()
        target = self.populated_target()
        with self.assertRaisesRegex(RuntimeError, "Refusing to overwrite"):
            self.restore_archive(archive, target, keep=False)
        self.assertFalse((target / "root/filebrowser.db").exists())

    def test_keep_existing_refuses_full_backup_before_extraction(self):
        archive = self.state_archive(state_only=False)
        target = self.populated_target()
        with self.assertRaisesRegex(RuntimeError, "requires a state-only backup"):
            self.restore_archive(archive, target)
        self.assertFalse((target / "root/filebrowser.db").exists())

    def test_keep_existing_refuses_existing_application_database(self):
        archive = self.state_archive()
        target = self.populated_target()
        database = target / "root/filebrowser.db"
        database.parent.mkdir()
        database.write_bytes(b"do not overwrite")
        with self.assertRaisesRegex(RuntimeError, "Refusing to overwrite"):
            self.restore_archive(archive, target)
        self.assertEqual(database.read_bytes(), b"do not overwrite")

    def test_keep_existing_refuses_checksum_failure_before_extraction(self):
        archive = self.state_archive()
        target = self.populated_target()
        Path(str(archive) + ".sha256").write_text("0" * 64 + "  " + archive.name)
        with self.assertRaisesRegex(RuntimeError, "checksum does not match"):
            self.restore_archive(archive, target)
        self.assertFalse((target / "root/filebrowser.db").exists())

    def test_keep_existing_refuses_data_subtree_and_ancestor_archive_roots(self):
        archive = self.state_archive()
        target = self.populated_target()
        original = migration.read_manifest(archive)
        for root in ("/mnt/data", "/mnt/data/nested", "/mnt"):
            manifest = dict(original, archive_roots=original["archive_roots"] + [root])
            with self.subTest(root=root), patch.object(migration, "read_manifest", return_value=manifest):
                with self.assertRaisesRegex(RuntimeError, "could change existing data"):
                    self.restore_archive(archive, target)
            self.assertFalse((target / "root/filebrowser.db").exists())

    def test_keep_existing_refuses_symbolic_data_root(self):
        archive = self.state_archive()
        target = self.root / "restored"
        (target / "mnt").mkdir(parents=True)
        (target / "mnt/data").symlink_to(self.root / "mnt/data", target_is_directory=True)
        with self.assertRaisesRegex(RuntimeError, "requires an existing directory"):
            self.restore_archive(archive, target)
        self.assertFalse((target / "root/filebrowser.db").exists())

    def check_data_link_rejected(self, link_type, link_target):
        archive = self.state_archive()
        target = self.populated_target()
        member = tarfile.TarInfo("etc/test-data-link")
        member.type, member.linkname = link_type, link_target
        self.rewrite_archive(archive, lambda manifest: manifest["archive_roots"].append("/etc/test-data-link"), member)
        with self.assertRaisesRegex(RuntimeError, "archive link could change existing data"):
            self.restore_archive(archive, target)
        self.assertFalse((target / "root/filebrowser.db").exists())

    def test_keep_existing_refuses_archive_symlink_into_data(self):
        self.check_data_link_rejected(tarfile.SYMTYPE, "../mnt/data/mounted-folder/keep.txt")

    def test_keep_existing_refuses_archive_hardlink_into_data(self):
        self.check_data_link_rejected(tarfile.LNKTYPE, "mnt/data/mounted-folder/keep.txt")


if __name__ == "__main__":
    unittest.main(verbosity=2)

class UploadJournalTests(NativeInstallerTests):
    def test_full_backup_restores_journal_and_state_clone_omits_it(self):
        journal=self.root/migration.UPLOAD_JOURNAL.lstrip("/")
        journal.write_bytes(b"saved journal")
        state=self.state_archive(state_only=True)
        self.assertNotIn(migration.UPLOAD_JOURNAL,migration.read_manifest(state)["archive_roots"])
        full=self.state_archive(state_only=False)
        self.assertIn(migration.UPLOAD_JOURNAL,migration.read_manifest(full)["archive_roots"])
        target=self.root/"full-restore";target.mkdir()
        self.restore_archive(full,target,keep=False)
        self.assertEqual((target/migration.UPLOAD_JOURNAL.lstrip("/")).read_bytes(),b"saved journal")

    def test_upgrade_rolls_back_journal_or_removes_new_journal(self):
        journal=self.root/migration.UPLOAD_JOURNAL.lstrip("/");journal.write_bytes(b"old journal")
        self.upgrade()
        rollback=next((self.root/"var/backups/filebrowser-upgrade").iterdir())
        self.assertEqual((rollback/migration.UPLOAD_JOURNAL.lstrip("/")).read_bytes(),b"old journal")
        starts=[]
        def fail_start(*args,**kwargs):
            if args[0]=="start" and not starts:
                starts.append(True);journal.write_bytes(b"changed journal")
                raise subprocess.CalledProcessError(1,["systemctl","start"])
            return self.control(*args,**kwargs)
        with patch.object(migration,"SYSTEM_ROOT",self.root),patch.object(migration,"systemctl",fail_start),patch.object(migration,"active",lambda:self.running):
            with self.assertRaises(subprocess.CalledProcessError):installer.upgrade_existing(self.new)
        self.assertEqual(journal.read_bytes(),b"old journal")
        journal.unlink();starts.clear()
        with patch.object(migration,"SYSTEM_ROOT",self.root),patch.object(migration,"systemctl",fail_start),patch.object(migration,"active",lambda:self.running):
            with self.assertRaises(subprocess.CalledProcessError):installer.upgrade_existing(self.new)
        self.assertFalse(journal.exists())

if __name__ == "__main__":
    unittest.main()

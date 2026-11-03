#!/usr/bin/env python3
"""Exercise the built binary with an isolated database and a 64 MiB TUS upload.

Run from the repository root: python3 validation/smoke-transfer.py
No existing application data or configuration is used.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.parse
import urllib.request


def main():
    repository = Path(__file__).resolve().parents[1]
    default_binary = repository / ("filebrowser.exe" if os.name == "nt" else "filebrowser")
    binary = Path(os.environ.get("FILEBROWSER_TEST_BINARY", str(default_binary)))
    initializer = Path(os.environ.get("FILEBROWSER_BASELINE_BINARY", str(binary)))
    with tempfile.TemporaryDirectory(prefix="filebrowser-upgrade-") as temporary:
        temporary = Path(temporary)
        root = temporary / "files"
        root.mkdir()
        database = temporary / "test.db"
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        base = f"http://127.0.0.1:{port}"
        username, password = "upgrade-test", secrets.token_urlsafe(24)
        for args in [
            ["config", "init", "--root", str(root), "--disableExec=true"],
            ["users", "add", username, password, "--perm.execute=false"],
        ]:
            subprocess.run([str(initializer), "--database", str(database), *args],
                           cwd=temporary, check=True, capture_output=True)
        with (temporary / "server.log").open("w") as log:
            server = subprocess.Popen(
                [str(binary), "--database", str(database), "--address", "127.0.0.1", "--port", str(port)],
                cwd=temporary, stdout=log, stderr=subprocess.STDOUT,
            )
            try:
                for _ in range(100):
                    try:
                        with urllib.request.urlopen(base + "/health", timeout=1):
                            break
                    except OSError:
                        if server.poll() is not None:
                            raise RuntimeError("The test server stopped during startup")
                        time.sleep(0.1)
                else:
                    raise RuntimeError("The test server did not become ready")

                def request(path, method="GET", data=None, headers=None):
                    url = urllib.parse.urljoin(base, path)
                    return urllib.request.urlopen(urllib.request.Request(
                        url, method=method, data=data, headers=headers or {}), timeout=60)

                credentials = json.dumps({"username": username, "password": password}).encode()
                with request("/api/login", "POST", credentials, {"Content-Type": "application/json"}) as response:
                    token = response.read().decode()
                auth = {"X-Auth": token, "Tus-Resumable": "1.0.0"}
                size = 64 * 1024 * 1024 + 123
                with request("/api/tus/upgrade-transfer.bin?override=false", "POST", b"",
                             {**auth, "Upload-Length": str(size)}) as response:
                    assert response.status == 201
                    location = response.headers["Location"]

                chunk = bytes(range(256)) * 32768
                expected = hashlib.sha256()
                offset = 0
                while offset < size:
                    part = chunk[:min(len(chunk), size - offset)]
                    with request(location, "PATCH", part, {
                        **auth, "Content-Type": "application/offset+octet-stream", "Upload-Offset": str(offset),
                    }) as response:
                        assert response.status == 204
                        offset += len(part)
                        assert int(response.headers["Upload-Offset"]) == offset
                    expected.update(part)
                    if offset == len(chunk):
                        # A new request must recover the saved offset before resuming.
                        with request(location, "HEAD", headers=auth) as response:
                            assert int(response.headers["Upload-Offset"]) == offset
                with request("/api/raw/upgrade-transfer.bin", headers={**auth, "Range": "bytes=300-555"}) as response:
                    assert response.status == 206
                    assert response.headers["Content-Range"] == f"bytes 300-555/{size}"
                    assert response.read() == chunk[300:556]
                downloaded = hashlib.sha256()
                count = 0
                with request("/api/raw/upgrade-transfer.bin", headers=auth) as response:
                    while part := response.read(1024 * 1024):
                        downloaded.update(part)
                        count += len(part)
                assert count == size and downloaded.digest() == expected.digest()
                with request("/") as response:
                    assert b"/assets/" in response.read()
                print(json.dumps({"upload_bytes": size, "download_bytes": count,
                                  "resume": "pass", "range": "pass", "sha256": expected.hexdigest()}))

                if os.environ.get("PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH"):
                    subprocess.run(["node", "tests/smoke-app.mjs"], cwd=repository / "frontend",
                                   input=json.dumps({"base": base, "username": username, "password": password}),
                                   text=True, check=True)
            finally:
                server.terminate()
                try:
                    server.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    server.kill()
                    server.wait()


if __name__ == "__main__":
    main()

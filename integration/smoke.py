#!/usr/bin/env python3
"""Isolated three-peer mDNS regression suite; requires Go, Docker and Compose."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import time

repo = Path(__file__).resolve().parents[1]
root = Path(tempfile.mkdtemp(prefix="gosync-regression-"))
env = dict(os.environ, TEST_ROOT=str(root), TEST_UID=str(os.getuid()), TEST_GID=str(os.getgid()))
command = ["docker", "compose", "-p", f"gosync-test-{os.getpid()}", "-f", str(repo / "compose.test.yaml")]
results = []
peers = [root / n / "files" for n in "abc"]
for n in "abc":
    for kind in ("files", "state"):
        (root / n / kind).mkdir(parents=True)

def compose(*args):
    return subprocess.run(command + list(args), cwd=repo, env=env, check=True, capture_output=True, text=True).stdout

def wait(predicate, timeout=15):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            if predicate():
                return True
        except (FileNotFoundError, OSError):
            pass
        time.sleep(.1)
    return False

def equals(name, data, selected=None):
    return all((p / name).read_bytes() == data for p in (selected or peers))

def check(name, predicate):
    passed = wait(predicate)
    results.append({"test": name, "passed": passed})
    print(f'{"PASS" if passed else "FAIL"} {name}', flush=True)
    if not passed:
        raise AssertionError(name)

try:
    (repo / ".test-bin").mkdir(exist_ok=True)
    subprocess.run(["go", "build", "-o", str(repo / ".test-bin/gosync"), "./cmd/server"], cwd=repo, env=dict(os.environ, CGO_ENABLED="0"), check=True)
    compose("up", "-d", "--build")
    check("mDNS discovery with different ports", lambda: compose("logs").count("Discovered peer") >= 3)
    content = b"abcdefghij"
    (peers[0] / "text").write_bytes(content)
    check("create", lambda: equals("text", content))
    content = b"abc"
    (peers[1] / "text").write_bytes(content)
    check("shrink from peer B", lambda: equals("text", content))
    time.sleep(1)
    check("no stale contents echoed back", lambda: equals("text", content))
    for content in (b"z" * 200000, b"short", b""):
        (peers[2] / "text").write_bytes(content)
        check(f"resize to {len(content)} bytes from C", lambda: equals("text", content))
    binary = bytes(range(256)) * 1024 + b"tail"
    (peers[0] / "binary").write_bytes(binary)
    check("binary including partial last chunk", lambda: equals("binary", binary))
    (peers[0] / "binary").chmod(0o755)
    check("executable permissions", lambda: all((p / "binary").stat().st_mode & 0o777 == 0o755 for p in peers))
    (peers[0] / "empty").touch()
    check("empty file", lambda: equals("empty", b""))
    (peers[0] / ".env").write_bytes(b"KEY=value")
    check("dotfile", lambda: equals(".env", b"KEY=value"))
    (peers[0] / "src/nested").mkdir(parents=True)
    (peers[0] / "src/nested/index.ts").write_bytes(b"export {}")
    check("new nested directories and file", lambda: equals("src/nested/index.ts", b"export {}"))
    (peers[1] / "src").rename(peers[1] / "renamed")
    check("directory rename removes old tree", lambda: equals("renamed/nested/index.ts", b"export {}") and all(not (p / "src").exists() for p in peers))
    (peers[0] / "binary").rename(peers[0] / "moved")
    check("file rename removes old name", lambda: equals("moved", binary) and all(not (p / "binary").exists() for p in peers))
    # Editor save by temporary-file replacement.
    (peers[0] / "save.tmp").write_bytes(b"new contents")
    (peers[0] / "save.tmp").replace(peers[0] / "text")
    check("atomic editor save", lambda: equals("text", b"new contents"))
    compose("stop", "b")
    (peers[0] / "moved").unlink()
    (peers[2] / "text").write_bytes(b"changed offline")
    check("remaining peers survive disconnect", lambda: equals("text", b"changed offline", [peers[0], peers[2]]))
    check("delete while peer offline", lambda: not (peers[0] / "moved").exists() and not (peers[2] / "moved").exists())
    compose("restart", "a")
    compose("start", "b")
    check("restart recovers existing-file updates", lambda: equals("text", b"changed offline"))
    check("persistent tombstones prevent resurrection", lambda: all(not (p / "moved").exists() for p in peers))
    time.sleep(2)
    check("deletion stays deleted", lambda: all(not (p / "moved").exists() for p in peers))
    # Keep writing faster than the old 500 ms debounce. Observe actual replicas
    # while the writer is still active, not only after it closes the file.
    finished = threading.Event()
    writer_errors = []
    def download():
        try:
            with (peers[0] / "download.bin").open("wb") as out:
                for i in range(100):
                    out.write(bytes([i]) * 16384)
                    out.flush()
                    time.sleep(.04)
        except Exception as error:
            writer_errors.append(str(error))
        finally:
            finished.set()
    writer = threading.Thread(target=download)
    writer.start()
    try:
        check("replicas receive bytes while download is still writing",
              lambda: not finished.is_set() and all((p / "download.bin").stat().st_size > 0 for p in peers[1:]))
        observed = [(p / "download.bin").stat().st_size for p in peers[1:]]
        check("replicas keep growing before writer finishes",
              lambda: not finished.is_set() and all((p / "download.bin").stat().st_size > n for p, n in zip(peers[1:], observed)))
    finally:
        writer.join()
    if writer_errors:
        raise AssertionError(writer_errors)
    expected_download = b"".join(bytes([i]) * 16384 for i in range(100))
    check("completed download has exact bytes on every peer", lambda: equals("download.bin", expected_download))
    check("growth reuses existing bytes", lambda: any("Delta download.bin:" in line and "reused_bytes=0" not in line for line in compose("logs").splitlines()))
    # Replace a block in a preallocated binary file without changing its length.
    with (peers[1] / "download.bin").open("r+b") as out:
        out.seek(70000)
        out.write(b"PATCH")
    expected_download = expected_download[:70000] + b"PATCH" + expected_download[70005:]
    check("same-size binary patch propagates from B", lambda: equals("download.bin", expected_download))
    # Compare every path, byte hash and executable bit, not just selected files.
    def manifest(p):
        return {str(x.relative_to(p)): ("dir" if x.is_dir() else hashlib.sha256(x.read_bytes()).hexdigest(), 0 if x.is_dir() else x.stat().st_mode & 0o777) for x in p.rglob("*") if not x.name.startswith(".gosync-stage-")}
    check("complete final directory equality", lambda: manifest(peers[0]) == manifest(peers[1]) == manifest(peers[2]))
    compose("stop")
    status = [json.loads(line) for line in compose("ps", "-a", "--format", "json").splitlines() if line]
    check("all peers shut down cleanly", lambda: len(status) == 3 and all(p["ExitCode"] == 0 for p in status))
finally:
    try:
        (root / "compose.log").write_text(compose("logs", "--no-color"))
    finally:
        compose("down", "--remove-orphans")
    (root / "results.json").write_text(json.dumps(results, indent=2))
    print(f"Logs and results: {root}", flush=True)

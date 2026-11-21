#!/usr/bin/env python3
"""Provisioned mutual TLS, folder isolation and maintenance CLI in Docker."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

repo = Path(__file__).resolve().parents[1]
root = Path(tempfile.mkdtemp(prefix="gosync-operations-"))
binary = repo / ".test-bin/gosync"
results = []

def run(*args, ok=True):
    result = subprocess.run([str(binary), *map(str, args)], capture_output=True, text=True)
    if ok and result.returncode:
        raise AssertionError(result.stderr)
    return result

def check(name, predicate, timeout=20):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            if predicate():
                results.append({"test": name, "passed": True})
                print("PASS", name, flush=True)
                return
        except (OSError, json.JSONDecodeError):
            pass
        time.sleep(.1)
    results.append({"test": name, "passed": False})
    raise AssertionError(name)

(repo / ".test-bin").mkdir(exist_ok=True)
subprocess.run(["go", "build", "-o", str(binary), "./cmd/server"], cwd=repo, env=dict(os.environ, CGO_ENABLED="0"), check=True)
run("trust", "init", "--out", root / "authority")
services = {}
for name in "abc":
    peer = root / name
    (peer / "files").mkdir(parents=True)
    (peer / "state").mkdir()
    run("trust", "issue", "--authority", root / "authority", "--name", name, "--out", peer / "identity")
    run("init", "--config", peer / "peer.json", "--sync-folder", peer / "files", "--state-dir", peer / "state", "--identity", peer / "identity", "--folder-id", "project" if name != "c" else "separate")
    config = json.loads((peer / "peer.json").read_text())
    config.update({"sync-folder": "/files", "state-dir": "/state", "tls-ca": "/identity/ca.pem", "tls-cert": "/identity/peer.pem", "tls-key": "/identity/peer-key.pem", "sync-interval": "250ms", "no-mdns": True, "peers": ",".join(f"{n}:50051" for n in "abc" if n != name)})
    (peer / "peer.json").write_text(json.dumps(config))
    services[name] = {"image": "gosync-regression:local", "build": {"context": str(repo), "dockerfile": "integration/Dockerfile"},
                      "user": f"{os.getuid()}:{os.getgid()}", "command": ["--config=/config/peer.json"],
                      "volumes": [f"{peer / 'files'}:/files", f"{peer / 'state'}:/state", f"{peer / 'identity'}:/identity:ro", f"{peer / 'peer.json'}:/config/peer.json:ro"], "stop_grace_period": "10s"}
compose_file = root / "compose.json"
compose_file.write_text(json.dumps({"services": services, "networks": {"default": {"internal": True}}}))
base = ["docker", "compose", "-p", f"gosync-operations-{os.getpid()}", "-f", str(compose_file)]
def compose(*args):
    return subprocess.run(base + list(args), check=True, capture_output=True, text=True).stdout

def status(name):
    return json.loads(run("status", "--state-dir", root / name / "state").stdout)

def maintenance(command, *args, ok=True):
    return run(command, "--sync-folder", root / "a/files", "--state-dir", root / "a/state", *args, ok=ok)

try:
    compose("up", "-d", "--build")
    check("configured TLS services report running status", lambda: all((root / n / "state/status.json").exists() and status(n)["running"] for n in "abc"))
    identities = {n: status(n)["node_id"] for n in "abc"}
    (root / "a/files/file").write_bytes(b"recover me")
    check("mutually authenticated peers synchronize", lambda: (root / "b/files/file").read_bytes() == b"recover me")
    check("different folder namespace is rejected", lambda: not (root / "c/files/file").exists() and any("identity does not match" in p.get("last_error", "") for p in status("c")["peers"]))
    run("rescan", "--state-dir", root / "a/state")
    check("online full rescan acknowledged", lambda: not list((root / "a/state/commands").glob("*.request")))
    (root / "b/files/file").write_bytes(b"new version")
    check("reverse edit creates retained history", lambda: (root / "a/files/file").read_bytes() == b"new version")
    compose("stop", "a")
    check("shutdown status is persisted", lambda: not status("a")["running"])
    maintenance("verify")
    check("offline inspection reports saved revision", lambda: any(f["file_name"] == "file" for f in json.loads(maintenance("inspect").stdout)["files"]))
    versions = json.loads(maintenance("history", "--path", "file").stdout)
    check("retained versions are listed", lambda: len(versions) == 1)
    (root / "a/files/file").write_bytes(b"unrecorded disk edit")
    check("verification detects changed bytes without adopting them", lambda: maintenance("verify", ok=False).returncode != 0)
    maintenance("restore", "--path", "file", "--version", versions[0]["id"])
    maintenance("verify")
    check("restored contents pass verification", lambda: (root / "a/files/file").read_bytes() == b"recover me")
    compose("start", "a")
    check("restored version propagates as a new edit", lambda: (root / "b/files/file").read_bytes() == b"recover me")
    compose("restart", "b")
    check("restart retains peer and folder identity", lambda: status("b")["running"] and status("b")["node_id"] == identities["b"] and status("b")["folder_id"] == "project")
    compose("stop")
    states = [json.loads(line) for line in compose("ps", "-a", "--format", "json").splitlines() if line]
    check("all configured services stop cleanly", lambda: len(states) == 3 and all(s["ExitCode"] == 0 for s in states))
finally:
    try:
        (root / "compose.log").write_text(compose("logs", "--no-color"))
    finally:
        compose("down", "--remove-orphans")
    (root / "results.json").write_text(json.dumps(results, indent=2))
    print(f"Logs and results: {root}", flush=True)

#!/usr/bin/env python3
"""Exercise init + bare startup, gonfig YAML, and automatic three-peer LAN sync."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

repo = Path(__file__).resolve().parents[1]
root = Path(tempfile.mkdtemp(prefix="gosync-startup-"))
binary = repo / ".test-bin/gosync"
binary.parent.mkdir(exist_ok=True)
subprocess.run(["go", "build", "-o", str(binary), "./cmd/server"], cwd=repo,
               env=dict(os.environ, CGO_ENABLED="0"), check=True)
folders = dict(zip("abc", ("project", "mirror", "shared")))
services = {}
results = []
for name, folder in folders.items():
    (root / name / folder).mkdir(parents=True)
    services[name] = {
        "image": "gosync-startup:local", "user": f"{os.getuid()}:{os.getgid()}",
        "environment": {"XDG_CONFIG_HOME": "/work/config", "GOSYNC_TEST_INTERVAL": "250ms"},
        "working_dir": "/work", "volumes": [f"{root / name}:/work"],
        "stop_grace_period": "10s",
    }
services["a"]["build"] = {"context": str(repo), "dockerfile": "integration/Dockerfile"}
config = root / "compose.json"
config.write_text(json.dumps({"services": services, "networks": {"default": {"internal": True}}}))
base = ["docker", "compose", "-p", f"gosync-startup-{os.getpid()}", "-f", str(config)]
peers = [root / name / folder for name, folder in folders.items()]


def compose(*args):
    result = subprocess.run(base + list(args), capture_output=True, text=True)
    if result.returncode:
        raise AssertionError(result.stderr)
    return result.stdout


def check(name, predicate, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
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


def status(name):
    return json.loads((root / name / "config/gosync/default.yaml.state/status.json").read_text())


def equals(filename, content, selected=None):
    return all((p / filename).read_bytes() == content for p in (selected or peers))


try:
    (peers[0] / "existing.txt").write_bytes(b"already here before setup")
    compose("build", "a")
    for name, folder in folders.items():
        compose("run", "--rm", "--no-deps", name, "init", "--sync-folder", f"/work/{folder}")
        saved = root / name / "config/gosync/default.yaml"
        check(f"{name}: folder-only init saved YAML and automatic LAN defaults",
              lambda: saved.exists() and "insecure: true" in saved.read_text()
              and "folder-id: gosync-lan" in saved.read_text() and "bootstrap:" not in saved.read_text())
        # Use a real environment-backed setting, without supplying any peer address.
        saved.write_text(saved.read_text().replace("sync-interval: 1s", "sync-interval: '${GOSYNC_TEST_INTERVAL:-1s}'") + "debug: true\n")
    check("setup leaves existing contents in place", lambda: (peers[0] / "existing.txt").read_bytes() == b"already here before setup")
    compose("up", "-d", "--no-build", "--pull", "never")
    check("bare startup reads the saved YAML on every peer", lambda: all(status(n)["running"] for n in folders))
    identities = {n: status(n)["node_id"] for n in folders}
    check("peers keep distinct identities in the automatic LAN group",
          lambda: len(set(identities.values())) == 3 and all(status(n)["folder_id"] == "gosync-lan" for n in folders))
    check("mDNS finds peers without addresses or pairing", lambda: compose("logs").count("Discovered peer") >= 6)
    check("pre-existing files replicate without a separate bootstrap command", lambda: equals("existing.txt", b"already here before setup"))
    (peers[1] / "existing.txt").write_bytes(b"edited on second machine")
    check("reverse edits propagate", lambda: equals("existing.txt", b"edited on second machine"))
    (peers[2] / "binary.bin").write_bytes(bytes(range(256)) * 1024)
    check("third peer can originate binary files", lambda: equals("binary.bin", bytes(range(256)) * 1024))
    (peers[2] / "existing.txt").rename(peers[2] / "renamed.txt")
    check("rename propagates", lambda: equals("renamed.txt", b"edited on second machine") and all(not (p / "existing.txt").exists() for p in peers))
    compose("stop", "c")
    (peers[0] / "renamed.txt").unlink()
    check("remaining peers propagate deletion", lambda: all(not (p / "renamed.txt").exists() for p in peers[:2]))
    compose("start", "c")
    check("bare restart retains identity and catches up", lambda: status("c")["running"] and status("c")["node_id"] == identities["c"] and not (peers[2] / "renamed.txt").exists())
    compose("stop")
    states = [json.loads(line) for line in compose("ps", "-a", "--format", "json").splitlines() if line]
    check("all default-config services stop cleanly", lambda: len(states) == 3 and all(s["ExitCode"] == 0 for s in states))
finally:
    try:
        (root / "compose.log").write_text(compose("logs", "--no-color"))
    finally:
        compose("down", "--remove-orphans")
    (root / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    print(f"Logs and results: {root}", flush=True)

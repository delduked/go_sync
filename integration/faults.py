#!/usr/bin/env python3
"""Docker-only network/resource/recovery qualification; never touches host qdiscs."""
import hashlib
import json
import os
from pathlib import Path
import platform
import statistics
import subprocess
import tempfile
import time

repo = Path(__file__).resolve().parents[1]
root = Path(tempfile.mkdtemp(prefix="gosync-faults-"))
binary = repo / ".test-bin/gosync"
names = "abc"
folders = {n: root / n / "files" for n in names}
services = {}
results = []
report = {"environment": {"platform": platform.platform(), "logical_cpus": os.cpu_count()}, "checks": results}
for n in names:
    folders[n].mkdir(parents=True)
    (root / n / "state").mkdir()
    services[n] = {"image": "gosync-regression:local", "build": {"context": str(repo), "dockerfile": "integration/Dockerfile"},
                   "user": f"{os.getuid()}:{os.getgid()}", "command": ["--sync-folder=/files", "--state-dir=/state", "--sync-interval=250ms", "--insecure", "--no-mdns", "--peers=" + ",".join(f"{p}:50051" for p in names if p != n)],
                   "volumes": [f"{folders[n]}:/files", f"{root / n / 'state'}:/state"], "stop_grace_period": "10s"}
config = root / "compose.json"
def save_config():
    config.write_text(json.dumps({"services": services, "networks": {"default": {"internal": True}}}))
save_config()
base = ["docker", "compose", "-p", f"gosync-faults-{os.getpid()}", "-f", str(config)]

def command(args):
    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError(f"{args[:4]}: {result.stderr}")
    return result.stdout

def compose(*args):
    return command(base + list(args))

def wait(predicate, timeout=90):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            if predicate():
                return
        except (OSError, json.JSONDecodeError):
            pass
        time.sleep(.1)
    raise AssertionError("condition deadline exceeded")

def check(name, predicate, timeout=90):
    started = time.monotonic()
    try:
        wait(predicate, timeout)
    except Exception:
        results.append({"test": name, "passed": False})
        raise
    results.append({"test": name, "passed": True, "seconds": round(time.monotonic() - started, 3)})
    print("PASS", name, flush=True)

def equals(name, data, selected=names):
    return all((folders[n] / name).read_bytes() == data for n in selected)

def status(n):
    return json.loads((root / n / "state/status.json").read_text())

def metrics(n):
    return json.loads((root / n / "state/metrics.json").read_text())

def payload(label, size):
    block = hashlib.sha256(label.encode()).digest()
    return (block * (size // 32 + 1))[:size]

def transfer(name, data):
    started = time.monotonic()
    (folders["a"] / name).write_bytes(data)
    wait(lambda: equals(name, data))
    return round(time.monotonic() - started, 3)

def netem(peer, *args):
    return command(["docker", "run", "--rm", "--network", f"container:{ids[peer]}", "--cap-add=NET_ADMIN", "gosync-network-tools:local", "tc", *args])

def resource_snapshot():
    raw = command(["docker", "stats", "--no-stream", "--format", "{{json .}}", *ids.values()])
    return [json.loads(line) for line in raw.splitlines() if line]

def storage_snapshot():
    output = {}
    for n in names:
        state = root / n / "state"
        backups = list((state / "versions").glob("*"))
        output[n] = {"state_apparent_bytes": sum(p.stat().st_size for p in state.rglob("*") if p.is_file()),
                     "state_allocated_bytes": sum(p.stat().st_blocks * 512 for p in state.rglob("*") if p.is_file()),
                     "backup_count": len(backups), "backup_bytes": sum(p.stat().st_size for p in backups)}
    return output

try:
    (repo / ".test-bin").mkdir(exist_ok=True)
    command(["go", "build", "-o", str(binary), "./cmd/server"])
    command(["docker", "build", "-f", str(repo / "integration/Dockerfile.network"), "-t", "gosync-network-tools:local", str(repo)])
    compose("up", "-d", "--build")
    ids = {n: compose("ps", "-q", n).strip() for n in names}
    check("fault-test peers start", lambda: all((root / n / "state/status.json").exists() and status(n)["running"] for n in names))
    network = next(iter(json.loads(command(["docker", "inspect", ids["a"]]))[0]["NetworkSettings"]["Networks"]))
    baseline = payload("baseline", 16 << 20)
    report["baseline"] = {"bytes": len(baseline), "convergence_seconds": transfer("baseline.bin", baseline)}
    print("BASELINE", json.dumps(report["baseline"]), flush=True)
    # This helper shares ONLY the isolated test container's network namespace.
    netem("a", "qdisc", "replace", "dev", "eth0", "root", "netem", "delay", "40ms", "10ms", "loss", "0.5%", "rate", "20mbit", "seed", "42")
    report["netem_configuration"] = netem("a", "-s", "qdisc", "show", "dev", "eth0")
    delayed = payload("latency-loss-rate", 4 << 20)
    report["impaired_transfer"] = {"bytes": len(delayed), "convergence_seconds": transfer("impaired.bin", delayed)}
    report["netem_counters"] = netem("a", "-s", "qdisc", "show", "dev", "eth0")
    results.append({"test": "latency loss and bandwidth impairment converges", "passed": True})
    print("IMPAIRED", json.dumps(report["impaired_transfer"]), flush=True)
    netem("a", "qdisc", "del", "dev", "eth0", "root")
    (folders["a"] / "offline.txt").write_bytes(b"delete while disconnected")
    check("offline scenario base", lambda: equals("offline.txt", b"delete while disconnected"))
    command(["docker", "network", "disconnect", network, ids["b"]])
    (folders["a"] / "offline.txt").unlink()
    (folders["b"] / "offline-edit.txt").write_bytes(b"edited without a network")
    check("online peers progress through a network partition", lambda: not (folders["c"] / "offline.txt").exists())
    command(["docker", "network", "connect", "--alias", "b", network, ids["b"]])
    check("network rejoin merges edits and deletion", lambda: equals("offline-edit.txt", b"edited without a network") and all(not (p / "offline.txt").exists() for p in folders.values()))
    # Throttle the origin so an abrupt process kill occurs during a real transfer.
    services["a"]["command"].append("--upload-limit=1048576")
    save_config()
    compose("up", "-d", "--no-deps", "--force-recreate", "a")
    ids["a"] = compose("ps", "-q", "a").strip()
    data = payload("kill-and-resume", 8 << 20)
    (folders["a"] / "resume.bin").write_bytes(data)
    check("receiver has a partial transfer before crash", lambda: any(p["path"] == "resume.bin" and p["payload_received_this_attempt"] >= 65536 for p in status("b")["transfers"]))
    report["before_crash_progress"] = status("b")["transfers"]
    command(["docker", "kill", "--signal=KILL", ids["b"]])
    compose("start", "b")
    check("abrupt receiver restart resumes and verifies exact bytes", lambda: equals("resume.bin", data))
    time.sleep(1.2)
    report["resumed_received_bytes"] = metrics("b")["payload_received_bytes"]
    check("restart reuses retained payload", lambda: report["resumed_received_bytes"] < len(data))
    services["a"]["command"].remove("--upload-limit=1048576")
    save_config()
    compose("up", "-d", "--no-deps", "--force-recreate", "a")
    ids["a"] = compose("ps", "-q", "a").strip()
    command(["docker", "update", "--cpus=0.25", "--memory=256m", "--memory-swap=256m", ids["b"]])
    limited = payload("limited-cpu", 2 << 20)
    report["limited_peer"] = {"cpus": .25, "memory_bytes": 256 << 20, "convergence_seconds": transfer("limited.bin", limited)}
    results.append({"test": "CPU and memory constrained peer converges", "passed": True})
    command(["docker", "update", "--cpus=0", "--memory=1g", "--memory-swap=1g", ids["b"]])
    (folders["a"] / "blocked").mkdir()
    check("storage scenario directory replicated", lambda: all((p / "blocked").is_dir() for p in folders.values()))
    (folders["b"] / "blocked").chmod(0o555)
    (folders["a"] / "blocked/file").write_bytes(b"retry after storage permissions recover")
    check("storage failure is visible", lambda: any("permission denied" in peer.get("last_error", "").lower() for peer in status("b")["peers"]))
    (folders["b"] / "blocked").chmod(0o755)
    check("storage failure retries successfully", lambda: equals("blocked/file", b"retry after storage permissions recover"))
    report["resources_before_sustained"] = resource_snapshot()
    report["storage_before_sustained"] = storage_snapshot()
    timings = []
    for cycle in range(60):
        origin = names[cycle % len(names)]
        data = payload(f"sustained-{cycle}", ((cycle % 4) + 1) * 16384)
        started = time.monotonic()
        (folders[origin] / "sustained.bin").write_bytes(data)
        wait(lambda: equals("sustained.bin", data))
        timings.append(time.monotonic() - started)
        if cycle % 15 == 14:
            print(f"SUSTAINED {cycle + 1}/60", flush=True)
    report["sustained"] = {"cycles": 60, "seconds": round(sum(timings), 3), "median_convergence_seconds": round(statistics.median(timings), 3), "max_convergence_seconds": round(max(timings), 3)}
    report["resources_after_sustained"] = resource_snapshot()
    report["storage_after_sustained"] = storage_snapshot()
    results.append({"test": "sixty rotating-origin overwrite cycles converge", "passed": True})
    compose("stop")
    pruned = {}
    for n in names:
        pruned[n] = json.loads(command([str(binary), "prune", "--sync-folder", str(folders[n]), "--state-dir", str(root / n / "state"), "--older-than=1ns"]))
    report["prune"] = pruned
    report["storage_after_prune"] = storage_snapshot()
    check("expired history is reclaimed", lambda: all(v["backup_count"] == 0 for v in report["storage_after_prune"].values()) and sum(v["backups_removed"] for v in pruned.values()) > 0)
    report["passed"] = True
finally:
    for p in folders.values():
        if (p / "blocked").exists():
            (p / "blocked").chmod(0o755)
    try:
        (root / "compose.log").write_text(compose("logs", "--no-color"))
    finally:
        compose("down", "--remove-orphans")
    (root / "results.json").write_text(json.dumps(report, indent=2))
    print(f"Logs and results: {root}", flush=True)

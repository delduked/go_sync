#!/usr/bin/env python3
"""Reproducible isolated N-peer distribution measurements; logs survive cleanup."""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--peers", type=int, default=10)
    parser.add_argument("--size-mib", type=int, default=8)
    parser.add_argument("--fanout", type=int, default=None)
    parser.add_argument("--all-origins", action="store_true")
    parser.add_argument("--relay-failure", action="store_true")
    args = parser.parse_args()
    if not 2 <= args.peers <= 50 or not 1 <= args.size_mib <= 512:
        parser.error("use 2–50 peers and 1–512 MiB")
    repo = Path(__file__).resolve().parents[1]
    root = Path(tempfile.mkdtemp(prefix="gosync-mesh-"))
    names = [f"p{i}" for i in range(args.peers)]
    folders = [root / name / "files" for name in names]
    services = {}
    for name, folder in zip(names, folders):
        folder.mkdir(parents=True)
        (root / name / "state").mkdir()
        command = ["--sync-folder=/files", "--state-dir=/state", "--sync-interval=500ms", "--insecure", "--no-mdns", "--debug",
                   "--peers=" + ",".join(f"{peer}:50051" for peer in names if peer != name)]
        if args.fanout is not None:
            command.append(f"--fanout={args.fanout}")
        services[name] = {"image": "gosync-regression:local", "build": {"context": str(repo), "dockerfile": "integration/Dockerfile"},
                          "user": f"{os.getuid()}:{os.getgid()}", "command": command,
                          "volumes": [f"{folder}:/files", f"{root / name / 'state'}:/state"], "stop_grace_period": "10s"}
    config = root / "compose.json"
    config.write_text(json.dumps({"services": services, "networks": {"default": {"internal": True}}}))
    base = ["docker", "compose", "-p", f"gosync-mesh-{os.getpid()}", "-f", str(config)]
    def compose(*commands):
        return subprocess.run(base + list(commands), check=True, capture_output=True, text=True).stdout
    def wait(predicate, timeout=90):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            try:
                if predicate():
                    return
            except (OSError, json.JSONDecodeError):
                pass
            time.sleep(.1)
        raise AssertionError("convergence deadline exceeded")
    def metrics():
        return [json.loads((root / n / "state/metrics.json").read_text()) for n in names]
    report = {"peers": args.peers, "size_mib": args.size_mib, "fanout": args.fanout, "workloads": []}
    try:
        (repo / ".test-bin").mkdir(exist_ok=True)
        subprocess.run(["go", "build", "-o", str(repo / ".test-bin/gosync"), "./cmd/server"], cwd=repo, env=dict(os.environ, CGO_ENABLED="0"), check=True)
        compose("up", "-d", "--build")
        wait(lambda: all((root / n / "state/metrics.json").exists() for n in names))
        time.sleep(2)  # Let discovery/metadata converge before measuring data.
        for origin in range(args.peers if args.all_origins else 1):
            before = metrics()
            block = hashlib.sha256(f"gosync-mesh-{origin}".encode()).digest()
            data = block * (args.size_mib * 1024 * 1024 // len(block))
            digest = hashlib.sha256(data).digest()
            name = f"origin-{origin}.bin"
            started = time.monotonic()
            (folders[origin] / name).write_bytes(data)
            wait(lambda: all(hashlib.sha256((p / name).read_bytes()).digest() == digest for p in folders))
            elapsed = time.monotonic() - started
            time.sleep(1.2)  # Atomic counters are exported once per second.
            after = metrics()
            sent = [a["payload_sent_bytes"] - b["payload_sent_bytes"] for a, b in zip(after, before)]
            received = [a["payload_received_bytes"] - b["payload_received_bytes"] for a, b in zip(after, before)]
            result = {"origin": origin, "bytes": len(data), "convergence_seconds": round(elapsed, 3),
                      "origin_payload_sent": sent[origin], "total_payload_sent": sum(sent), "total_payload_received": sum(received), "per_peer_sent": sent}
            report["workloads"].append(result)
            print(json.dumps(result), flush=True)
        if args.relay_failure:
            identities = {name: re.search(r"node=([a-f0-9]{32})", compose("logs", name)).group(1) for name in names}
            relay = sorted(names[1:], key=lambda n: identities[n])[0]
            compose("stop", relay)
            data = hashlib.sha256(b"relay failure").digest() * (args.size_mib * 1024 * 1024 // 32)
            digest = hashlib.sha256(data).digest()
            started = time.monotonic()
            (folders[0] / "relay-failure.bin").write_bytes(data)
            online = [p for n, p in zip(names, folders) if n != relay]
            wait(lambda: all(hashlib.sha256((p / "relay-failure.bin").read_bytes()).digest() == digest for p in online))
            elapsed = time.monotonic() - started
            compose("start", relay)
            wait(lambda: all(hashlib.sha256((p / "relay-failure.bin").read_bytes()).digest() == digest for p in folders))
            report["relay_failure"] = {"stopped_relay": relay, "online_convergence_seconds": round(elapsed, 3), "rejoined": True}
            print(json.dumps(report["relay_failure"]), flush=True)
        compose("stop")
        states = [json.loads(line) for line in compose("ps", "-a", "--format", "json").splitlines() if line]
        if len(states) != args.peers or any(s["ExitCode"] != 0 for s in states):
            raise AssertionError("unclean peer shutdown")
        report["passed"] = True
    finally:
        try:
            (root / "compose.log").write_text(compose("logs", "--no-color"))
        finally:
            compose("down", "--remove-orphans")
        (root / "results.json").write_text(json.dumps(report, indent=2))
        print(f"Logs and results: {root}", flush=True)

if __name__ == "__main__":
    main()

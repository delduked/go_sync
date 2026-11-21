#!/usr/bin/env python3
"""Native two/three-peer LAN tests in fresh folders; SSH auth stays outside the harness."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tempfile
import time

REPO = Path(__file__).resolve().parents[1]
HELPER = (REPO / "integration/lan_peer.py").read_text()

class Peer:
    def __init__(self, spec, run_id):
        self.spec = spec
        self.name = spec["name"]
        if not re.fullmatch(r"[a-z0-9_-]+", self.name):
            raise ValueError("peer names must use lowercase letters, digits, dash or underscore")
        self.ssh = spec.get("ssh")
        if self.ssh and (self.ssh.startswith("-") or any(c.isspace() for c in self.ssh)):
            raise ValueError("invalid SSH target")
        self.options = ["-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=8"]
        if spec.get("control"):
            self.options += ["-o", "ControlPath=" + spec["control"]]
        self.root = ""
        self.info = self.call("probe")
        self.root = str(Path(self.info["home"]) / ".local/state/gosync-lan" / run_id / self.name)
        self.started = False
        self.prepared = False

    def call(self, op, **kwargs):
        args = [sys.executable, "-c", HELPER] if not self.ssh else ["ssh", *self.options, self.ssh, shlex.join(["python3", "-c", HELPER])]
        result = subprocess.run(args, input=json.dumps(dict(op=op, root=self.root, **kwargs)), capture_output=True, text=True, timeout=90)
        if result.returncode:
            raise RuntimeError(f"{self.name}/{op}: {result.stderr.strip()}")
        return json.loads(result.stdout)

    def copy(self, source, destination):
        destination = self.root + "/" + destination
        if not self.ssh:
            shutil.copy2(source, destination)
        else:
            result = subprocess.run(["scp", *self.options, str(source), self.ssh + ":" + destination], capture_output=True, text=True, timeout=120)
            if result.returncode:
                raise RuntimeError(f"{self.name}/copy: {result.stderr.strip()}")

    def start(self):
        self.started = True
        self.call("start")

    def stop(self):
        if self.started:
            result = self.call("stop")
            self.started = False
            return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--inventory", type=Path, required=True, help="JSON list of name/address and optional ssh/control fields; no passwords")
    parser.add_argument("--port", type=int, default=54171)
    parser.add_argument("--size-mib", type=int, default=64)
    parser.add_argument("--explicit-peers", action="store_true", help="Use listed LAN addresses instead of mDNS")
    args = parser.parse_args()
    specs = json.loads(args.inventory.read_text())
    if not 2 <= len(specs) <= 3 or len({s["name"] for s in specs}) != len(specs):
        parser.error("supply two or three distinct peers")
    if not 1 <= args.port <= 65535 or not 1 <= args.size_mib <= 256:
        parser.error("invalid port or file size")
    output = Path(tempfile.mkdtemp(prefix="gosync-native-lan-"))
    output.chmod(0o700)
    run_id = output.name
    report = {"run_id": run_id, "transport": "mutual TLS", "discovery": "explicit LAN addresses" if args.explicit_peers else "mDNS", "port": args.port, "checks": [], "peers": []}
    peers = []
    def wait(predicate, timeout=60):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            if predicate():
                return
            time.sleep(.2)
        raise AssertionError("condition deadline exceeded")
    def check(name, predicate, timeout=60):
        started = time.monotonic()
        try:
            wait(predicate, timeout)
        except Exception:
            report["checks"].append({"test": name, "passed": False})
            raise
        report["checks"].append({"test": name, "passed": True, "seconds": round(time.monotonic() - started, 3)})
        print("PASS", name, flush=True)
    def matches(name, digest):
        for p in peers:
            info = p.call("inspect", names=[name])[name]
            if not info or info.get("sha256") != digest:
                return False
        return True
    def text_digest(text):
        return hashlib.sha256(text.encode()).hexdigest()
    try:
        peers = [Peer(spec, run_id) for spec in specs]
        local_binary = output / "gosync-controller"
        subprocess.run(["go", "build", "-o", str(local_binary), "./cmd/server"], cwd=REPO, env=dict(os.environ, CGO_ENABLED="0"), check=True)
        subprocess.run([str(local_binary), "trust", "init", "--out", str(output / "authority")], check=True, capture_output=True)
        built = {}
        for p in peers:
            system = {"Linux": "linux", "Darwin": "darwin"}[p.info["system"]]
            arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}[p.info["machine"]]
            key = system + "-" + arch
            if key not in built:
                built[key] = output / ("gosync-" + key)
                subprocess.run(["go", "build", "-o", str(built[key]), "./cmd/server"], cwd=REPO, env=dict(os.environ, CGO_ENABLED="0", GOOS=system, GOARCH=arch), check=True)
            p.call("prepare")
            p.prepared = True
            identity = output / (p.name + "-identity")
            subprocess.run([str(local_binary), "trust", "issue", "--authority", str(output / "authority"), "--name", p.name, "--out", str(identity)], check=True, capture_output=True)
            p.copy(built[key], "gosync")
            for name in ("ca.pem", "peer.pem", "peer-key.pem"):
                p.copy(identity / name, "identity/" + name)
            config = {"sync-folder": p.root + "/files", "state-dir": p.root + "/state", "folder-id": run_id, "port": str(args.port), "sync-interval": "250ms", "tls-ca": p.root + "/identity/ca.pem", "tls-cert": p.root + "/identity/peer.pem", "tls-key": p.root + "/identity/peer-key.pem", "debug": True}
            if args.explicit_peers:
                config.update({"no-mdns": True, "peers": ",".join(other.spec["address"] + ":" + str(args.port) for other in peers if other is not p)})
            p.call("configure", config=config)
            report["peers"].append(dict(name=p.name, address=p.spec["address"], root=p.root, **p.info))
        for p in peers:
            p.start()
        check("native services start", lambda: all(p.call("status").get("running") for p in peers))
        def connected():
            for p in peers:
                records = p.call("status").get("peers", [])
                for other in peers:
                    if p is other:
                        continue
                    if not any(r["address"] == other.spec["address"] + ":" + str(args.port) and r.get("node_id") and not r.get("last_success", "0001").startswith("0001") for r in records):
                        return False
            return True
        check("authenticated connections use the physical LAN addresses", connected)
        for p in peers:
            text = f"edited on {p.name}\n"
            p.call("write", files={"hello.txt": text})
            check(f"text edit from {p.name}", lambda: matches("hello.txt", text_digest(text)))
        time.sleep(1.2)  # Flush counters from the preceding small-file checks.
        before = peers[0].call("metrics")
        started = time.monotonic()
        digest = peers[0].call("binary", name="large.bin", size=args.size_mib << 20)
        check("large binary matches on every host", lambda: matches("large.bin", digest))
        report["large_transfer"] = {"bytes": args.size_mib << 20, "seconds_including_validation": round(time.monotonic() - started, 3)}
        time.sleep(1.2)
        after = peers[0].call("metrics")
        report["large_transfer"]["origin_payload_sent"] = after["payload_sent_bytes"] - before["payload_sent_bytes"]
        digest = peers[1].call("patch", name="large.bin", offset=70000, text="BINARY PATCH")
        check("same-size binary edit from second host", lambda: matches("large.bin", digest))
        for p in peers:
            name = "growing-" + p.name + ".bin"
            with ThreadPoolExecutor(max_workers=1) as pool:
                writer = pool.submit(p.call, "grow", name=name, blocks=128, block_size=262144, delay=.04)
                def progressed():
                    for other in peers:
                        if other is p:
                            continue
                        info = other.call("inspect", names=[name], hash=False)[name]
                        if not info or info["size"] == 0:
                            return False
                    return not writer.done()
                check(f"replicas progress while {p.name} is still writing", progressed, timeout=5)
                digest = writer.result()
            check(f"completed growing binary from {p.name} matches", lambda: matches(name, digest))
            p.call("complete", name=name)
            check(f"completion marker from {p.name} verifies", lambda: all(other.call("verify-complete", name=name) for other in peers))
        project = {f"project/package{i // 10}/module{i}.ts": f"export const value = {i};\n" for i in range(40)}
        peers[0].call("write", files=project)
        check("project tree replicates", lambda: all(all(info and info.get("sha256") == text_digest(project[name]) for name, info in p.call("inspect", names=list(project)).items()) for p in peers))
        peers[1].call("rename", source="project", destination="moved-project")
        check("directory rename removes old paths", lambda: all(p.call("inspect", names=["project", "moved-project/package0/module0.ts"])["project"] is None for p in peers) and matches("moved-project/package0/module0.ts", text_digest(project["project/package0/module0.ts"])))
        peers[0].call("write", files={"offline.txt": "remove during outage"})
        check("offline deletion base", lambda: matches("offline.txt", text_digest("remove during outage")))
        peers[1].stop()
        peers[0].call("delete", name="offline.txt")
        peers[0].call("write", files={"hello.txt": "changed while peer stopped"})
        time.sleep(.5)
        peers[1].start()
        check("restart catches edits and persistent deletion", lambda: matches("hello.txt", text_digest("changed while peer stopped")) and all(p.call("inspect", names=["offline.txt"])["offline.txt"] is None for p in peers))
        peers[0].call("write", files={"conflict.txt": "common base"})
        check("shared base before partition", lambda: matches("conflict.txt", text_digest("common base")))
        for p in peers:
            p.stop()
        versions = set()
        for p in peers:
            text = "independent edit on " + p.name
            versions.add(text_digest(text))
            p.call("write", files={"conflict.txt": text})
        for p in peers:
            p.start()
        check("concurrent offline edits are preserved everywhere", lambda: all(versions.issubset(set(p.call("conflicts", name="conflict.txt"))) for p in peers))
        peers[0].call("write", files={"conflict.txt": "resolved"})
        check("conflict resolution propagates", lambda: matches("conflict.txt", text_digest("resolved")))
        def equal_trees():
            manifests = [p.call("manifest") for p in peers]
            return all(m == manifests[0] for m in manifests[1:])
        check("complete test-directory hashes and modes match", equal_trees)
        report["passed"] = True
    finally:
        cleanup_errors = []
        for p in peers:
            try:
                result = p.stop()
                if result and result.get("running"):
                    raise RuntimeError("service still reports running")
                if p.prepared:
                    (output / (p.name + ".log")).write_text(p.call("log"))
            except Exception as error:
                cleanup_errors.append(str(error))
        report["cleanup_errors"] = cleanup_errors
        if cleanup_errors:
            report["passed"] = False
        (output / "results.json").write_text(json.dumps(report, indent=2))
        print(f"Logs, credentials and results (private): {output}", flush=True)
        if cleanup_errors:
            raise RuntimeError("; ".join(cleanup_errors))

if __name__ == "__main__":
    main()

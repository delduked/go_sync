#!/usr/bin/env python3
"""Record real, mutually authenticated three-container GoSync file operations."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import time

REPO = Path(__file__).resolve().parents[2]
NAMES = ("atlas", "orbit", "vault")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path(__file__).with_name("trace.json"))
    args = parser.parse_args()
    root = Path(tempfile.mkdtemp(prefix="gosync-readme-"))
    binary = REPO / ".test-bin/gosync"
    binary.parent.mkdir(exist_ok=True)
    subprocess.run(["go", "build", "-o", str(binary), "./cmd/server"], cwd=REPO,
                   env=dict(os.environ, CGO_ENABLED="0"), check=True)

    def cli(*command):
        return subprocess.run([str(binary), *map(str, command)], check=True,
                              capture_output=True, text=True).stdout

    cli("trust", "init", "--out", root / "authority")
    services = {}
    for name in NAMES:
        for directory in ("files", "state"):
            (root / name / directory).mkdir(parents=True)
        cli("trust", "issue", "--authority", root / "authority", "--name", name,
            "--out", root / name / "identity")
        services[name] = {
            "image": "gosync-readme:local",
            "build": {"context": str(REPO), "dockerfile": "integration/Dockerfile"},
            "user": f"{os.getuid()}:{os.getgid()}",
            "networks": ["demo"], "stop_grace_period": "5s",
            "command": ["--sync-folder=/files", "--state-dir=/state",
                        "--sync-interval=250ms", "--folder-id=readme-demo", "--debug",
                        "--tls-cert=/identity/peer.pem", "--tls-key=/identity/peer-key.pem",
                        "--tls-ca=/identity/ca.pem"],
            "volumes": [f"{root / name / 'files'}:/files",
                        f"{root / name / 'state'}:/state",
                        f"{root / name / 'identity'}:/identity:ro"],
        }
        if name != NAMES[0]:
            services[name].pop("build")  # Build the shared image once, without tag races.
    # Isolated bridge, no host ports, no contact with a user's existing shared folders.
    config = root / "compose.json"
    config.write_text(json.dumps({"services": services, "networks": {"demo": {"internal": True}}}))
    compose_command = ["docker", "compose", "-p", f"gosync-readme-{os.getpid()}", "-f", str(config)]

    def compose(*command):
        result = subprocess.run(compose_command + list(command), capture_output=True, text=True)
        if result.returncode:
            raise RuntimeError(f"docker compose {' '.join(command)} failed:\n{result.stderr}")
        return result.stdout

    peers = [root / name / "files" for name in NAMES]

    def state():
        result = []
        for peer in peers:
            files = {}
            for filename in ("ideas.md", "launch.md"):
                try:
                    data = (peer / filename).read_bytes()
                    files[filename] = {"text": data.decode(), "bytes": len(data),
                                       "sha256": hashlib.sha256(data).hexdigest()}
                except FileNotFoundError:
                    pass
            result.append(files)
        return result

    def observe(snapshot, predicate, timeout=30):
        start = time.monotonic()
        samples = []
        while time.monotonic() - start < timeout:
            current = snapshot()
            samples.append({"seconds": round(time.monotonic() - start, 4), "peers": current})
            if predicate(current):
                return samples
            time.sleep(.04)
        raise AssertionError(f"Demo failed to converge: {samples[-1]}")

    trace = {"schema": 1, "recorded_at": datetime.now(timezone.utc).isoformat(),
             "peers": list(NAMES), "transport": "mutual TLS 1.3",
             "discovery": "mDNS", "environment": "three containers on an isolated Docker bridge",
             "source_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=REPO, text=True).strip(),
             "operations": []}
    try:
        compose("build", NAMES[0])
        compose("up", "-d", "--no-build", "--pull", "never")
        deadline = time.monotonic() + 30
        while compose("logs").count("Discovered peer") < 6:
            if time.monotonic() > deadline:
                raise AssertionError("Three-way mDNS discovery did not complete")
            time.sleep(.25)
        print("PASS three peers discovered over mDNS; mutual TLS configured", flush=True)

        operations = [
            ("create", 0, "ideas.md", "# Ship it\n"),
            ("edit", 1, "ideas.md", "# Ship it together\n"),
            ("rename", 2, "launch.md", "# Ship it together\n"),
            ("delete", 0, None, None),
        ]
        for action, origin, filename, content in operations:
            before = state()
            if action == "rename":
                (peers[origin] / "ideas.md").rename(peers[origin] / filename)
            elif action == "delete":
                (peers[origin] / "launch.md").unlink()
            else:
                (peers[origin] / filename).write_text(content)
            expected = {} if content is None else {filename: {
                "text": content, "bytes": len(content.encode()),
                "sha256": hashlib.sha256(content.encode()).hexdigest()}}
            samples = observe(state, lambda states: all(s == expected for s in states))
            trace["operations"].append({"action": action, "origin": origin,
                                         "before": before, "samples": samples})
            print(f"PASS {action} from {NAMES[origin]}: exact contents on all three peers", flush=True)
            time.sleep(.75)

        writer_done = threading.Event()
        errors = []
        chunk = 256 * 1024
        chunks = 64
        expected_hash = hashlib.sha256(b"".join(bytes([i]) * chunk for i in range(chunks))).hexdigest()

        def download():
            try:
                with (peers[0] / "download.bin").open("wb") as output:
                    for i in range(chunks):
                        output.write(bytes([i]) * chunk)
                        output.flush()
                        time.sleep(.08)
            except Exception as error:
                errors.append(str(error))
            finally:
                writer_done.set()

        def growth_state():
            sizes = []
            for peer in peers:
                try:
                    sizes.append((peer / "download.bin").stat().st_size)
                except FileNotFoundError:
                    sizes.append(0)
            return {"bytes": sizes, "writer_open": not writer_done.is_set()}

        writer = threading.Thread(target=download)
        writer.start()
        try:
            growth = observe(growth_state, lambda s: not s["writer_open"] and all(n == chunk * chunks for n in s["bytes"]))
        finally:
            writer.join()
        if errors:
            raise AssertionError(errors)
        for i in (1, 2):
            observed = {s["peers"]["bytes"][i] for s in growth
                        if s["peers"]["writer_open"] and s["peers"]["bytes"][i] > 0}
            assert len(observed) >= 2, f"{NAMES[i]} did not grow while writer was open"
        hashes = [hashlib.sha256((p / "download.bin").read_bytes()).hexdigest() for p in peers]
        assert all(value == expected_hash for value in hashes), "Final binary contents differ"
        trace["growth"] = {"origin": 0, "filename": "download.bin", "target_bytes": chunk * chunks,
                           "samples": growth, "final_sha256": hashes,
                           "replicas_grew_before_writer_closed": True}
        print("PASS both replicas grew before writer closed; final SHA-256 identical on all peers", flush=True)
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(trace, indent=2) + "\n")
        print(f"Trace: {args.out}", flush=True)
    finally:
        try:
            (root / "containers.log").write_text(compose("logs", "--no-color"))
        finally:
            compose("down", "--remove-orphans")
        print(f"Private test fixtures and logs retained outside the repository: {root}", flush=True)


if __name__ == "__main__":
    main()

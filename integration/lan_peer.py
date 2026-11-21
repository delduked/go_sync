"""Small host-side driver, executed over authenticated SSH by lan.py."""
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import socket
import subprocess
import sys
import time

request = json.load(sys.stdin)
op = request["op"]
if op == "probe":
    print(json.dumps({"hostname": socket.gethostname(), "system": platform.system(), "machine": platform.machine(), "home": str(Path.home())}))
    sys.exit(0)
root = Path(request["root"])
files = root / "files"
def target(name):
    if not name or name == "." or Path(name).is_absolute() or ".." in Path(name).parts:
        raise ValueError("test paths must stay below the isolated root")
    return files / name

def status():
    try:
        return json.loads((root / "state/status.json").read_text())
    except FileNotFoundError:
        return {}

def digest(p):
    h = hashlib.sha256()
    with p.open("rb") as f:
        while data := f.read(1024 * 1024):
            h.update(data)
    return h.hexdigest()

result = None
if op == "prepare":
    root.mkdir(parents=True, mode=0o700, exist_ok=False)
    files.mkdir(mode=0o700)
    (root / "state").mkdir(mode=0o700)
    (root / "identity").mkdir(mode=0o700)
    result = str(root)
elif op == "configure":
    (root / "peer.json").write_text(json.dumps(request["config"], indent=2))
    (root / "peer.json").chmod(0o600)
elif op == "start":
    with (root / "service.log").open("ab") as out:
        process = subprocess.Popen([str(root / "gosync"), "--config", str(root / "peer.json")], stdin=subprocess.DEVNULL, stdout=out, stderr=subprocess.STDOUT, start_new_session=True)
    (root / "service.pid").write_text(str(process.pid))
    result = process.pid
elif op == "stop":
    if (root / "service.pid").exists():
        pid = int((root / "service.pid").read_text())
        ps = subprocess.run(["ps", "-p", str(pid), "-o", "command="], capture_output=True, text=True)
        if ps.returncode == 0 and ps.stdout.strip() and "<defunct>" not in ps.stdout:
            if str(root / "gosync") not in ps.stdout:
                raise RuntimeError("PID no longer belongs to this test service")
            os.kill(pid, signal.SIGTERM)
            end = time.monotonic() + 15
            while time.monotonic() < end:
                ps = subprocess.run(["ps", "-p", str(pid), "-o", "stat="], capture_output=True, text=True)
                if ps.returncode != 0 or ps.stdout.strip().startswith("Z"):
                    break
                time.sleep(.1)
            else:
                raise RuntimeError("test service did not stop within 15 seconds")
    result = status()
elif op == "status":
    result = status()
elif op == "metrics":
    result = json.loads((root / "state/metrics.json").read_text())
elif op == "log":
    result = (root / "service.log").read_text() if (root / "service.log").exists() else ""
elif op == "write":
    for name, text in request["files"].items():
        p = target(name)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text)
elif op == "binary":
    p = target(request["name"])
    temporary = files / ".gosync-local-test-input"
    block = bytes(range(256)) * 4096
    with temporary.open("wb") as out:
        remaining = request["size"]
        while remaining:
            data = block[:min(len(block), remaining)]
            out.write(data)
            remaining -= len(data)
    temporary.replace(p)
    result = digest(p)
elif op == "patch":
    with target(request["name"]).open("r+b") as f:
        f.seek(request["offset"])
        f.write(request["text"].encode())
    result = digest(target(request["name"]))
elif op == "grow":
    p = target(request["name"])
    with p.open("wb") as out:
        for i in range(request["blocks"]):
            out.write(bytes([i % 256]) * request["block_size"])
            out.flush()
            time.sleep(request["delay"])
    result = digest(p)
elif op == "inspect":
    result = {}
    for name in request["names"]:
        p = target(name)
        if not p.exists():
            result[name] = None
        else:
            info = p.stat()
            result[name] = {"size": info.st_size, "mode": info.st_mode & 0o777, "directory": p.is_dir()}
            if request.get("hash", True) and p.is_file():
                result[name]["sha256"] = digest(p)
elif op == "rename":
    target(request["source"]).rename(target(request["destination"]))
elif op == "delete":
    p = target(request["name"])
    if p.is_dir():
        shutil.rmtree(p)
    else:
        p.unlink()
elif op == "chmod":
    target(request["name"]).chmod(request["mode"])
elif op == "conflicts":
    result = [digest(target(request["name"]))]
    for details in (files / ".gosync-conflicts").glob("*/details.json"):
        if json.loads(details.read_text()).get("path") == request["name"] and (details.parent / "data").exists():
            result.append(digest(details.parent / "data"))
elif op == "complete":
    p = subprocess.run([str(root / "gosync"), "complete", "--file", str(target(request["name"]))], capture_output=True, text=True)
    if p.returncode:
        raise RuntimeError(p.stderr)
elif op == "verify-complete":
    p = subprocess.run([str(root / "gosync"), "verify-complete", "--file", str(target(request["name"]))], capture_output=True, text=True)
    result = p.returncode == 0
elif op == "manifest":
    result = {}
    for p in files.rglob("*"):
        if any(part.startswith((".gosync-stage-", ".gosync-local-")) for part in p.relative_to(files).parts):
            continue
        result[str(p.relative_to(files))] = "directory" if p.is_dir() else [digest(p), p.stat().st_mode & 0o777]
else:
    raise ValueError("unknown operation")
print(json.dumps(result))

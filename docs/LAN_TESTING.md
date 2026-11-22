# Native LAN validation

`integration/lan.py` runs two or three native GoSync processes on real hosts.
It creates fresh private directories under each user's
`~/.local/state/gosync-lan/<run-id>/<peer>/`, provisions a unique folder identity
and mutual-TLS certificates, and stops its processes when the run ends. Test
files, state and logs remain available for inspection. Authentication uses your
existing SSH configuration/agent or a per-peer SSH control socket; passwords are
never accepted or stored by the harness. Python 3.8+ is required on each host,
and Go 1.25.1+ on the controller to build the Linux/macOS binaries.

Copy `integration/lan.inventory.example.json` outside the repository and replace
the documentation addresses with your actual LAN addresses. The entry without
`ssh` runs locally. A remote entry can optionally set `control` to that target's
already-authenticated SSH control socket. Do not reuse one target's socket for
another target. SSH host-key checking stays enabled.

```sh
python3 integration/lan.py --inventory /private/path/lan.json
```

The default test uses mDNS discovery, mutual TLS and TCP port 54171. The hosts
need multicast discovery and that TCP port reachable on the LAN. Choose another
unused port with `--port`. For a network without multicast, repeat with
`--explicit-peers`; that result is reported separately from mDNS validation.
The runner does not modify firewalls or install a background service.

The workload includes edits from every host, a 64 MiB binary transfer (adjustable
with `--size-mib`), an in-place binary patch, a 32 MiB growing file from every
host, completion verification, a project-directory move, offline deletion and
restart, simultaneous offline edits, conflict resolution and final whole-tree
hash/mode equality. Transfer timings include native file generation, SSH control
polling and hash verification, so they are end-to-end observations, not raw link
throughput. Logs and the full report are saved in the printed private controller
directory, together with the temporary test credentials. Archive only the report
and logs when sharing evidence, not its private keys.

## Spark and Plex result

On 2026-11-21, an ARM64 DGX Spark and an x86-64 Linux Plex host passed all 20
checks over their directly connected wired LAN. Both used native processes,
mDNS and mutual TLS. The 64 MiB static transfer sent exactly 64 MiB of application
payload and converged in 4.244 seconds including the controller's validation.
Each host's 32 MiB growing transfer reached its peer while its writer was active.
All test processes stopped afterward. A sanitized report is retained in
`results/native-lan-two-peer.json`; host addresses and home paths are omitted.

This establishes a two-machine Linux LAN result. Native macOS and three-machine
LAN behavior are not yet validated. To add a Mac, add an inventory entry with its
LAN address and SSH target, or run this controller locally on the Mac and make
the Spark and Plex entries remote. Use Python 3 and working SSH access on each
host. The runner builds the appropriate Darwin ARM64/amd64 binary automatically.

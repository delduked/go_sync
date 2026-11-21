<p align="center">
  <img src="docs/media/hero.png" alt="GoSync — One folder. Every machine. Keep local copies in step across your LAN, from any peer." width="1280">
</p>

<p align="center">
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.25%2B-66edc1?style=flat-square&labelColor=111c2d" alt="Go 1.25 or newer"></a>
  <a href="docs/GUIDE.md#trusted-lan-setup"><img src="https://img.shields.io/badge/Transport-mutual_TLS_1.3-bba0ff?style=flat-square&labelColor=111c2d" alt="Mutual TLS 1.3"></a>
  <a href="docs/results/README.md"><img src="https://img.shields.io/badge/Docker-10_peers_tested-ffc477?style=flat-square&labelColor=111c2d" alt="Ten peers tested in Docker"></a>
</p>

<p align="center">
  <b>Edit anywhere. Sync while downloading. Keep a local copy on every peer.</b><br>
  GoSync mirrors folders between trusted computers on your LAN.
</p>

<p align="center">
  <a href="#see-it-sync">See it sync</a> ·
  <a href="#build-your-own-setup">Your setup</a> ·
  <a href="#get-started">Get started</a> ·
  <a href="#go-deeper">Documentation</a>
</p>

## See it sync

Create a file on **Atlas**. Edit it on **Orbit**. Rename it on **Vault**.
The change follows you to the other machines, including when you delete it.

<picture>
  <source media="(prefers-reduced-motion: reduce)" srcset="docs/media/three-peers-poster.png">
  <img src="docs/media/three-peers.gif" alt="Three Docker peers: Atlas creates ideas.md, Orbit edits its contents, Vault renames it to launch.md, and Atlas deletes it. All three copies match after each operation." width="1280">
</picture>

*Actual three-container GoSync run, using mDNS and mutual TLS. The display visualizes
recorded file states; GoSync itself is a headless service. Playback is slowed for
readability.* [Still image](docs/media/three-peers-poster.png) · [Reproduce the demo](docs/demo/README.md)

| ✏️ Work from any peer | 🌊 Start sharing sooner | 🌿 Grow beyond two machines |
|:---|:---|:---|
| Code, photos, music, binaries. Create, edit, rename, and delete regular files. | Growing downloads replicate as verified prefixes while the source is still writing. | Cascading transfers let peers relay content. Ten-peer operation is tested. |

### Still downloading. Already arriving.

A large download starts on one machine. The others begin receiving it **before the
writer closes**. When it finishes, every copy has the same bytes.

<picture>
  <source media="(prefers-reduced-motion: reduce)" srcset="docs/media/growing-download-poster.png">
  <img src="docs/media/growing-download.gif" alt="A 16 MiB binary download grows on Atlas while verified prefixes appear on Orbit and Vault. Both replicas grow before the source writer closes; all final SHA-256 hashes match." width="1280">
</picture>

*Recorded local file sizes from the same Docker run, with slower playback. The demo
knows its planned 16 MiB size; applications can use an explicit completion marker
when they need to distinguish a partial download from a finished file.*
[Still image](docs/media/growing-download-poster.png) · [Transfer behavior](docs/GUIDE.md#transfer-and-recovery-behavior)

## Build your own setup

<img src="docs/media/topologies.png" alt="Two setups: three computers each keep a local folder and can originate changes; or a laptop accesses a NAS through SMB/NFS while GoSync replicates the NAS folder to two other storage machines." width="1280">

**Your workspace, on every computer.** Put a GoSync peer beside each local folder.
Work on one machine, let the changes arrive, then pick up on another. Each computer
keeps a full copy. Disconnected peers catch up when they reconnect; concurrent
edits are preserved as conflicts for you to resolve.

**Replicated storage behind a NAS.** Serve a folder through your existing SMB or
NFS service and use GoSync to keep copies on other storage machines. A replica can
also be a computer you work on. This supplies the replication layer: pooled
capacity, a single shared mount, and automatic NAS failover need separate services.

## What comes with it

| Feature | What it does |
|:---|:---|
| **LAN discovery** | Finds peers with mDNS. Explicit peer addresses work where multicast is unavailable. |
| **Trusted connections** | Mutual TLS 1.3, per-peer identities, and named shared folders. |
| **Verified transfers** | Hash-checked snapshots become visible atomically on receivers. Text and binary files use the same transfer path. |
| **Incremental updates** | Reuses matching fixed-size blocks for edits and appends, with resumable interrupted transfers. |
| **Cascading distribution** | A preferred relay tree with default fanout two spreads upload work; fallback handles missing relays. |
| **Conflict recovery** | Preserves concurrent versions, remembers deletions, and offers history, inspection, and restore commands. |
| **Project-friendly folders** | Shared `.gosyncignore` rules, nested directories, dotfiles, and executable permissions. |
| **Operational controls** | Transfer limits, upload throttling, status, rescans, saved configuration, Docker, and a Linux user service. |

## Get started

Requires **Go 1.25+**. Build the service:

```sh
git clone https://github.com/delduked/go_sync.git
cd go_sync
make build
```

**Try the three-peer demo locally.** With Docker Compose and Python 3 installed:

```sh
python3 docs/demo/capture.py
```

This builds the current code, starts three disposable containers, runs the file
operations above, verifies the results, and removes its containers and network.
It uses fresh test folders and identities. The [demo guide](docs/demo/README.md)
explains how to regenerate the GIFs.

<details>
<summary><b>Set up trusted peers on your LAN</b></summary>

On a provisioning machine, create a private authority and a separate identity
bundle for each peer. Keep these outside your repository:

```sh
./gosync trust init --out "$HOME/gosync-setup/authority"
./gosync trust issue --authority "$HOME/gosync-setup/authority" \
  --name atlas --out "$HOME/gosync-setup/atlas-identity"
./gosync trust issue --authority "$HOME/gosync-setup/authority" \
  --name orbit --out "$HOME/gosync-setup/orbit-identity"
```

Transfer each peer's own identity bundle to that machine through a trusted
channel. Keep the authority's signing key private. On Atlas, create a configuration
for a fresh, empty shared folder:

```sh
./gosync init --config "$HOME/gosync-setup/gosync.json" \
  --sync-folder "$HOME/Shared" --state-dir "$HOME/gosync-state" \
  --identity "$HOME/gosync-setup/atlas-identity" --folder-id studio
./gosync --config "$HOME/gosync-setup/gosync.json"
```

Repeat on Orbit with `orbit-identity` and the **same folder ID**. Add more peers the
same way. Each peer needs its own persistent state directory outside the shared
folder. mDNS finds peers; their certificates authorize access.

Starting with existing files? Choose the appropriate [bootstrap policy](docs/GUIDE.md#run).
See the [operating guide](docs/GUIDE.md) for explicit addresses, certificate handling,
Docker deployment, and background services.

</details>

## Tested, with evidence

The [recorded qualification](docs/results/README.md) includes **70 Docker checks**
across files, secure operations, and faults; **ten peers** originating both static
and growing transfers; and **20 checks on two physical Linux machines** over a
wired LAN with mDNS and mutual TLS.

Linux amd64 and arm64 have native LAN coverage. macOS and Windows builds are
compile-checked; native runtime testing there, a three-machine physical LAN run,
and longer soak tests remain open. [Measurements and limits →](docs/PERFORMANCE.md)

GoSync is an evolving project with **eventual consistency**. Allow synchronization
to finish before switching machines when you need the latest version. Deletions
propagate too, so keep independent backups. Symlinks, special files, ACLs, and
extended attributes are outside the current mirrored file model.

## Go deeper

| | |
|:---|:---|
| [**Operating guide**](docs/GUIDE.md) | Configuration, trust, file semantics, conflicts, recovery, and service deployment |
| [**Performance & scaling**](docs/PERFORMANCE.md) | Delta transfer costs, relay measurements, resource limits, and validation boundaries |
| [**LAN testing**](docs/LAN_TESTING.md) | Repeatable two- and three-machine validation |
| [**Qualification reports**](docs/results/README.md) | Saved Docker and native LAN results |
| [**Roadmap**](docs/ROADMAP.md) | Implementation epics and acceptance criteria |
| [**Demo source**](docs/demo/README.md) | Real Docker capture, recorded observations, and reproducible artwork |

For development: `make test` runs race checks, `make integration` runs the
three-peer regression suite, and `make qualification` runs the full Docker matrix.
See the [build and test guide](docs/GUIDE.md#build-and-test) for prerequisites.

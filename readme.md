# GoSync

GoSync is a Go service that mirrors a directory between trusted peers. Peers
find one another with mDNS or explicitly configured addresses, and exchange
files over gRPC. This branch repairs the original prototype's correctness and
recovery behavior before adding streaming downloads or cascading distribution.

## Build and test

Requires Go 1.25 or newer. Generated protobuf files are checked in; `protoc` is
only needed when changing the protocol.

```sh
make build
make test         # unit/RPC tests with the race detector; requires a C compiler
make integration  # isolated three-container regression suite; requires Docker Compose and Python 3
```

The integration suite builds a static binary, starts three peers on an internal
Docker bridge, and exercises real mDNS discovery. Each peer has a different
local sync path and port. It checks byte equality, executable permissions,
empty/nested/hidden files, file and directory renames, truncation, atomic editor
saves, offline updates/deletions, restart recovery, and clean shutdown. It removes
its containers/network in a `finally` block and prints the temporary location of
its logs, fixtures, and JSON results. The test image and build cache remain reusable.

## Run

Use a separate persistent state directory for every peer, outside its sync folder.
The folder is created if missing. State contains the peer's identity, file
revisions, deletion records, and retained overwritten file contents. Do not copy
one peer's state directory to another running peer or delete it to reset a peer:
doing so loses deletion history and can reintroduce old files.

For an isolated local test:

```sh
./gosync --sync-folder ./files-a --state-dir ./state-a --port 50051 --insecure
./gosync --sync-folder ./files-b --state-dir ./state-b --port 50052 --insecure
```

mDNS uses `_gosync-v2._tcp` and the peer's persistent identity and advertised port.
For networks where multicast is unavailable, configure both directions explicitly:

```sh
./gosync --sync-folder ./files-a --state-dir ./state-a --port 50051 \
  --no-mdns --peers 127.0.0.1:50052 --insecure
./gosync --sync-folder ./files-b --state-dir ./state-b --port 50052 \
  --no-mdns --peers 127.0.0.1:50051 --insecure
```

Other flags:

| Flag | Default | Purpose |
|---|---|---|
| `--sync-interval` | `1s` | Periodic reconciliation and recursive scan interval |
| `--chunk-size` | `64` | Transport chunk size in KiB; 1–1024 |
| `--debug` | false | Log discovery and retry failures |
| `--tls-cert`, `--tls-key`, `--tls-ca` | unset | Mutual TLS credentials |
| `--insecure` | false | Explicitly permit unauthenticated plaintext for isolated tests |

Local roots may differ. Wire paths are relative and accessed through `os.Root`,
which confines operations even if a path is replaced with an escaping symlink.
Symlinks and special files are not synchronized. Dotfiles are included. Only the
reserved `.gosync-stage-` prefix is excluded from synchronization.

## Trusted LAN setup

Use a private CA dedicated to peers that are allowed to read and modify this
shared directory. mDNS discovers addresses; it does not authorize peers. All peers
sharing this CA belong to the same trust group; there are no per-folder ACLs yet.
Each peer needs its own certificate/key, a DNS SAN of `gosync`, and both server
and client authentication usages. Mutual TLS requires TLS 1.3 and rejects peers
without a trusted certificate. There is no interactive pairing UI yet.

Example certificate setup using OpenSSL (keep the CA private key on the signing
machine; repeat the peer-key/CSR/signing commands with unique names for each peer):

```sh
umask 077
openssl req -x509 -newkey ed25519 -nodes -keyout ca.key -out ca.crt \
  -days 365 -subj '/CN=GoSync private CA' \
  -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign'
openssl req -newkey ed25519 -nodes -keyout peer-a.key -out peer-a.csr -subj '/CN=peer-a'
printf 'subjectAltName=DNS:gosync\nextendedKeyUsage=serverAuth,clientAuth\nkeyUsage=digitalSignature\n' > peer.ext
openssl x509 -req -in peer-a.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out peer-a.crt -days 90 -extfile peer.ext
./gosync --sync-folder ./files-a --state-dir ./state-a \
  --tls-cert peer-a.crt --tls-key peer-a.key --tls-ca ca.crt
```

## Transfer and recovery behavior

Filesystem events prompt recursive scans; periodic scans catch missed events.
A SHA-256 digest and file length detect content changes. Badger persists a logical
revision and originating peer ID, independently of wall-clock time. Deletions
are persistent tombstones, so a returning peer can learn that a file was removed.
Renames currently synchronize as deletion of the old path plus creation of the
new path, rather than a bandwidth-saving rename operation.

Peers reconcile manifests repeatedly. A newer revision is fetched in bounded
chunks, staged next to its destination, and checked for exact length and SHA-256
before an atomic rename. Empty files work without data chunks. Truncation is
implicit in replacing the whole file. The receiver rechecks its current local
state before committing, so an edit made during a transfer cancels that commit.
Errors leave the revision pending for retry instead of dropping a queued update.
One unavailable peer does not block other peer workers. Cancellation stops
connections, filesystem watches, and the gRPC server before closing the database.

Concurrent same-path edits converge using the deterministic `(revision, origin)`
ordering; this is **not** a semantic merge or a wall-clock last-writer rule. Before
an incoming revision overwrites or deletes a regular local file, its old contents
are retained under `STATE/versions/SHA256(relative-path)-SHA256(contents)`.
These backups are local to the overwritten peer, are not synchronized, and have
no automatic expiry. They preserve losing content but do not provide an automatic
conflict-resolution UI. Local edits/deletions made by the user are not themselves
versioned backups. Keep state backed up when testing valuable files.

## Compatibility and remaining work

Protocol v2 intentionally rejects the old v1 push RPCs, which could write paths
outside the configured folder. Upgrade all peers together. The default state
folder is now `badgerdb-v2`; v1 metadata is not migrated. The original public Go
service internals were replaced as part of this lifecycle repair.

This is a correctness baseline, not the completed download-streaming design:

- Every changed file is currently transferred in full, using bounded chunks.
  Delta transfer, bounded partial-download replication, and cascade/swarm routing
  are future work. Whole-file SHA-256 rescans favor correctness over performance.
- A file changing continuously may fail snapshot validation and retry until it
  stabilizes. Partial files are never published as completed destination files.
- Manifests are currently a single RPC response (gRPC's default 4 MiB limit).
  Pagination is needed before very large directory trees.
- Directory permissions are local; regular-file permission bits are copied.
  Ownership, timestamps, ACLs, hard links, symlinks and sparse-file layout are not
  replicated. Atomic replace has been exercised on Linux; other OSes need testing.
- File commits are atomic, but a multi-file project update or rename is not a
  transaction. Network partitions allow temporary divergence.
- Tombstones and backup history have no garbage collection yet. A peer with lost
  state cannot distinguish an old orphaned copy from a newly created file.
- Docker tests establish behavior on one host, not LAN throughput or scalability.

Implementation entry points are `cmd/server/main.go`, `internal/servers/file.go`
(scans, confinement, atomic apply), `meta.go` (persistent revisions), `grpc.go`
(verified transfer), `conn.go` (reconciliation/retry), and `mdns.go` (discovery).

To regenerate protobufs, install `protoc`, run `make install` for the pinned Go
plugins, put `$(go env GOPATH)/bin` on PATH, and run `make generate_proto`.

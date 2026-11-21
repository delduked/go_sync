# GoSync

GoSync is a Go service that mirrors a directory between trusted peers. Peers
find one another with mDNS or explicitly configured addresses, and exchange
files over gRPC. This branch repairs the original prototype's correctness and
recovery behavior and adds incremental replication of growing files. Cascading
distribution remains a separate planned feature.

## Build and test

Requires Go 1.25 or newer. Generated protobuf files are checked in; `protoc` is
only needed when changing the protocol.

```sh
make build
make test         # unit/RPC tests with the race detector; requires a C compiler
make benchmark    # repeatable append/overwrite/truncate payload and logical-I/O costs
make integration  # isolated three-container regression suite; requires Docker Compose and Python 3
```

The integration suite builds a static binary, starts three peers on an internal
Docker bridge, and exercises real mDNS discovery. Each peer has a different
local sync path and port. It checks byte equality, executable permissions,
empty/nested/hidden files, file and directory renames, truncation, atomic editor
saves, offline updates/deletions, restart recovery, and clean shutdown. It also
checks that both replicas grow during a continuous download and that a same-size
binary patch propagates from another peer. It removes
its containers/network in a `finally` block and prints the temporary location of
its logs, fixtures, and JSON results. The test image and build cache remain reusable.

## Run

Use a separate persistent state directory for every peer, outside its sync folder.
The folder is created if missing. When a new state directory is paired with a
nonempty folder, choose an explicit bootstrap policy:

- `--bootstrap seed` publishes the existing contents as intentional local data.
  Use it when establishing the first source.
- `--bootstrap join` moves existing top-level entries into a protected local
  `.gosync-local-prejoin-ID` archive before joining the shared history. This keeps
  stale copies from silently resurrecting deletions after state loss. Stop other
  writers during this per-entry archival step. Normal restarts retain the same
  identity and do not repeat bootstrap.

Private `.gosync-local-` archives are neither advertised nor remotely writable.
They are retained for manual inspection/restoration and are never pruned as sync
history. State contains the peer's identity, file
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
| `--repair-interval` | `1m` | Maximum cache age before full content verification |
| `--sync-interval` | `1s` | Periodic reconciliation and recursive scan interval |
| `--chunk-size` | `64` | Delta block size requested by this peer, in KiB; 1–1024 |
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
A SHA-256 digest and file length detect content changes. Unchanged file digests
and canonical 64 KiB block hashes are cached in memory after verifying file
identity, size, permissions and modification time. Watch events invalidate the
cache; `--repair-interval` forces periodic full verification even when an edit
preserves its timestamp or an event is missed. Startup begins with an empty cache.
Destination commits always rehash local contents and never trust this cache. Badger persists a logical
revision and originating peer ID, independently of wall-clock time. Deletions
are persistent tombstones, so a returning peer can learn that a file was removed.
Renames currently synchronize as deletion of the old path plus creation of the
new path, rather than a bandwidth-saving rename operation.

Peers reconcile manifests repeatedly. For a newer revision, the receiver seeds
an isolated staging file from its current local bytes and streams their block
hashes to the sender in small batches. The sender transmits only blocks whose
SHA-256 differs at the same offset. The assembled staging file is checked for
exact length and whole-file SHA-256 before an atomic rename. Neither unverified
chunks nor an interrupted transfer are written into the live destination. Empty
files work without data chunks. Truncation is
implicit in replacing the whole file. The receiver rechecks its current local
state before committing, so an edit made during a transfer cancels that commit.
Errors leave the revision pending for retry instead of dropping a queued update.
One unavailable peer does not block other peer workers. Cancellation stops
connections, filesystem watches, and the gRPC server before closing the database.

A scan captures the file length at its start, hashes exactly that bounded prefix,
and allows the file to grow beyond it. The sender serves that prefix even if a
newer append revision has already been observed. Changes within the requested
prefix cause a hash failure and retry rather than publishing inconsistent bytes.
Receivers therefore see successive verified prefixes while an append-only
writer continues. File size and zero-filled preallocated ranges do not imply completion; a visible
replica may still be shorter than the source. A producer can explicitly mark a
finished file, and receivers can verify that exact version:

```sh
# Run after the producer closes its writer:
./gosync complete --file ./files-a/download.bin
# On any receiver; exits nonzero until both the marker and matching bytes arrive:
./gosync verify-complete --file ./files-b/download.bin
```

The `.gosync-complete.json` sidecar contains the filename, size and SHA-256 and is
synchronized like an ordinary file. Its presence alone is not proof of completion:
verification hashes the local file. A later edit invalidates the old marker.
Preallocated downloads can fill blocks in any order; unfilled zeros remain
ordinary bytes until changed. A producer must decide when its work is finished. Existing file handles on
the receiving machine keep their old inode after atomic replacement; reopen the
path to see the latest snapshot.

The regression suite measures actual file-data RPC payload: a one-byte edit in a
9 MiB file sends one 64 KiB block; appending 1,000 bytes at an aligned boundary
sends 1,000 bytes; a prefix-only truncation sends no file data. Block hashes and
protocol framing add traffic beyond those payload figures. Appending to a
partial final block retransmits that block; inserting bytes can shift many later
blocks and require substantial retransmission. This is fixed-block delta sync,
not a rolling-window/rsync matching algorithm.

Per-file version vectors distinguish causal successors from independent edits.
Concurrent same-path edits choose a deterministic `(revision, origin)` winner;
this is not a semantic merge or a wall-clock last-writer rule. Differing losing
contents are preserved in `.gosync-conflicts/ID/data`, with the original path and
revision in `details.json`. These conflict artifacts synchronize to every peer.
Merged histories ensure that a later edit made after seeing both versions is a
normal successor. Identical concurrent contents merge histories without creating
extra copies. Older v2 records without vectors are treated conservatively.
Edit/delete conflicts preserve either the edited bytes or a deletion notice.
A directory deletion or file replacement cannot erase retained local children:
the directory is kept as a causally newer revision and the incoming operation
(or replacement file) is preserved in the conflict folder. Parent/type conflicts
may need another reconciliation pass before their children can transfer.

Ordinary overwritten/deleted file contents are also retained locally under
`STATE/versions/SHA256(relative-path)-SHA256(contents)`.
A same-origin update that preserves the old file as an exact prefix (and keeps
its permissions) does not create a full backup: all previous bytes still exist
in the new version. This prevents a growing download from accumulating a full
backup for every intermediate length. Overwrites, deletions, and differing-origin
updates still retain the previous contents.
These backups are local to the overwritten peer, are not synchronized, and have
no automatic expiry. They preserve losing content but do not provide an automatic
conflict-resolution UI. Local edits/deletions made by the user are not themselves
versioned backups. Keep state backed up when testing valuable files.

## Compatibility and remaining work

Protocol v2 intentionally rejects the old v1 push RPCs, which could write paths
outside the configured folder. All peers must run at least v2. The new `ReadDelta`
RPC is additive: updated receivers fall back to full-file `ReadFile` transfers
when an older v2 peer returns `Unimplemented`; old v2 receivers still work with
updated senders. Incremental/growing-file guarantees require updated peers.
The default state folder is now `badgerdb-v2`; v1 metadata is not migrated. The original public Go
service internals were replaced as part of this lifecycle repair.

Remaining limitations:

- Delta sync reduces network payload, but verification of changed files, staging copies, and
  verification still perform substantial local disk I/O. RAM for a transfer's
  basis hashes grows with its block count. The in-memory block index reduces repeated scans. Linux attempts copy-on-write
  staging on reflink-capable filesystems, otherwise it copies the basis normally.
  Prefix preservation is checked during final verification without another pass.
- Append-only writers can keep running during replication. Continuously rewriting
  existing bytes can invalidate snapshots and postpone convergence until those
  bytes stabilize. Preallocated/out-of-order downloads have no "which ranges are
  complete" metadata; their zero-filled ranges are treated as ordinary bytes.
- Interrupted delta transfers retain private staging data and a journal under
  `STATE/transfers`. After reconnect or restart, every retained block is rehashed
  and compared with the source; corrupted or stale blocks are retransmitted.
  A newer revision can reuse a valid earlier prefix. Only one transfer per path
  writes staging at a time. Successful commits, deletions and invalid transfers
  clean up staging. Transient interruptions retain at most one partial file per
  path; pending data has no automatic age-based expiry yet. Legacy v2 full-file
  fallback cannot resume at block granularity.
- Cascading/swarm routing and bandwidth scheduling remain future work.
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
(verified transfer), `delta.go` (block comparison and incremental transport),
`conn.go` (reconciliation/retry), and `mdns.go` (discovery).

To regenerate protobufs, install `protoc`, run `make install` for the pinned Go
plugins, put `$(go env GOPATH)/bin` on PATH, and run `make generate_proto`.

## Transfer measurements

Each running service atomically exports `STATE/metrics.json` every second and
once after shutdown. Counters include attempts/failures, aggregate transfer time,
file payload sent/received, reused bytes, and logical regular-file reads/writes
for scanning, transfer, staging, verification and backups. They reset at restart.
They exclude protocol framing, TLS, database I/O and filesystem metadata, and do
not measure physical device traffic. Live counters are sampled independently.

`make benchmark` measures a 2 MiB file using append (1 KiB), overwrite (one byte),
and truncation (to 1 MiB). Setup is excluded. The `converge-ms/op` field covers
edit detection through destination commit. See `docs/PERFORMANCE.md` for baseline
measurements and subsequent comparisons.

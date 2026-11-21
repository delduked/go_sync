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
- History pruning is explicit, using the command below. Compact deletion floors
  retain per-path causal information indefinitely; forgetting all deletion history
  cannot be made safe for arbitrary offline peers. Fresh-state joins require an
  explicit bootstrap policy. Compacted-floor recovery requires updated peers or
  bidirectional reconciliation with older v2 peers.
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

## Retiring old history

Stop the service before using its state directory from another process:

```sh
./gosync prune --sync-folder ./files-a --state-dir ./state-a --older-than 720h
```

This removes aged local backups and abandoned partial transfers and compacts
aged tombstones out of ordinary manifests. It never prunes synchronized conflict
artifacts or private bootstrap archives. Active transfers are skipped. JSON output
reports counts of each operation. Backups without an expired age are retained.

Compact deletion floors remain in Badger and preserve the logical clock and
causal context. When a returning peer mentions an old path, the deletion becomes
active again and is sent to that peer. Thus an offline copy cannot silently
resurrect just because retention ran. Deliberately recreating a file inherits its
retained history and is treated as a new edit. Retention is not automatic by default.

### Project exclusions

Place a `.gosyncignore` in the shared root. It synchronizes before other changes.
For example:

```text
node_modules/
.git/
/dist/
**/*.tmp
```

Patterns without `/` match names at any depth. A leading `/` anchors at the root;
other slash-containing patterns are also root-relative. `*`, `?`, character classes
and whole-component `**` are supported; a trailing `/` matches directories and
all descendants. Blank lines and `#` comments are ignored. Negation and backslash
escapes are unsupported and reported as configuration errors. This is a small
explicit glob language, not the full Git ignore language.

Excluded paths stay on disk and are neither advertised, served nor overwritten.
Adding a rule does not remove replicas already present. Removing a rule makes
those local files eligible again; divergent edits can create conflicts. The
policy and `.gosync-conflicts` cannot exclude themselves. Install the policy
before starting peers if the excluded data must never be shared.

Unchanged copies and renames reuse matching local content or retained versions.
The receiver rehashes those bytes and verifies the full result; an unchanged move
can require zero file-content payload. A move combined with an edit may require
more data when no matching basis remains. Renames are represented as a deletion
and creation, so a whole directory move is eventually consistent, not one atomic
operation across machines.

### Filesystem semantics

Regular files, empty files, directories and binary bytes are supported. Regular
file permission bits and modification times (millisecond precision) propagate;
a timestamp-only change reuses existing content. Directory modes and timestamps
are local, with new directories created as `0755`. Ownership, ACLs, extended
attributes, creation/access times, hard-link relationships and sparse allocation
are not mirrored. Hard links are treated as independent regular files.

Symlinks, sockets, devices and named pipes stay local and are omitted from the
manifest. They do not block unrelated files. Remote changes cannot overwrite
these objects or traverse a symlink parent, even when it points inside the root.
Replace or exclude an unsupported object explicitly to resolve a blocked path.
File publication is atomic; whole-tree operations are eventually consistent.

Large project manifests and known-path requests use bounded streaming batches
between updated peers. Older v2 peers use the unary fallback and remain subject
to its 4 MiB message limit. Metadata still occupies memory proportional to the
number of tracked paths and causal identities; streaming removes the per-message
limit, not those total resource costs.

Names must be UTF-8 in Unicode NFC form, use `/` on the wire, and fit 255 bytes
per component. Windows device names, trailing dots/spaces, control characters
and `< > : " \\ | ? *` are rejected. Unicode case-fold collisions are rejected,
including untracked destination files; case-only renames should use an
intermediate distinct name. An ambiguous local tree stops manifest publication
with an error until renamed or excluded, rather than selecting one spelling.
Linux is runtime-tested in Docker. Windows amd64 and macOS arm64 are build-checked;
watcher behavior, filesystem timestamp/mode differences and service installation
on those systems still require native validation. Windows does not provide POSIX
executable-bit semantics. This is regular-file mirroring, not a filesystem image.

Peers exchange a small node/generation announcement each interval and fetch
manifests when either side changes. Chunk retrieval remains a separate verified
RPC. Generations include a process-session token, so a restart forces fresh
reconciliation. Older v2 peers fall back to periodic full manifests.

### Cascading transfers

`--fanout 2` (default) selects a deterministic tree rooted at each revision's
origin. All peers exchange small announcements; a receiver normally fetches file
content from its parent in that tree. `--fanout 0` selects the full-mesh baseline.
The data is independently hashed and verified at each relay. Per-path transfer
serialization suppresses duplicate downloads after another source succeeds.

After at least three seconds without its preferred relay's revision, a receiver
can use any authenticated holder. This supports departed origins, partitions and
unequal discovery views. The configured fanout is a preferred topology, not a
hard upload cap during recovery or membership changes. Metadata discovery still
uses a full mesh; large populations require additional control-plane scaling.

`--transfers 2` bounds concurrent uploads and downloads separately.
`--upload-limit BYTES_PER_SECOND` caps aggregate outgoing file payload (zero is
unlimited), with a burst of at most one configured chunk. Protocol overhead is
not included. Updated delta senders yield after `--transfer-quantum` payload
bytes (default 8 MiB), retaining the receiver's verified-on-reuse partial data.
At low upload limits, the quantum shrinks to about two seconds of payload, with
one chunk as the minimum. Small files are scheduled first within each directory
depth, so a large transfer does not indefinitely hold a slot. Hashing and local
I/O still cost time, and legacy full-file senders cannot yield this way.

### Provision trusted peers

```sh
gosync trust init --out ./authority
gosync trust issue --authority ./authority --name host-a --out ./host-a-identity
gosync trust issue --authority ./authority --name host-b --out ./host-b-identity
```

Keep the authority directory private and offline when unused. Transfer each
peer's identity directory through a trusted channel to that peer only; compare
the CA fingerprint through a trusted channel when pairing. Each bundle contains
its own private key, a certificate for mutual TLS, and the public CA certificate.
It does not contain the CA signing key. Existing output directories are rejected.
Certificates default to one year; `--days` changes their lifetime. To renew, issue
a new bundle into a new directory and restart the peer using it. Certificate
revocation lists are not implemented: remove trust in the old CA and re-provision
remaining peers if a trusted key is compromised. Discovery alone grants no trust.

### Save a configuration

```sh
gosync init --config ./host-a.json --sync-folder /data/project \
  --state-dir /data/gosync-project-state --identity ./host-a-identity
# Use the printed folder ID on every other peer:
gosync init --config ./host-b.json --sync-folder /other/project \
  --state-dir /other/gosync-project-state --identity ./host-b-identity \
  --folder-id THE_SAME_FOLDER_ID
gosync --config ./host-a.json --bootstrap seed
gosync --config ./host-b.json
```

For a new peer whose root already contains files, deliberately select `seed` or
`join` as described above. Leave that one-time bootstrap choice out of persistent
configuration so lost state requires a new decision. Subsequent starts need only
`gosync --config FILE`. Config keys are long flag names without `--`; durations
are strings such as `"500ms"`, booleans/numbers are JSON values, and explicit flags
override the file. Unknown keys fail. Relative filesystem paths resolve relative
to the configuration file. Generated files and identity directories are private.

The folder ID is persisted with node history and cannot be changed in place.
mDNS filters by folder ID, and every RPC checks it, including explicit peer
connections. It is a namespace, not a secret or a substitute for mutual TLS:
all certificates signed by the configured CA are trusted. Old unnamed folders
remain compatible with the empty namespace; name them explicitly to isolate them.
Run one configured service per shared folder, using a distinct port/state directory.

### Check service health

`gosync status --state-dir /path/to/state` reads the service's private, atomic
`status.json` without locking its database. It reports node/folder identity,
scan errors, unsupported paths, conflict and partial-transfer counts, active
transfers, payload counters, and each peer's last attempt/success/error. Active
transfer progress distinguishes received payload from total target size; reused
bytes and preallocated staging lengths are not presented as verified progress.
Snapshots update once per second and on shutdown; `stale: true` means a running
snapshot is over five seconds old. This is observed local health, not proof that
an unreachable peer is current. Peer failures retry automatically. Check TLS
paths/expiry, folder ID and reachability for persistent errors; inspect
`.gosync-conflicts/*/details.json` to find the original path of a preserved edit.

### Run in the background

On Linux with a user systemd session:

```sh
make install install-service
# Save the configuration at ~/.config/gosync/project.json first.
systemctl --user daemon-reload
systemctl --user enable --now gosync@project
journalctl --user -u gosync@project -f
systemctl --user restart gosync@project
systemctl --user stop gosync@project
```

The unit restarts failures and sends SIGTERM for a graceful stop. It uses the
user's permissions; enable user lingering separately if it must run after logout.
No service is installed or enabled merely by building. `make install-tools`
installs the protobuf generators. `make dist` builds amd64/arm64 binaries for
Linux, macOS and Windows; native macOS launchd/Windows service installation is
not supplied or runtime-validated.

`docker build -t gosync:local .` builds the production image. The example
`deploy/compose.yaml` mounts configuration/identity read-only and data/state
separately. Set `GOSYNC_CONFIG`, `GOSYNC_IDENTITY`, `GOSYNC_FILES`, `GOSYNC_STATE`,
and the matching UID/GID before starting it. Its host networking is intended for
Linux mDNS; use explicit peers and published TCP ports where host networking or
multicast is unavailable. The Docker regression suites use isolated networks,
not host networking. Keep roots/state on persistent writable storage owned by
the configured UID. Back up private state and trust material separately.

### Inspect, verify and recover

```sh
# Online: request and wait for a full rehash (bypasses the scan cache).
gosync rescan --state-dir /data/gosync-project-state
# Stop the service for commands that open its database:
gosync inspect --sync-folder /data/project --state-dir /data/gosync-project-state
gosync verify --sync-folder /data/project --state-dir /data/gosync-project-state
gosync history --sync-folder /data/project --state-dir /data/gosync-project-state --path src/main.go
gosync restore --sync-folder /data/project --state-dir /data/gosync-project-state \
  --path src/main.go --version VERSION_ID_FROM_HISTORY
gosync restore --sync-folder /data/project --state-dir /data/gosync-project-state \
  --conflict CONFLICT_DIRECTORY_ID
```

`verify` reports differences from saved revisions and exits nonzero when it finds
any; it does not adopt disk changes. Those differences may be intentional edits,
so inspect them before restarting synchronization. `restore` checks the retained
content hash, preserves the replaced file in history, and records the restored
bytes as a new local edit that propagates on restart. Historical backups retain
bytes, not all original metadata; they restore with mode `0644`. Visible data
conflicts retain the original mode. Conflicts describing deletion or directory
operations require inspecting their details and explicitly applying the desired
local operation. Keep external writers stopped during offline recovery. Backups
are recovery aids, not a substitute for independent backups.

`make integration` runs the file/mDNS suite; `python3 integration/operations.py`
exercises real provisioned mutual TLS, configuration, namespace isolation,
status, rescan, verification, history, restoration and clean lifecycle in Docker.

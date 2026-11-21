# Implementation roadmap

Work is performed sequentially, with tests and a local commit for each story.
An epic is merged locally after its acceptance checks pass. Nothing is pushed
without a request. The user selected Docker-only validation; physical LAN testing
is explicitly deferred and is not a completion requirement for this run.

Baseline: correctness repair `4984876` and incremental growing-file transfers
`f1f5fc9` are on local main. The latter passed race tests, vet and 27 Docker checks.

## 1. Efficient large-file downloads — complete

- [x] 1.1 Measure payload, logical file I/O, reuse, latency and failures for repeatable append/overwrite/truncate workloads.
- [x] 1.2 Cache validated file/block metadata; retain periodic full repair scans.
- [x] 1.3 Reduce staging I/O without exposing unverified content.
- [x] 1.4 Persist and validate partial transfers across interruptions and restarts.
- [x] 1.5 Cover preallocation/out-of-order writes and explicit completion semantics.

Accept: replicas progress before an append-only writer finishes, recover from
interruptions, and converge byte-for-byte with measured costs.

Evidence: race suite and vet passed; Docker suite passed 29 checks, including
live growth and explicit completion verification. Restart/resume tests retained
half a transfer and sent only the missing half; corrupting one retained block
caused exactly that additional block to be retransmitted. Out-of-order writes
into a preallocated file transferred one changed block per update. Reflinks are
unsupported on this workspace filesystem; fallback is tested and that limit is
recorded in PERFORMANCE.md. Final Docker log: `/tmp/gosync-regression-_qv8chnb`.


## 2. Concurrent edits and offline recovery — complete

- [x] 2.1 Track causal histories and distinguish concurrent revisions.
- [x] 2.2 Preserve conflicting contents visibly and deterministically.
- [x] 2.3 Handle edit/delete, rename/edit, and same-path creation conflicts.
- [x] 2.4 Specify and test fresh, stale and replaced-state joining behavior.
- [x] 2.5 Retire backups and tombstones only under safe retention rules.

Accept: partition, edit independently, reconnect, converge, recover all conflicting data.

Evidence: race suite and vet passed; Docker suite passed 37 checks, including
three offline edits, preservation of every conflicting version, resolution and
a stale peer rejoining after deletion-history compaction. Safe seed/join and
retention/restart behavior have focused unit tests. Compact causal deletion
floors remain indefinitely to prevent resurrection. Docker log:
`/tmp/gosync-regression-ihwbzfoa`.

## 3. Complete project-folder mirroring — complete

- [x] 3.1 Configurable exclusions.
- [x] 3.2 Efficient moves/renames with content reuse.
- [x] 3.3 Explicit permissions, timestamps, symlink and unsupported-file policies.
- [x] 3.4 Editor/build and many-small-file workload tests.
- [x] 3.5 Portable path rules and platform-specific checks/documented limits.

Accept: representative TypeScript/Go trees behave predictably through edit/build cycles.

Evidence: full race suite and vet; 600-source-file editor/build workload;
10,000-entry streaming manifest beyond the unary 4 MiB limit; unchanged moves
and copies sent zero content bytes. All 43 Docker checks passed, including
project save bursts, exclusions, timestamps and directory moves. Windows amd64,
macOS arm64 and Go 1.25.5 builds passed; native non-Linux runtime tests are deferred.
Docker log: `/tmp/gosync-regression-5xpj5jne`.

## 4. Cascading distribution — in progress

- [x] 4.1 Ten-peer baseline harness and measurements.
- [x] 4.2 Separate revision announcements from chunk retrieval.
- [x] 4.3 Limited fanout and duplicate suppression.
- [x] 4.4 Alternate routes/sources after relay failures.
- [x] 4.5 Bounded concurrency, bandwidth and fair scheduling.
- [ ] 4.6 Exercise changes from every host, including growing files.

Accept: ten peers converge despite relay failures; measure origin upload reduction.

## 5. Easy setup and operation

- [ ] 5.1 Trusted pairing/certificate provisioning workflow.
- [ ] 5.2 Persistent config and stable shared-folder identities.
- [ ] 5.3 Status, progress, conflicts and actionable failures.
- [ ] 5.4 Background service packaging and lifecycle.
- [ ] 5.5 Inspect, rescan, verify and restore commands.

Accept: install, authorize, choose folders and determine sync health with documented commands.

## 6. Docker validation and tuning (physical LAN deferred by user)

- [ ] 6.1 Reproducible throughput/convergence baseline.
- [ ] 6.2 Latency/loss/disconnect/bandwidth fault scenarios.
- [ ] 6.3 Uneven peer/storage/CPU scenarios within available facilities.
- [ ] 6.4 Sustained workloads and resource/history growth checks.
- [ ] 6.5 Tune defaults from results; document physical-LAN and OS validation gaps.

Accept: repeatable reports distinguish tested results from simulations and untested platforms.

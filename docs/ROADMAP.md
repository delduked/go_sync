# Implementation roadmap

Work is performed sequentially, with tests and a local commit for each story.
An epic is merged locally after its acceptance checks pass. Nothing is pushed
without a request. The user selected Docker-only validation; physical LAN testing
is explicitly deferred and is not a completion requirement for this run.

Baseline: correctness repair `4984876` and incremental growing-file transfers
`f1f5fc9` are on local main. The latter passed race tests, vet and 27 Docker checks.

## 1. Efficient large-file downloads — in progress

- [x] 1.1 Measure payload, logical file I/O, reuse, latency and failures for repeatable append/overwrite/truncate workloads.
- [x] 1.2 Cache validated file/block metadata; retain periodic full repair scans.
- [x] 1.3 Reduce staging I/O without exposing unverified content.
- [x] 1.4 Persist and validate partial transfers across interruptions and restarts.
- [ ] 1.5 Cover preallocation/out-of-order writes and explicit completion semantics.

Accept: replicas progress before an append-only writer finishes, recover from
interruptions, and converge byte-for-byte with measured costs.

## 2. Concurrent edits and offline recovery

- [ ] 2.1 Track causal histories and distinguish concurrent revisions.
- [ ] 2.2 Preserve conflicting contents visibly and deterministically.
- [ ] 2.3 Handle edit/delete, rename/edit, and same-path creation conflicts.
- [ ] 2.4 Specify and test fresh, stale and replaced-state joining behavior.
- [ ] 2.5 Retire backups and tombstones only under safe retention rules.

Accept: partition, edit independently, reconnect, converge, recover all conflicting data.

## 3. Complete project-folder mirroring

- [ ] 3.1 Configurable exclusions.
- [ ] 3.2 Efficient moves/renames with content reuse.
- [ ] 3.3 Explicit permissions, timestamps, symlink and unsupported-file policies.
- [ ] 3.4 Editor/build and many-small-file workload tests.
- [ ] 3.5 Portable path rules and platform-specific checks/documented limits.

Accept: representative TypeScript/Go trees behave predictably through edit/build cycles.

## 4. Cascading distribution

- [ ] 4.1 Ten-peer baseline harness and measurements.
- [ ] 4.2 Separate revision announcements from chunk retrieval.
- [ ] 4.3 Limited fanout and duplicate suppression.
- [ ] 4.4 Alternate routes/sources after relay failures.
- [ ] 4.5 Bounded concurrency, bandwidth and fair scheduling.
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

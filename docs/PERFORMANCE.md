# Transfer cost measurements

Command: `make benchmark` (one measured operation per workload). Linux arm64,
local gRPC transport, 2 MiB initial file, default 64 KiB blocks. These figures
are an initial development-machine sample, not LAN throughput claims. Read/write
counts are logical application file I/O across source and destination; OS caching
means they are not physical storage traffic. Latency varies with host load.

## Before scan/staging optimizations (Epic 1.1)

| Operation | Payload bytes | Reused bytes | Read bytes | Written bytes | Convergence ms |
|---|---:|---:|---:|---:|---:|
| Append 1 KiB | 1,024 | 2,097,152 | 14,684,160 | 2,098,176 | 19.56 |
| Overwrite one byte | 65,536 | 2,031,616 | 16,777,216 | 4,259,840 | 25.11 |
| Truncate to 1 MiB | 0 | 1,048,576 | 9,437,184 | 3,145,728 | 19.86 |

The small network payload hides significant scanning, basis copying, verification
and backup I/O. The next stories target repeated scans and destination copying
while preserving the final integrity check and local-edit protection.

## Metadata cache (Epic 1.2)

The same workloads now read 12,585,984 / 14,680,064 / 8,388,608 bytes respectively.
Payload and writes are unchanged. An unchanged scan performs zero content reads;
periodic full verification, event invalidation and an uncached destination commit
check are covered by regression tests. Timing samples were 18.43 / 24.75 / 19.36 ms.

## Staging/verification (Epic 1.3)

Append/overwrite/truncate now read 10,488,832 / 12,582,912 / 8,388,608 bytes.
Prefix preservation shares the verification pass. Writes are unchanged on this
ext filesystem, which does not support reflinks. Linux reflink support is attempted
and falls back automatically; successful clones are separately counted in
`reflink_bytes`. The clone-isolation test is skipped when the filesystem rejects
the ioctl, and no copy-on-write performance claim is made for this machine.

## Ten-peer baseline (before cascading)

`python3 integration/mesh.py` ran ten isolated Docker peers with an 8 MiB file,
500 ms reconciliation, plaintext test transport and the existing full mesh.
One run converged in 0.577 s. The origin sent 48 MiB; relays sent another 24 MiB,
for the minimum 72 MiB total payload needed by nine new replicas. This already
shows opportunistic reuse of relays, but source choice has no fanout limit.
These are application payload counters on one machine, not LAN throughput.
Logs: `/tmp/gosync-mesh-1ynzvf7o`. The harness records per-peer bytes and cleans
up containers/networks while retaining its report and logs.

With `--fanout 2`, the same ten-peer/8 MiB workload converged in 1.023 s. The
origin sent 16 MiB (two copies), a 66.7% reduction from the observed 48 MiB
baseline; aggregate payload stayed at 72 MiB. Each relay sent at most two copies.
This single-host run demonstrates the upload/latency tradeoff, not a universal
speedup. Logs: `/tmp/gosync-mesh-5hl3fg4y`.

Stopping a preferred relay in a ten-peer fanout-two tree left the other nine
peers able to converge in 3.604 s via fallback holders. The stopped peer caught
up after restart. Logs: `/tmp/gosync-mesh-fnzrmauz`.

## Docker fault qualification (Epic 6)

`python3 integration/faults.py` runs three isolated peers. Network impairment is
applied inside a test container's network namespace, never to the host/LAN.
The test helper needs Docker `NET_ADMIN` and kernel netem support. One initial run
on Linux arm64 (20 logical CPUs) produced:

| Scenario | Result |
|---|---|
| Unimpaired 16 MiB transfer, including resumable quantum yields | 0.838 s |
| 4 MiB with 40 ± 10 ms delay, 0.5% loss, 20 Mbit/s egress cap | 3.940 s; qdisc recorded dropped packets |
| Network disconnect, offline edit/delete, reconnect | Converged |
| SIGKILL receiver during 8 MiB transfer | Reused 512 KiB; received 7.5 MiB after restart |
| Receiver limited to 0.25 CPU and 256 MiB | 2 MiB converged in 0.906 s |
| Unwritable destination then restored permissions | Error visible; automatic retry succeeded |
| 60 rotating-origin overwrites | Median 0.201 s; max 0.302 s |
| Explicit history pruning | Removed 40 backups per peer; compact deletion context retained |

Initial report: `/tmp/gosync-faults-hs1kfs8w`. The sustained segment lasted about
13.5 seconds; it is a repeated-workload regression, not a days-long soak or proof
of a memory plateau. Container memory increased by about 21–24 MiB over that
segment. State files showed about 2.28 GB of apparent size while running because
Badger preallocates/memory-maps large files; this is not a measurement of physical
disk use. The harness now records allocated and apparent bytes separately.

The final small-operation sample retained the same logical I/O/payload costs as
Epic 1.3; append/overwrite/truncate latencies were 22.65/31.95/26.09 ms. Timing
samples vary and are not regression thresholds.

## Final defaults and limits

Badger now uses 8 MiB memtables (two), 16 MiB value-log files, a 16 MiB block
cache and an 8 MiB index cache. In the repeated fault suite, running state files
had about 51.4 MB apparent size before history accumulated, versus 2.28 GB with
the original defaults. Actual allocated state storage was only 37–45 KiB before
that workload and about 1.64–1.71 MB after its retained backups. These filesystem
allocation figures exclude page cache and the mirrored data itself.

The second run passed every fault/recovery check. Median/max overwrite latency
remained 0.201/0.302 s. Observed post-workload container memory was 27–40 MiB
versus 41–140 MiB in the initial run; these are short-run snapshots affected by
GC and mmap residency, not guaranteed memory ceilings. Logs:
`/tmp/gosync-faults-uuwcbbjj`.

Keep the measured defaults of 64 KiB chunks, fanout two, two simultaneous transfers
per direction, 8 MiB delta quanta and one-minute full repair scans. Upload-rate
limits remain opt-in because appropriate bandwidth depends on the deployment.
Fallback after three seconds favors recovery over strict fanout; on a slow link
it can increase origin uploads while a relay is still downloading. Metadata
announcements remain a full mesh, block caches scale with tracked file sizes,
and compact deletion histories remain indefinitely. No unbounded-peer-count or
constant-memory claim is made.

Not covered: physical LAN/Wi-Fi throughput, native Windows/macOS filesystem and
watcher behavior, true disk-full/slow-device faults, power-loss durability, or a
multi-day soak. The storage fault test covers write permissions; the abrupt-crash
test covers process death with the kernel/filesystem still running. Successful
reflinks are untested here because the workspace filesystem rejects them.

Final ten-peer repetitions exposed intermittent gRPC 1.66.0 flow-control window
underflow errors (`received 4294967292-bytes data exceeding ...`). Recovery worked,
but abandoned streams added payload. Upstream gRPC 1.66.3 explicitly fixes this
issue ([release notes](https://github.com/grpc/grpc-go/releases/tag/v1.66.3), #7667).
The dependency is upgraded to that patch release, preserving Go 1.25 compatibility;
the extended harness now fails on unexpected Internal transport errors. Static
measurement inputs are atomically published so deliberate growing snapshots are
measured separately. All qualification suites are rerun after the patch.


The final post-patch `make qualification` run passed. Across all ten static origins,
8 MiB revisions converged in 0.841–1.516 s, with exactly 16 MiB sent by each origin
and 72 MiB total payload per workload. Every growing origin reached all nine
replicas before its writer finished. Preferred-relay failure recovered in 3.612 s.
No Internal transport errors appeared. Final reports are retained in `results/`,
including the complete network/resource suite and a machine-readable summary.

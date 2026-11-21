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

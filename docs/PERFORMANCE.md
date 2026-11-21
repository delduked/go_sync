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

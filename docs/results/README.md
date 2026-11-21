# Local qualification reports

Collected on 2026-11-21 using disposable Docker networks on Linux arm64.
Reports are preserved here so the evidence does not depend on temporary logs.
They are measurements on one machine, not physical-LAN benchmarks.

- `summary.json`: final validation matrix, versions and explicit deferred tests.
- `files.json`: 43 mDNS, file/project, conflict and recovery checks.
- `operations.json`: 13 provisioned TLS, namespace, CLI and lifecycle checks.
- `faults-before-tuning.json`: original database allocation/cache defaults.
- `faults.json`: all 14 network/process/resource/storage/history checks after tuning.
- `ten-peer-growing.json`: every origin, shared edits, growing files and relay failure.
- `ten-peer-static.json`: static revisions published atomically for payload accounting.

The pre-fix ten-peer runs occasionally retried a stream after gRPC 1.66.0
miscalculated its flow-control window. Logs identified the error; the upstream
1.66.3 patch fixes it. Final reports were regenerated after upgrading. Static
measurement inputs are published atomically; deliberate growing-file tests
still write directly. Counters include retries and abandoned attempts, so
sent/received counters can differ.

Run `make qualification` to reproduce the suites. Full temporary logs are printed
by each harness. Generated test private keys are intentionally not archived here
or uploaded by the extended CI workflow. See ../PERFORMANCE.md for scope and limits.

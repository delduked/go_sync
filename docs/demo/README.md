# The README demo

[← Back to GoSync](../../readme.md)

The animations visualize **real file observations from three GoSync containers**.
Atlas, Orbit, and Vault are presentation names for those containers. They discover
each other over mDNS and exchange data using separately issued mutual TLS
identities on an isolated Docker bridge.

GoSync is a headless service. The three-panel display is artwork for the README,
not a dashboard shipped with the application. It makes all three folders readable
at once without recording tiny terminals. All names, contents, hashes, byte counts,
and writer states come from the recorded run. Connector pulses are decorative;
they do not show measured packets or the actual relay path.

## Capture a new run

Requires Go 1.25+, Python 3, Docker, and Docker Compose. Run from the repository root:

```sh
python3 docs/demo/capture.py
```

The script builds the current Go code and starts three disposable peers, using
fresh roots, state, and certificates in a private temporary directory. It asserts:

1. All three peers discover one another with mDNS.
2. A create from Atlas, an edit from Orbit, a rename from Vault, and a delete from
   Atlas each converge to exactly the expected file names and bytes.
3. Both receivers grow more than once while the source's 16 MiB binary writer is
   still open.
4. All three final binary SHA-256 hashes match the expected content.

It saves portable observations to [trace.json](trace.json), then removes its
containers and network in a `finally` block. The image/build cache and private
temporary fixtures/logs remain for debugging; their location is printed. Test
private keys are never copied into the repository. No production folders are used,
no host ports are exposed, and the demo never installs a background service.

Use `--out /tmp/gosync-demo-check.json` to validate without replacing the committed
trace. Run one capture at a time because the build binary and Docker image tag are
shared. To inspect a failure, use the printed temporary directory's container log.

## Render the artwork

Rendering uses Pillow and Ubuntu fonts; no browser, VHS, or video encoder is
required. On Ubuntu, install the `fonts-ubuntu` package if these fonts are missing.
The renderer expects `Ubuntu-R.ttf`, `Ubuntu-B.ttf`, and `UbuntuMono-R.ttf` in
`/usr/share/fonts/truetype/ubuntu`; use `--font-dir PATH` on other systems.

```sh
python3 -m venv /tmp/gosync-demo-venv
/tmp/gosync-demo-venv/bin/pip install -r docs/demo/requirements.txt
/tmp/gosync-demo-venv/bin/python docs/demo/render.py
```

This generates the hero, topology illustration, two GIFs, and two static posters
in [docs/media](../media). The source artwork lives in [render.py](render.py).
Fonts are used for rasterization and are not bundled in the repository.

The file-operation animation holds recorded state changes long enough to read;
the download animation shows every recorded size sample with slower playback.
**Playback durations are not benchmarks.** The planned 16 MiB total is a property
of this demo, not a claim that GoSync can infer when any arbitrary writer is done.
The final equality checks happen after the writer closes. See the operating guide
for [producer completion markers](../GUIDE.md#transfer-and-recovery-behavior).

The GIFs use a shared palette and changed-region encoding to stay small. README
`picture` elements select still posters when the viewer supports reduced-motion
preferences; explicit still-image links are provided too. Image descriptions and
the surrounding prose explain each demonstration without requiring animation.

## Review after changing the demo

- Run the capture and require every assertion to pass before committing a new trace.
- Render from that trace; inspect all four operations, a mid-download frame, and
  the final matching-hash frame for clipping and accuracy.
- Check GIF frame counts/durations and README relative links. Keep the display
  readable at GitHub's content width and avoid adding network-speed claims from
  an isolated-container run.

These assets demonstrate the happy path. The separate [qualification reports](../results/README.md)
cover offline changes, conflicts, resource faults, relay failures, and ten-peer runs.

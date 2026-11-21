#!/usr/bin/env python3
"""Render README art and GIFs from capture.py observations (no invented file states)."""
import argparse
from functools import lru_cache
import json
import math
from pathlib import Path

from PIL import Image, ImageColor, ImageDraw, ImageFont

HERE = Path(__file__).resolve().parent
OUT = HERE.parent / "media"
W = 1280
BG = "#090f1c"
PANEL = "#111c2d"
INSET = "#0b1423"
LINE = "#26364d"
WHITE = "#f2f6ff"
MUTED = "#a2b2c9"
FAINT = "#657b98"
ACCENTS = ("#66edc1", "#bba0ff", "#ffc477")
NAMES = ("ATLAS", "ORBIT", "VAULT")
FONT_ROOT = Path("/usr/share/fonts/truetype/ubuntu")


@lru_cache(maxsize=None)
def font(size, kind="regular"):
    filename = {"regular": "Ubuntu-R.ttf", "bold": "Ubuntu-B.ttf", "mono": "UbuntuMono-R.ttf"}[kind]
    path = FONT_ROOT / filename
    if not path.exists():
        raise SystemExit(f"Missing {path}. Install fonts-ubuntu or pass --font-dir; see docs/demo/README.md.")
    return ImageFont.truetype(str(path), size)


def text(draw, xy, value, size=24, color=WHITE, kind="regular", anchor=None):
    draw.text(xy, value, font=font(size, kind), fill=color, anchor=anchor)


def box(draw, bounds, fill=PANEL, outline=LINE, radius=18, width=1):
    draw.rounded_rectangle(bounds, radius, fill=fill, outline=outline, width=width)


def pill(draw, xy, value, color=ACCENTS[0], fill=INSET, size=19):
    x, y = xy
    width = draw.textlength(value, font=font(size, "mono")) + 28
    box(draw, (x, y, x + width, y + 34), fill, None, 17)
    text(draw, (x + 14, y + 6), value, size, color, "mono")
    return width


def canvas(height):
    im = Image.new("RGB", (W, height), BG)
    draw = ImageDraw.Draw(im)
    # Quiet grid, deliberately flat so GIFs compress well.
    for x in range(0, W, 40):
        for y in range(0, height, 40):
            draw.point((x, y), fill="#182437")
    box(draw, (0, 0, W - 1, height - 1), None, LINE, 24)
    return im, draw


def folder(draw, x, y, color, scale=1):
    s = scale
    draw.rounded_rectangle((x, y, x + 24*s, y + 13*s), 4*s, fill=color)
    draw.rounded_rectangle((x, y + 7*s, x + 52*s, y + 40*s), 5*s, fill=color)
    draw.line((x + 10*s, y + 22*s, x + 41*s, y + 22*s), fill=INSET, width=max(1, int(2*s)))


def device(draw, x, y, color, storage=False, scale=1):
    s = scale
    if storage:
        box(draw, (x, y, x + 48*s, y + 42*s), INSET, color, int(5*s), 2)
        for h in (13, 28):
            draw.line((x + 8*s, y + h*s, x + 31*s, y + h*s), fill=color, width=2)
            draw.ellipse((x + 37*s, y + (h-2)*s, x + 41*s, y + (h+2)*s), fill=color)
    else:
        box(draw, (x, y, x + 48*s, y + 32*s), INSET, color, int(4*s), 2)
        draw.line((x - 5*s, y + 39*s, x + 53*s, y + 39*s), fill=color, width=3)
        # Friendly little terminal prompt, legible even in the tiny node icon.
        draw.line((x + 11*s, y + 11*s, x + 17*s, y + 16*s, x + 11*s, y + 21*s), fill=color, width=2)
        draw.line((x + 23*s, y + 21*s, x + 34*s, y + 21*s), fill=color, width=2)


def edge(draw, start, end, color=LINE, arrows=False):
    draw.line((*start, *end), fill=color, width=3)
    if arrows:
        angle = math.atan2(end[1] - start[1], end[0] - start[0])
        for tip, theta in ((end, angle), (start, angle + math.pi)):
            p = [(tip[0] - 10 * math.cos(theta + d), tip[1] - 10 * math.sin(theta + d)) for d in (-.5, .5)]
            draw.line((p[0], tip, p[1]), fill=color, width=3)


def hero():
    im, d = canvas(520)
    folder(d, 48, 40, ACCENTS[0], .7)
    text(d, (100, 34), "GoSync", 36, WHITE, "bold")
    text(d, (48, 124), "One folder.", 76, WHITE, "bold")
    text(d, (48, 208), "Every machine.", 76, ACCENTS[0], "bold")
    text(d, (51, 316), "Keep local copies in step.", 29, MUTED)
    text(d, (51, 356), "Across your LAN. From any peer.", 29, MUTED)
    nodes = [(930, 111), (790, 298), (1070, 298)]
    for a, b in ((0, 1), (0, 2), (1, 2)):
        edge(d, (nodes[a][0], nodes[a][1] + 40), (nodes[b][0], nodes[b][1] + 40), "#365168")
    for i, (x, y) in enumerate(nodes):
        box(d, (x - 81, y - 28, x + 81, y + 112), PANEL, ACCENTS[i], 20, 2)
        device(d, x - 24, y - 2, ACCENTS[i], storage=i == 2)
        text(d, (x, y + 60), ("DESKTOP", "LAPTOP", "STORAGE")[i], 20, WHITE, "mono", "mt")
        d.ellipse((x - 4, y + 92, x + 4, y + 100), fill=ACCENTS[0])
    x = 49
    for label in ("mDNS discovery", "Verified transfers", "Cascading replication"):
        x += pill(d, (x, 445), label, MUTED) + 12
    im.save(OUT / "hero.png", optimize=True)


def card(d, i, y=183, height=344):
    x = 36 + i * 410
    box(d, (x, y, x + 388, y + height), PANEL, LINE, 18)
    device(d, x + 24, y + 23, ACCENTS[i], storage=i == 2, scale=.7)
    text(d, (x + 80, y + 20), NAMES[i], 28, WHITE, "bold")
    text(d, (x + 80, y + 57), "docker peer / 0" + str(i + 1), 19, MUTED, "mono")
    d.line((x + 24, y + 96, x + 364, y + 96), fill=LINE, width=1)
    return x


def demo_header(d, title, subtitle, tag="3 PEERS / REAL DOCKER RUN"):
    text(d, (36, 34), title, 45, WHITE, "bold")
    text(d, (38, 100), subtitle, 24, MUTED)
    pill(d, (939, 108), tag, ACCENTS[0], size=16)


def sync_frame(op, state, step, progress, tick):
    im, d = canvas(688)
    titles = ("Create on Atlas.", "Edit on Orbit.", "Rename on Vault.", "Delete on Atlas.")
    demo_header(d, "Change it here. See it everywhere.", f"0{step + 1}  /  {titles[step]}")
    target = op["samples"][-1]["peers"][0]
    equal = all(s == target for s in state)
    for i, files in enumerate(state):
        x = card(d, i)
        if files:
            # A rename can briefly leave both entries; show that observation faithfully.
            for j, (name, details) in enumerate(files.items()):
                text(d, (x + 24, 304 + 67*j), name, 26, ACCENTS[i], "mono")
                text(d, (x + 24, 341 + 67*j), details["text"].strip(), 24, WHITE, "mono")
            if len(files) == 1:
                details = next(iter(files.values()))
                text(d, (x + 24, 397), "sha256  " + details["sha256"][:12], 19, MUTED, "mono")
        else:
            text(d, (x + 24, 314), "/files/", 26, MUTED, "mono")
            text(d, (x + 24, 360), "No demo files", 25, FAINT)
        if equal:
            label, color = "ALL COPIES MATCH", ACCENTS[0]
        elif i == op["origin"]:
            label, color = "LOCAL CHANGE", ACCENTS[i]
        elif files == target:
            label, color = "CHANGE ARRIVED", ACCENTS[0]
        else:
            label, color = "AWAITING CHANGE", MUTED
        pill(d, (x + 24, 465), label, color)
    # Connectors are decorative, not a packet capture or a claimed transfer route.
    edge(d, (230, 566), (1050, 566), LINE)
    for i in range(3):
        x = 230 + i * 410
        d.ellipse((x - 6, 560, x + 6, 572), fill=ACCENTS[0] if equal else ACCENTS[i])
    if not equal:
        x = 230 + (tick % 12) / 11 * 820
        d.ellipse((x - 4, 562, x + 4, 570), fill=WHITE)
    labels = ("01  CREATE", "02  EDIT", "03  RENAME", "04  DELETE")
    for i, label in enumerate(labels):
        x = 36 + i * 309
        d.line((x, 613, x + 281, 613), fill=LINE, width=3)
        if i <= step:
            amount = 1 if i < step else progress
            d.line((x, 613, x + int(281 * amount), 613), fill=ACCENTS[0], width=3)
        text(d, (x, 633), label, 20, WHITE if i == step else FAINT, "mono")
    return im


def growth_frame(growth, sample, tick):
    im, d = canvas(658)
    data = sample["peers"]
    sizes, writer_open = data["bytes"], data["writer_open"]
    done = not writer_open and all(n == growth["target_bytes"] for n in sizes)
    demo_header(d, "Still downloading. Already arriving.", "A 16 MiB binary file, written in chunks on Atlas.")
    for i, n in enumerate(sizes):
        x = card(d, i, height=348)
        text(d, (x + 24, 304), "download.bin", 26, ACCENTS[i], "mono")
        text(d, (x + 24, 353), f"{n / 1048576:04.1f}", 53, WHITE, "bold")
        text(d, (x + 168, 379), "/ 16 MiB", 24, MUTED, "mono")
        box(d, (x + 24, 429, x + 364, 440), INSET, None, 5)
        width = int(340 * n / growth["target_bytes"])
        if width > 2:
            box(d, (x + 24, 429, x + 24 + width, 440), ACCENTS[i], None, 5)
        label = "SHA-256 MATCH" if done else (("WRITER OPEN" if writer_open else "WRITER CLOSED") if i == 0 else ("VERIFIED PREFIX" if n else "AWAITING BYTES"))
        pill(d, (x + 24, 468), label, ACCENTS[i] if not done else ACCENTS[0])
    if done:
        title = "Writer closed. All three SHA-256 hashes match."
        subtitle = "sha256  " + growth["final_sha256"][0][:24] + "..."
    elif writer_open and all(sizes):
        title = "Both replicas have bytes. The writer is still open."
        subtitle = "Verified prefixes arrive as the source file grows."
    elif not writer_open:
        title = "The writer has closed. Replicas are catching up."
        subtitle = "Waiting for the final bytes on every peer."
    else:
        title = "The download starts on Atlas."
        subtitle = "Each peer keeps its own local copy."
    text(d, (640, 561), title, 27, ACCENTS[0], "regular", "mt")
    text(d, (640, 607), subtitle, 20, MUTED, "mono", "mt")
    return im


def topology():
    im, d = canvas(658)
    text(d, (40, 36), "Your machines. Your storage. Your LAN.", 44, WHITE, "bold")
    d.line((40, 334, 1240, 334), fill=LINE)
    text(d, (40, 139), "WORK ANYWHERE", 20, ACCENTS[0], "mono")
    text(d, (40, 178), "A local folder on", 30, WHITE, "bold")
    text(d, (40, 218), "every computer.", 30, WHITE, "bold")
    text(d, (40, 272), "Changes can start at any peer.", 21, MUTED)
    for a, b in ((600, 870), (870, 1140)):
        edge(d, (a + 73, 206), (b - 73, 206), "#48677b", arrows=True)
    for i, x in enumerate((600, 870, 1140)):
        box(d, (x - 68, 143, x + 68, 258), PANEL, LINE, 16)
        device(d, x - 24, 162, ACCENTS[i])
        text(d, (x, 225), f"PEER {i + 1}", 21, WHITE, "mono", "mt")
        text(d, (x, 279), "local copy", 20, MUTED, "regular", "mt")
    text(d, (40, 381), "REPLICATED STORAGE", 20, ACCENTS[1], "mono")
    text(d, (40, 423), "Keep copies behind", 30, WHITE, "bold")
    text(d, (40, 463), "your NAS.", 30, WHITE, "bold")
    text(d, (40, 526), "SMB / NFS serves the files.", 21, MUTED)
    text(d, (40, 556), "GoSync replicates the folder.", 21, MUTED)
    edge(d, (596, 476), (750, 476), "#48677b", arrows=True)
    text(d, (670, 443), "SMB / NFS", 18, MUTED, "mono", "mt")
    edge(d, (886, 476), (1058, 414), "#48677b", arrows=True)
    edge(d, (886, 476), (1058, 555), "#48677b", arrows=True)
    text(d, (974, 471), "GoSync", 21, ACCENTS[0], "mono", "mt")
    for x, y, label, storage, color in ((530, 414, "LAPTOP", False, MUTED), (818, 414, "NAS", True, ACCENTS[0]), (1125, 352, "REPLICA", True, ACCENTS[1]), (1125, 493, "REPLICA", True, ACCENTS[2])):
        box(d, (x - 66, y, x + 66, y + 119), PANEL, LINE, 16)
        device(d, x - 24, y + 17, color, storage)
        text(d, (x, y + 82), label, 21, WHITE, "mono", "mt")
    im.save(OUT / "topologies.png", optimize=True)


def save_gif(frames, name):
    # One shared palette avoids flicker and permits small changed-region GIF frames.
    preview = Image.new("RGB", (320 * 8, 180))
    for i in range(8):
        thumb = frames[min(len(frames) - 1, int(i * len(frames) / 8))].resize((320, 180))
        preview.paste(thumb, (i * 320, 0))
    adaptive = preview.quantize(colors=128).getpalette()[:128 * 3]
    # Keep the exact theme colors even when thin glyphs disappear in thumbnails.
    for color in (BG, PANEL, INSET, LINE, WHITE, MUTED, FAINT, *ACCENTS):
        adaptive.extend(ImageColor.getrgb(color))
    palette = Image.new("P", (1, 1))
    palette.putpalette(adaptive + [0] * (768 - len(adaptive)))
    indexed = [frame.quantize(palette=palette, dither=Image.Dither.NONE) for frame in frames]
    indexed[0].save(OUT / name, save_all=True, append_images=indexed[1:], duration=100,
                    loop=0, optimize=True, disposal=1)
    print(f"Rendered {name}: {len(frames) / 10:.1f}s, {(OUT / name).stat().st_size / 1048576:.2f} MiB")


def main():
    global FONT_ROOT
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--trace", type=Path, default=HERE / "trace.json")
    parser.add_argument("--font-dir", type=Path, default=FONT_ROOT)
    args = parser.parse_args()
    FONT_ROOT = args.font_dir
    trace = json.loads(args.trace.read_text())
    OUT.mkdir(exist_ok=True)
    hero()
    topology()
    frames = []
    for step, op in enumerate(trace["operations"]):
        unique = []
        for sample in op["samples"]:
            if not unique or unique[-1] != sample["peers"]:
                unique.append(sample["peers"])
        states = []
        for state in unique[:-1]:
            states.extend([state] * max(1, 18 // max(1, len(unique) - 1)))
        states.extend([unique[-1]] * 27)
        for tick, state in enumerate(states):
            frames.append(sync_frame(op, state, step, (tick + 1) / len(states), tick))
        if step == 1:
            frames[-1].save(OUT / "three-peers-poster.png", optimize=True)
    save_gif(frames, "three-peers.gif")
    # Keep every recorded byte count and writer state, with slower playback for reading.
    growth = trace["growth"]
    frames = [growth_frame(growth, s, i) for i, s in enumerate(growth["samples"])]
    frames = [frames[0]] * 10 + frames + [frames[-1]] * 30
    middle = next(s for s in growth["samples"] if s["peers"]["writer_open"] and min(s["peers"]["bytes"]) > growth["target_bytes"] // 2)
    growth_frame(growth, middle, 0).save(OUT / "growing-download-poster.png", optimize=True)
    save_gif(frames, "growing-download.gif")


if __name__ == "__main__":
    main()

# /// script
# requires-python = ">=3.11"
# dependencies = ["numpy", "pillow"]
# ///
# white-on-white embossed baroque scrollwork for the page background.
# draws a height map of acanthus scrolls, lights it from the top left and writes webp.
# usage: uv run tools/relief.py site/static/img/relief.webp 2400 1500
#        uv run tools/relief.py site/static/img/relief-tall.webp 1200 2100
import math
import sys

import numpy as np
from PIL import Image

OUT = sys.argv[1] if len(sys.argv) > 1 else "site/static/img/relief.webp"
W = int(sys.argv[2]) if len(sys.argv) > 2 else 2400
H = int(sys.argv[3]) if len(sys.argv) > 3 else 1500
rng = np.random.default_rng(7)
U = min(W, H) / 1500
hmap = np.zeros((H, W), np.float32)


def stamp(cx, cy, r, amp):
    if r < 0.6:
        return
    x0, x1 = max(int(cx - r - 1), 0), min(int(cx + r + 2), W)
    y0, y1 = max(int(cy - r - 1), 0), min(int(cy + r + 2), H)
    if x0 >= x1 or y0 >= y1:
        return
    ys, xs = np.mgrid[y0:y1, x0:x1]
    d2 = ((xs - cx) ** 2 + (ys - cy) ** 2) / (r * r)
    h = amp * np.sqrt(np.clip(1 - d2, 0, 1))
    np.maximum(hmap[y0:y1, x0:x1], h, out=hmap[y0:y1, x0:x1])


def groove(pts, depth, r):
    for x, y, _ in pts:
        x0, x1 = max(int(x - r - 1), 0), min(int(x + r + 2), W)
        y0, y1 = max(int(y - r - 1), 0), min(int(y + r + 2), H)
        if x0 >= x1 or y0 >= y1:
            continue
        ys, xs = np.mgrid[y0:y1, x0:x1]
        d2 = ((xs - x) ** 2 + (ys - y) ** 2) / (r * r)
        g = depth * np.clip(1 - d2, 0, 1)
        sub = hmap[y0:y1, x0:x1]
        sub -= np.minimum(g, sub * 0.5)


def path(x, y, th, length, k0, grow, step=1.2):
    pts, s = [], 0.0
    while s < length:
        t = s / length
        pts.append((x, y, t))
        k = k0 * math.exp(grow * t)
        th += k * step
        x += math.cos(th) * step
        y += math.sin(th) * step
        s += step
    return pts, th


def leaf(x, y, th, length, width, bend, lobes):
    pts, _ = path(x, y, th, length, bend / length, 2.6)
    for px, py, t in pts:
        w = width * math.sin(math.pi * min(1, t ** 0.75)) ** 0.9
        w *= 0.72 + 0.28 * abs(math.cos(lobes * math.pi * t))
        stamp(px, py, w, min(w, 10 * U) * 0.9 + 2 * U)
    groove(pts[: int(len(pts) * 0.8)], 2.2 * U, 1.6 * U)


def scroll(x, y, th, length, width, bend, depth):
    pts, _ = path(x, y, th, length, bend / length, 3.2)
    for px, py, t in pts:
        w = width * (1 - t) ** 0.7 + width * 0.18
        stamp(px, py, w, min(w, 9 * U) + 3 * U)
    groove(pts[: int(len(pts) * 0.7)], 1.4 * U, 1.2 * U)
    sgn = 1 if bend > 0 else -1
    n = 4 + depth
    for i in range(n):
        t = 0.12 + 0.62 * i / n + rng.uniform(-0.03, 0.03)
        px, py, _ = pts[int(t * len(pts))]
        a = math.atan2(pts[int(t * len(pts)) + 1][1] - py, pts[int(t * len(pts)) + 1][0] - px)
        side = -sgn if i % 2 == 0 else sgn
        ll = length * rng.uniform(0.20, 0.30) * (1 - 0.4 * t)
        leaf(px, py, a + side * rng.uniform(0.55, 0.95), ll, width * rng.uniform(2.4, 3.2) * (1 - 0.35 * t),
             -side * rng.uniform(0.45, 0.7), rng.integers(3, 6))
    if depth > 0:
        t = rng.uniform(0.3, 0.45)
        i = int(t * len(pts))
        px, py, _ = pts[i]
        a = math.atan2(pts[i + 1][1] - py, pts[i + 1][0] - px)
        scroll(px, py, a - sgn * 0.7, length * 0.5, width * 0.7, -bend * 1.1, depth - 1)


def frond(x, y, th, size, bend):
    scroll(x, y, th, size, 15 * U, bend, 1)


# left half; mirrored onto the right
S = 1500 * U
frond(-40 * U, H * 0.04, 0.20, 0.95 * S, 0.95)
frond(-30 * U, H * 0.40, -0.10, 0.70 * S, -0.9)
frond(-30 * U, H * 0.66, 0.05, 0.70 * S, 0.85)
frond(-20 * U, H * 0.98, -0.30, 0.95 * S, -0.95)
frond(W * 0.24, -30 * U, 1.30, 0.62 * S, 0.9)
frond(W * 0.26, H + 30 * U, -1.30, 0.62 * S, -0.9)
frond(W * 0.47, -30 * U, 2.0, 0.42 * S, -0.8)
frond(W * 0.46, H + 30 * U, -2.0, 0.42 * S, 0.8)

half = W // 2
hmap[:, W - half:] = np.maximum(hmap[:, W - half:], hmap[:, :half][:, ::-1])


def blur(a, sigma):
    if sigma <= 0:
        return a
    p = int(3 * sigma) + 1
    b = np.pad(a, p, mode="reflect")
    fy = np.fft.fftfreq(b.shape[0])[:, None]
    fx = np.fft.rfftfreq(b.shape[1])[None, :]
    g = np.exp(-2 * (math.pi * sigma) ** 2 * (fx ** 2 + fy ** 2))
    return np.fft.irfft2(np.fft.rfft2(b) * g, s=b.shape)[p:-p, p:-p].astype(np.float32)


ys, xs = np.mgrid[0:H, 0:W].astype(np.float32)
rx, ry = (xs - W / 2) / (W * 0.36), (ys - H / 2) / (H * 0.40)
r = np.sqrt(rx ** 2 + ry ** 2)
fade = np.clip((r - 0.45) / 0.8, 0, 1) ** 1.3
h = blur(hmap, 1.6 * U) * fade

gy, gx = np.gradient(h)
nx, ny, nz = -gx * 0.55, -gy * 0.55, np.ones_like(h)
nl = np.sqrt(nx ** 2 + ny ** 2 + nz ** 2)
L = np.array([-0.55, -0.75, 0.95])
L /= np.linalg.norm(L)
shade = (nx * L[0] + ny * L[1] + nz * L[2]) / nl - L[2]

sh = blur(h, 5 * U)
off = int(7 * U)
cast = np.zeros_like(h)
cast[off:, off:] = sh[:-off, :-off]
shadow = np.clip(cast - h, 0, None) / (12 * U)
ao = np.clip(blur(h, 14 * U) - h, 0, None) / (12 * U)

v = 1 + 0.17 * shade - 0.06 * shadow - 0.03 * ao
v += 0.003 * blur(rng.standard_normal((H, W)).astype(np.float32), 0.7) / 0.4
v += 0.006 * blur(rng.standard_normal((H, W)).astype(np.float32), 60 * U) / 0.0047
v -= 0.035 * np.clip(r - 0.2, 0, None) ** 2

base = np.array([248, 245, 239], np.float32)
rgb = np.clip(base[None, None, :] * v[..., None], 0, 255).astype(np.uint8)
Image.fromarray(rgb).save(OUT, "WEBP", quality=82, method=6)
print(OUT, W, H)

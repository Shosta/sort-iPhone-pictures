#!/usr/bin/env python3
"""Builds docs/assets/photosort-demo.gif and photosort-demo.png from the website screenshots.

Usage (repo root):  python3 -m pip install pillow && python3 docs/assets/build-demo-gif.py
The screenshots use public-domain (CC0) photos; see website/src/assets/screenshots/CREDITS.md.
"""
from pathlib import Path
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
SHOTS = ROOT / "website" / "src" / "assets" / "screenshots"
OUT = Path(__file__).resolve().parent
ORDER = ["hero", "tri", "iphone", "nas", "cartes", "doublons"]
WIDTH, HEIGHT = 800, 506          # 1600x1012 screenshots, halved
HOLD_MS, FADE_MS, FADE_STEPS = 900, 70, 3


def load(name):
    img = Image.open(SHOTS / f"{name}.webp").convert("RGB")
    h = round(img.height * WIDTH / img.width)
    img = img.resize((WIDTH, h), Image.LANCZOS)
    canvas = Image.new("RGB", (WIDTH, HEIGHT), img.getpixel((2, 2)))
    canvas.paste(img, (0, (HEIGHT - h) // 2))
    return canvas


def quantize(img):
    return img.quantize(colors=128, method=Image.MEDIANCUT, dither=Image.Dither.NONE)


shots = [load(n) for n in ORDER]
frames, durations = [], []
for i, shot in enumerate(shots):
    frames.append(quantize(shot))
    durations.append(HOLD_MS)
    nxt = shots[(i + 1) % len(shots)]
    for s in range(1, FADE_STEPS + 1):
        frames.append(quantize(Image.blend(shot, nxt, s / (FADE_STEPS + 1))))
        durations.append(FADE_MS)

frames[0].save(OUT / "photosort-demo.gif", save_all=True, append_images=frames[1:],
               duration=durations, loop=0, optimize=True, disposal=1)
shots[0].save(OUT / "photosort-demo.png", optimize=True)

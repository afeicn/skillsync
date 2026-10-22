#!/usr/bin/env python3
# Generate a 1024px app icon: rounded-rect violet→cyan gradient + white S
from PIL import Image, ImageDraw, ImageFont

S = 1024
img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
px = img.load()
c1, c2 = (124, 58, 237), (34, 211, 238)
for y in range(S):
    for x in range(S):
        t = (x + y) / (2 * S)
        px[x, y] = (
            int(c1[0] + (c2[0] - c1[0]) * t),
            int(c1[1] + (c2[1] - c1[1]) * t),
            int(c1[2] + (c2[2] - c1[2]) * t),
            255,
        )
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=int(S * 0.22), fill=255)
img.putalpha(mask)

d = ImageDraw.Draw(img)
try:
    font = ImageFont.truetype("/System/Library/Fonts/Helvetica.ttc", int(S * 0.52), index=1)
except Exception:
    font = ImageFont.load_default()
txt = "S"
bb = d.textbbox((0, 0), txt, font=font)
w, h = bb[2] - bb[0], bb[3] - bb[1]
d.text(((S - w) / 2 - bb[0], (S - h) / 2 - bb[1]), txt, font=font, fill=(255, 255, 255, 255))

img.save("app/macos/AppIcon.png")
print("wrote app/macos/AppIcon.png")

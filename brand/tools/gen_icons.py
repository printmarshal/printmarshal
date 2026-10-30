#!/usr/bin/env python3
"""Generates the PrintMarshal app icons (D1 Cobalt, D2 Mist, D3 black) and the shield-P glyph.

Usage: python3 brand/tools/gen_icons.py brand/icons
Then render PNGs with brand/tools/export_icons.sh (headless Chrome + sips).

Geometry is the layered-MFP concept chosen on 2026-10-30 (brand/concepts/logo-concepts-round7-*.md):
a translucent second machine behind, a shaded machine in front, a shield at the bottom-left corner
carrying a P. The P is the Barlow Bold glyph, outlined here so no font is needed at render time.
"""
import os, sys


# Barlow Bold "P", 1000 units per em, y up (SIL Open Font License).
P_GLYPH = ("M571 484Q571 421 542.5 373.0Q514 325 462.5 298.5Q411 272 344 272H202Q197 272 197 267V12"
           "Q197 7 193.5 3.5Q190 0 185 0H68Q63 0 59.5 3.5Q56 7 56 12V689Q56 694 59.5 697.5Q63 701 68 701"
           "H350Q416 701 466.0 674.0Q516 647 543.5 597.5Q571 548 571 484ZM431 481Q431 525 403.0 552.0"
           "Q375 579 329 579H202Q197 579 197 574V390Q197 385 202 385H329Q375 385 403.0 411.5Q431 438 431 481Z")
P_BBOX_CX = (56 + 571) / 2  # horizontal centre of the glyph's bounding box

SHIELD = "M12 1.5 L22 5 V13 C22 19.5 17.5 24.5 12 26.5 C6.5 24.5 2 19.5 2 13 V5 Z"  # 24 x 28 box

# The MFP on a 128 grid: feeder, lid, upper body, two drawers, casters, side panel; gaps are cut in the
# background colour so the outline is the icon itself.
BLOCKS = [(30, 7, 34, 11, 3), (20, 20, 88, 10, 2.5), (20, 33, 88, 39, 4), (20, 75, 88, 19, 3),
          (20, 97, 88, 21, 3), (26, 120, 8, 5, 1.5), (94, 120, 8, 5, 1.5)]
PANEL = (97, 35, 24, 12, 2.5)
GAPS = [(30, 43, 50, 8, 1.5), (54, 83, 20, 3.5, 1.5), (54, 106, 20, 3.5, 1.5)]


def mfp(x, y, size, blk, gap, opacity=1.0, extra=""):
    s = size / 128
    r = [f'<g transform="translate({x:.1f},{y:.1f}) scale({s:.4f})" opacity="{opacity}"{extra}>']
    r.append(f'<g fill="{blk}">')
    for bx, by, bw, bh, rx in BLOCKS:
        r.append(f'<rect x="{bx}" y="{by}" width="{bw}" height="{bh}" rx="{rx}"/>')
    r.append('</g>')
    bx, by, bw, bh, rx = PANEL
    r.append(f'<rect x="{bx}" y="{by}" width="{bw}" height="{bh}" rx="{rx}" fill="{blk}" stroke="{gap}" stroke-width="3"/>')
    r.append(f'<g fill="{gap}">')
    for bx, by, bw, bh, rx in GAPS:
        r.append(f'<rect x="{bx}" y="{by}" width="{bw}" height="{bh}" rx="{rx}"/>')
    r.append('</g></g>')
    return "".join(r)


def shield(x, y, w, fill, extra=""):
    s = w / 24
    return f'<path d="{SHIELD}" transform="translate({x:.1f},{y:.1f}) scale({s:.4f})" fill="{fill}"{extra}/>'


def letter_p(cx, baseline, size, fill):
    s = size / 1000
    x = cx - P_BBOX_CX * s
    return f'<path d="{P_GLYPH}" transform="translate({x:.2f},{baseline:.2f}) scale({s:.5f},{-s:.5f})" fill="{fill}"/>'


WAYS = {
    "d1": dict(name="Cobalt", bg=("#4A6FF0", "#1E3AA8"), border=None, gloss=True,
               ghost="#AFC0F5", ghost_op=0.55, gap="#2E54E6", blk=("#FFFFFF", "#DCE3EF"),
               shield=("#2748C9", "#15287A"), shield_gloss=True, p="#FFFFFF",
               shadow=("#081547", 0.40)),
    "d2": dict(name="Mist", bg=("#FFFFFF", "#DDE3EC"), border="#C9D1DE", gloss=False,
               ghost="#8DA4F2", ghost_op=0.50, gap="#EEF1F6", blk=("#3D63F0", "#2142BE"),
               shield=("#FFFFFF", "#E3E8F2"), shield_gloss=False, p="#2E54E6",
               shadow=("#1B2A5E", 0.22)),
    "d3": dict(name="Black", bg=("#1C2029", "#07080B"), border=None, gloss=True,
               ghost="#8DA4F2", ghost_op=0.35, gap="#0E1016", blk=("#FFFFFF", "#DCE3EF"),
               shield=("#3D63F0", "#1E3AA8"), shield_gloss=True, p="#FFFFFF",
               shadow=("#000000", 0.55)),
}


def icon(key, w, radius=230):
    S = 1024
    g = f'''<defs>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="{w['bg'][0]}"/><stop offset="1" stop-color="{w['bg'][1]}"/></linearGradient>
    <linearGradient id="gloss" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#FFFFFF" stop-opacity="0.16"/><stop offset="0.5" stop-color="#FFFFFF" stop-opacity="0"/></linearGradient>
    <linearGradient id="blk" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="{w['blk'][0]}"/><stop offset="1" stop-color="{w['blk'][1]}"/></linearGradient>
    <linearGradient id="shield" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="{w['shield'][0]}"/><stop offset="1" stop-color="{w['shield'][1]}"/></linearGradient>
    <filter id="shadow" x="-20%" y="-20%" width="140%" height="150%"><feDropShadow dx="0" dy="26" stdDeviation="26" flood-color="{w['shadow'][0]}" flood-opacity="{w['shadow'][1]}"/></filter>
    <clipPath id="clip"><rect width="{S}" height="{S}" rx="{radius}"/></clipPath>
  </defs>
  <g clip-path="url(#clip)">
    <rect width="{S}" height="{S}" fill="url(#bg)"/>'''
    if w["gloss"]:
        g += f'\n    <rect width="{S}" height="{S}" fill="url(#gloss)"/>'
    g += "\n    " + mfp(299, 51, 717, w["ghost"], w["gap"], w["ghost_op"])
    g += "\n    " + mfp(175, 154, 717, "url(#blk)", w["gap"], 1.0, ' filter="url(#shadow)"')
    g += "\n    " + shield(60, 529, 358, "url(#shield)", ' filter="url(#shadow)"')
    if w["shield_gloss"]:
        g += "\n    " + shield(60, 529, 358, "url(#gloss)")
    g += "\n    " + letter_p(239, 815, 222, w["p"])
    if w["border"]:
        g += f'\n    <rect x="2" y="2" width="{S-4}" height="{S-4}" rx="228" fill="none" stroke="{w["border"]}" stroke-width="4"/>'
    g += "\n  </g>"
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}" width="{S}" height="{S}" role="img" aria-label="PrintMarshal">\n'
            f'  <title>PrintMarshal app icon, {w["name"]}</title>\n  {g}\n</svg>\n')


def glyph(fill, p, border=None):
    # 64 x 64 box, shield centred: 24x28 scaled to 48x56 at (8,4)
    extra = f' stroke="{border}" stroke-width="1"' if border else ""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="256" height="256" role="img" aria-label="PrintMarshal">\n'
            f'  <title>PrintMarshal shield glyph</title>\n'
            f'  {shield(8, 4, 48, fill, extra)}\n  {letter_p(32, 45, 30, p)}\n</svg>\n')


def write_all(out):
    os.makedirs(out, exist_ok=True)
    for key, w in WAYS.items():
        with open(os.path.join(out, f"app-icon-{key}.svg"), "w") as f:
            f.write(icon(key, w))
    # Full-bleed square for places that round the corners themselves (GitHub org avatar).
    with open(os.path.join(out, "org-avatar.svg"), "w") as f:
        f.write(icon("d1", WAYS["d1"], radius=0))
    with open(os.path.join(out, "glyph-shield-p.svg"), "w") as f:
        f.write(glyph("#1E3AA8", "#FFFFFF"))
    with open(os.path.join(out, "glyph-shield-p-light.svg"), "w") as f:
        f.write(glyph("#FFFFFF", "#2E54E6", "#C9D1DE"))
    with open(os.path.join(out, "glyph-shield-p-mono.svg"), "w") as f:
        f.write(glyph("#14171E", "#FFFFFF"))
    print("wrote", sorted(os.listdir(out)))


if __name__ == "__main__":
    write_all(os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else "brand/icons"))

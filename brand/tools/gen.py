# Generates every file in brand/ from the three mark paths and Barlow Bold.
# Usage: python3 -m venv v && v/bin/pip install fonttools uharfbuzz && BARLOW=/path/Barlow-Bold.ttf v/bin/python brand/tools/gen.py brand

import os, re, subprocess, sys
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
import uharfbuzz as hb

SP = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.abspath(sys.argv[1])
PNG = os.path.join(OUT, "png")
os.makedirs(PNG, exist_ok=True)

# Steel & Signal palette (docs/branding-prompt.md, web/src/styles.css)
GRAPHITE, GUNMETAL, SLATE, MIST = "#14171E", "#2A313D", "#5B6676", "#E8ECF2"
COBALT, DEEP, BRASS, GLOW = "#2E54E6", "#1E3AA8", "#D7A442", "#EFC065"
COBALT_ON_DARK = "#8DA4F2"
WHITE = "#FFFFFF"

SHIELD = "M24 3 42 9v14c0 11-8 18.5-18 22C14 41.5 6 34 6 23V9z"
CHEVRON = "M14 31 24 23 34 31v6l-10-8-10 8z"
STAR = "M24 8.5l1.9 4.1 4.1 1.9-4.1 1.9-1.9 4.1-1.9-4.1-4.1-1.9 4.1-1.9z"

def mark_paths(glint=BRASS, shield=COBALT, chevron=WHITE):
    return (f'<path d="{SHIELD}" fill="{shield}"/>'
            f'<path d="{CHEVRON}" fill="{chevron}"/>'
            f'<path d="{STAR}" fill="{glint}"/>')

def mark_mono(color):
    # one ink: chevron and star are knocked out of the shield
    return f'<path fill-rule="evenodd" d="{SHIELD} {CHEVRON} {STAR}" fill="{color}"/>'

def svg(vb_w, vb_h, body, title):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vb_w:g} {vb_h:g}" '
            f'width="{vb_w:g}" height="{vb_h:g}" role="img" aria-label="{title}">'
            f'<title>{title}</title>{body}</svg>\n')

def write(name, content):
    with open(os.path.join(OUT, name), "w") as f:
        f.write(content)
    print("wrote", name)

# ---- wordmark: "PrintMarshal" in Barlow Bold, letter-spacing -0.015em, kerned by HarfBuzz
# Barlow-Bold.ttf (SIL OFL) from https://github.com/google/fonts/tree/main/ofl/barlow
FONT = os.environ.get("BARLOW", os.path.join(SP, "Barlow-Bold.ttf"))
tt = TTFont(FONT)
upem = tt["head"].unitsPerEm
cap = tt["OS/2"].sCapHeight
glyphset = tt.getGlyphSet()
blob = hb.Blob.from_file_path(FONT); face = hb.Face(blob); font = hb.Font(face)
font.scale = (upem, upem)

def shape(text, size, x0, y_base, color, tracking=-0.015):
    buf = hb.Buffer(); buf.add_str(text); buf.guess_segment_properties()
    hb.shape(font, buf, {"kern": True, "liga": True})
    s = size / upem
    x = x0
    paths = []
    order = tt.getGlyphOrder()
    for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
        gname = order[info.codepoint]
        pen = SVGPathPen(glyphset)
        tp = TransformPen(pen, (s, 0, 0, -s, x + pos.x_offset * s, y_base - pos.y_offset * s))
        glyphset[gname].draw(tp)
        d = pen.getCommands()
        if d:
            paths.append(d)
        x += pos.x_advance * s + tracking * size
    x -= tracking * size  # no tracking after the last glyph
    return f'<path d="{" ".join(paths)}" fill="{color}"/>', x

def wordmark(size, x0, y_base, c1, c2):
    p1, x1 = shape("Print", size, x0, y_base, c1)
    p2, x2 = shape("Marshal", size, x1 - 0.015 * size, y_base, c2)  # keep tracking across the join
    return p1 + p2, x2

def wordmark_svg(c1, c2, title):
    size = 100; capv = cap * size / upem
    pad = 8
    body, xend = wordmark(size, pad, pad + capv, c1, c2)
    # descender room is nil (no descenders in the word); keep a little air below
    return svg(round(xend + pad, 1), round(capv + 2 * pad, 1), body, title)

write("wordmark.svg", wordmark_svg(GRAPHITE, COBALT, "PrintMarshal"))
write("wordmark-on-dark.svg", wordmark_svg(MIST, COBALT_ON_DARK, "PrintMarshal"))
write("wordmark-mono.svg", wordmark_svg(GRAPHITE, GRAPHITE, "PrintMarshal"))
write("wordmark-reversed.svg", wordmark_svg(WHITE, WHITE, "PrintMarshal"))

# ---- mark
write("mark.svg", svg(48, 48, mark_paths(BRASS), "PrintMarshal mark"))
write("mark-on-dark.svg", svg(48, 48, mark_paths(GLOW), "PrintMarshal mark"))
write("mark-mono.svg", svg(48, 48, mark_mono(GRAPHITE), "PrintMarshal mark"))
write("mark-reversed.svg", svg(48, 48, mark_mono(WHITE), "PrintMarshal mark"))

# ---- horizontal lockup: mark 48 high, wordmark cap height centred on the mark's centre
def horizontal(glint, c1, c2, title):
    size = 32; capv = cap * size / upem
    pad = 6; gap = 14
    mark_x = pad
    text_x = mark_x + 48 + gap
    y_base = pad + 24 + capv / 2 - 0.5  # optical centre: shield mass sits slightly high
    body = f'<g transform="translate({mark_x} {pad})">{mark_paths(glint)}</g>'
    wm, xend = wordmark(size, text_x, y_base, c1, c2)
    return svg(round(xend + pad, 1), 48 + 2 * pad, body + wm, title)

write("logo-horizontal.svg", horizontal(BRASS, GRAPHITE, COBALT, "PrintMarshal"))
write("logo-horizontal-on-dark.svg", horizontal(GLOW, MIST, COBALT_ON_DARK, "PrintMarshal"))

def horizontal_mono(color, title):
    size = 32; capv = cap * size / upem; pad = 6; gap = 14
    y_base = pad + 24 + capv / 2 - 0.5
    body = f'<g transform="translate({pad} {pad})">{mark_mono(color)}</g>'
    wm, xend = wordmark(size, pad + 48 + gap, y_base, color, color)
    return svg(round(xend + pad, 1), 48 + 2 * pad, body + wm, title)

write("logo-horizontal-mono.svg", horizontal_mono(GRAPHITE, "PrintMarshal"))
write("logo-horizontal-reversed.svg", horizontal_mono(WHITE, "PrintMarshal"))

# ---- stacked lockup: mark 64 high centred over the wordmark
def stacked(glint, c1, c2, title):
    size = 28; capv = cap * size / upem; pad = 8; gap = 12
    wm, xend = wordmark(size, 0, 0, c1, c2)
    text_w = xend
    W = text_w + 2 * pad
    mark_s = 64 / 48
    mark_x = (W - 64) / 2
    body = f'<g transform="translate({mark_x:.2f} {pad}) scale({mark_s:.4f})">{mark_paths(glint)}</g>'
    y_base = pad + 64 + gap + capv
    wm, _ = wordmark(size, pad, y_base, c1, c2)
    return svg(round(W, 1), round(y_base + pad, 1), body + wm, title)

write("logo-stacked.svg", stacked(BRASS, GRAPHITE, COBALT, "PrintMarshal"))
write("logo-stacked-on-dark.svg", stacked(GLOW, MIST, COBALT_ON_DARK, "PrintMarshal"))

# ---- app icon / org avatar: mark on a graphite tile
def app_icon():
    body = (f'<rect width="512" height="512" rx="112" fill="{GRAPHITE}"/>'
            f'<g transform="translate(80 72) scale(7.3333)">{mark_paths(GLOW)}</g>')
    return svg(512, 512, body, "PrintMarshal")
write("app-icon.svg", app_icon())

def badge():
    # compact "PM" badge for tiles and avatars where the full word cannot fit:
    # the mark itself is the badge; on a cobalt tile for contrast with the app icon
    body = (f'<rect width="512" height="512" rx="112" fill="{COBALT}"/>'
            f'<g transform="translate(80 72) scale(7.3333)">{mark_paths(GLOW, shield=DEEP)}</g>')
    return svg(512, 512, body, "PrintMarshal")
write("app-icon-cobalt.svg", badge())

# ---- PNG export through headless Chrome
CHROME = os.environ.get("CHROME", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
def export(svg_name, png_name, width, height=None, background="transparent"):
    src = os.path.join(OUT, svg_name)
    # read the viewBox to keep the aspect ratio
    head = open(src).read(300)
    vb = head.split('viewBox="')[1].split('"')[0].split()
    vw, vh = float(vb[2]), float(vb[3])
    if height is None:
        height = round(width * vh / vw)
    # Screenshot a copy of the SVG sized to the export, so the page itself is
    # the image and nothing can be captured before it has loaded.
    text = open(src).read()
    text = re.sub(r'width="[^"]*" height="[^"]*"', f'width="{width}" height="{height}"', text, count=1)
    tmp = os.path.join(SP, "render.svg")
    with open(tmp, "w") as f:
        f.write(text)
    out = os.path.join(PNG, png_name)
    subprocess.run([CHROME, "--headless=new", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
                    "--force-device-scale-factor=1", "--default-background-color=00000000",
                    f"--window-size={width},{height}", f"--screenshot={out}",
                    f"--user-data-dir={os.path.join(SP, 'chrome-profile')}", f"file://{tmp}"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=90)
    print("exported", png_name, width, height)

export("app-icon.svg", "org-avatar-1024.png", 1024)
for s in (512, 256, 128, 64, 32, 16):
    export("app-icon.svg", f"app-icon-{s}.png", s)
export("mark.svg", "mark-512.png", 512)
export("mark.svg", "mark-1024.png", 1024)  # transparent avatar: the shield alone
export("mark-on-dark.svg", "mark-on-dark-512.png", 512)
export("mark.svg", "favicon-32.png", 32)
export("logo-horizontal.svg", "logo-horizontal-1600.png", 1600)
export("logo-horizontal-on-dark.svg", "logo-horizontal-on-dark-1600.png", 1600)
export("logo-stacked.svg", "logo-stacked-1200.png", 1200)
export("logo-stacked-on-dark.svg", "logo-stacked-on-dark-1200.png", 1200)
# GitHub social preview (1280x640): graphite ground, horizontal lockup, anchor line
def social():
    size = 32; capv = cap * size / upem
    wm, xend = wordmark(size, 0, 0, MIST, COBALT_ON_DARK)
    lock_w = 48 + 14 + xend
    scale = 2.6
    x0 = (1280 - lock_w * scale) / 2
    y0 = 200
    y_base = 24 + capv / 2 - 0.5
    wm, _ = wordmark(size, 48 + 14, y_base, MIST, COBALT_ON_DARK)
    tag, tag_end = shape("The command-and-security console for your printer fleet.", 36, 0, 0, SLATE, tracking=0)
    tag, _ = shape("The command-and-security console for your printer fleet.", 36, (1280 - tag_end) / 2, 470, "#9AA7B8", tracking=0)
    body = (f'<rect width="1280" height="640" fill="{GRAPHITE}"/>'
            f'<g transform="translate({x0:.1f} {y0}) scale({scale})">{mark_paths(GLOW)}{wm}</g>{tag}')
    return svg(1280, 640, body, "PrintMarshal")
write("social-preview.svg", social())
export("social-preview.svg", "social-preview-1280x640.png", 1280, 640)

# Generates every file in brand/ for the 30 Oct 2026 identity: the layered-MFP
# app icon with the shield-P (from gen_icons.py) and the lowercase wordmark,
# "print" in IBM Plex Mono Medium beside "Marshal" in Barlow SemiBold.
#
# Usage: python3 -m venv v && v/bin/pip install fonttools uharfbuzz
#        FONTS=/dir/with/IBMPlexMono-Medium.ttf+Barlow-SemiBold.ttf v/bin/python brand/tools/gen.py brand
# Fonts: https://github.com/google/fonts (ofl/ibmplexmono, ofl/barlow), SIL OFL.

import os, re, subprocess, sys
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
import uharfbuzz as hb

SP = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, SP)
import gen_icons  # noqa: E402

OUT = os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else "brand")
PNG = os.path.join(OUT, "png")
os.makedirs(PNG, exist_ok=True)
FONTS = os.environ.get("FONTS", SP)

GRAPHITE, GUNMETAL, SLATE, MIST = "#14171E", "#2A313D", "#5B6676", "#E8ECF2"
COBALT, DEEP, TINT, WHITE = "#2E54E6", "#1E3AA8", "#8DA4F2", "#FFFFFF"


class Face:
    def __init__(self, path):
        self.tt = TTFont(path)
        self.upem = self.tt["head"].unitsPerEm
        self.cap = self.tt["OS/2"].sCapHeight
        self.xh = self.tt["OS/2"].sxHeight
        self.glyphs = self.tt.getGlyphSet()
        self.order = self.tt.getGlyphOrder()
        blob = hb.Blob.from_file_path(path)
        self.font = hb.Font(hb.Face(blob))
        self.font.scale = (self.upem, self.upem)

    def shape(self, text, size, x0, y_base, color, tracking=0.0):
        buf = hb.Buffer(); buf.add_str(text); buf.guess_segment_properties()
        hb.shape(self.font, buf, {"kern": True, "liga": True})
        s = size / self.upem
        x = x0
        paths = []
        for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
            pen = SVGPathPen(self.glyphs)
            tp = TransformPen(pen, (s, 0, 0, -s, x + pos.x_offset * s, y_base - pos.y_offset * s))
            self.glyphs[self.order[info.codepoint]].draw(tp)
            if pen.getCommands():
                paths.append(pen.getCommands())
            x += pos.x_advance * s + tracking * size
        x -= tracking * size
        return f'<path d="{" ".join(paths)}" fill="{color}"/>', x


MONO = Face(os.path.join(FONTS, "IBMPlexMono-Medium.ttf"))
BARLOW = Face(os.path.join(FONTS, "Barlow-SemiBold.ttf"))


def svg(w, h, body, title):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w:g} {h:g}" width="{w:g}" height="{h:g}" '
            f'role="img" aria-label="{title}"><title>{title}</title>{body}</svg>\n')


def write(name, content):
    with open(os.path.join(OUT, name), "w") as f:
        f.write(content)
    print("wrote", name)


# ---- wordmark: "print" (mono, the surface ink) + "Marshal" (Barlow, Cobalt), one baseline
def wordmark(size, x0, y_base, c_print, c_marshal):
    p1, x1 = MONO.shape("print", size, x0, y_base, c_print)
    p2, x2 = BARLOW.shape("Marshal", size, x1 + 0.10 * size, y_base, c_marshal, tracking=-0.01)
    return p1 + p2, x2


def wordmark_svg(c_print, c_marshal, title="PrintMarshal"):
    size = 100
    capv = BARLOW.cap * size / BARLOW.upem
    desc = 0.22 * size  # "p" descender
    pad = 8
    body, xend = wordmark(size, pad, pad + capv, c_print, c_marshal)
    return svg(round(xend + pad, 1), round(capv + desc + 2 * pad, 1), body, title)


write("wordmark.svg", wordmark_svg(GRAPHITE, COBALT))
write("wordmark-on-dark.svg", wordmark_svg(MIST, TINT))
write("wordmark-mono.svg", wordmark_svg(GRAPHITE, GRAPHITE))
write("wordmark-reversed.svg", wordmark_svg(WHITE, WHITE))


# ---- the mark: the app icon (rounded) and the shield-P glyph for one-ink uses
def embed(svg_text, x, y, w, h):
    inner = re.sub(r"^<svg[^>]*>", "", svg_text.strip())
    inner = re.sub(r"</svg>\s*$", "", inner)
    inner = re.sub(r"<title>.*?</title>", "", inner, count=1, flags=re.S)
    vb = re.search(r'viewBox="([^"]+)"', svg_text).group(1)
    return f'<svg x="{x:g}" y="{y:g}" width="{w:g}" height="{h:g}" viewBox="{vb}">{inner}</svg>'


ICON = {k: gen_icons.icon(k, w) for k, w in gen_icons.WAYS.items()}
GLYPH_MONO = gen_icons.glyph(GRAPHITE, WHITE)
GLYPH_REV = gen_icons.glyph(WHITE, GRAPHITE)

write("mark.svg", ICON["d1"])
write("mark-on-dark.svg", ICON["d3"])
write("mark-mono.svg", GLYPH_MONO)
write("mark-reversed.svg", GLYPH_REV)
write("app-icon.svg", ICON["d1"])
write("app-icon-mist.svg", ICON["d2"])
write("app-icon-black.svg", ICON["d3"])


# ---- horizontal lockup: icon 48 high, wordmark beside it
def horizontal(mark_svg, c_print, c_marshal, title="PrintMarshal"):
    size = 30
    capv = BARLOW.cap * size / BARLOW.upem
    pad, gap = 6, 14
    y_base = pad + 24 + capv / 2
    body = embed(mark_svg, pad, pad, 48, 48)
    wm, xend = wordmark(size, pad + 48 + gap, y_base, c_print, c_marshal)
    return svg(round(xend + pad, 1), 48 + 2 * pad, body + wm, title)


write("logo-horizontal.svg", horizontal(ICON["d1"], GRAPHITE, COBALT))
write("logo-horizontal-on-dark.svg", horizontal(ICON["d1"], MIST, TINT))
write("logo-horizontal-mono.svg", horizontal(GLYPH_MONO, GRAPHITE, GRAPHITE))
write("logo-horizontal-reversed.svg", horizontal(GLYPH_REV, WHITE, WHITE))


# ---- stacked lockup: icon 72 centred over the wordmark
def stacked(mark_svg, c_print, c_marshal, title="PrintMarshal"):
    size = 28
    capv = BARLOW.cap * size / BARLOW.upem
    pad, gap = 8, 12
    _, xend = wordmark(size, 0, 0, c_print, c_marshal)
    W = xend + 2 * pad
    body = embed(mark_svg, (W - 72) / 2, pad, 72, 72)
    y_base = pad + 72 + gap + capv
    wm, _ = wordmark(size, pad, y_base, c_print, c_marshal)
    return svg(round(W, 1), round(y_base + 0.22 * size + pad, 1), body + wm, title)


write("logo-stacked.svg", stacked(ICON["d1"], GRAPHITE, COBALT))
write("logo-stacked-on-dark.svg", stacked(ICON["d1"], MIST, TINT))


# ---- GitHub social preview, 1280 x 640
def social():
    size = 96
    capv = BARLOW.cap * size / BARLOW.upem
    _, xend = wordmark(size, 0, 0, MIST, TINT)
    icon_s, gap = 176, 40
    total = icon_s + gap + xend
    x0 = (1280 - total) / 2
    y0 = 180
    body = f'<rect width="1280" height="640" fill="{GRAPHITE}"/>'
    body += embed(ICON["d1"], x0, y0, icon_s, icon_s)
    wm, _ = wordmark(size, x0 + icon_s + gap, y0 + icon_s / 2 + capv / 2, MIST, TINT)
    tag = "Printer fleet security and configuration, self-hosted and open source."
    _, tend = BARLOW.shape(tag, 34, 0, 0, SLATE)
    line, _ = BARLOW.shape(tag, 34, (1280 - tend) / 2, 470, "#9AA7B8")
    return svg(1280, 640, body + wm + line, "PrintMarshal")


write("social-preview.svg", social())


# ---- PNG export through headless Chrome
CHROME = os.environ.get("CHROME", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")


def export(svg_name, png_name, width, height=None):
    src = os.path.join(OUT, svg_name)
    text = open(src).read()
    vb = re.search(r'viewBox="([^"]+)"', text).group(1).split()
    vw, vh = float(vb[2]), float(vb[3])
    if height is None:
        height = round(width * vh / vw)
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


write("org-avatar.svg", gen_icons.icon("d1", gen_icons.WAYS["d1"], radius=0))
export("org-avatar.svg", "org-avatar-1024.png", 1024)
for s in (512, 256, 128, 64, 32, 16):
    export("app-icon.svg", f"app-icon-{s}.png", s)
export("mark.svg", "mark-512.png", 512)
export("mark.svg", "mark-1024.png", 1024)
export("mark-on-dark.svg", "mark-on-dark-512.png", 512)
export("app-icon.svg", "favicon-32.png", 32)
export("logo-horizontal.svg", "logo-horizontal-1600.png", 1600)
export("logo-horizontal-on-dark.svg", "logo-horizontal-on-dark-1600.png", 1600)
export("logo-stacked.svg", "logo-stacked-1200.png", 1200)
export("logo-stacked-on-dark.svg", "logo-stacked-on-dark-1200.png", 1200)
export("social-preview.svg", "social-preview-1280x640.png", 1280, 640)
os.remove(os.path.join(SP, "render.svg"))

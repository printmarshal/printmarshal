# PrintMarshal brand

The visual identity for PrintMarshal: the **Guard Chevron** mark, the wordmark,
the lockups, the app icon and the "Steel & Signal" colour system. Everything in
this folder is original work for this project; it references no printer vendor
or other company. The same mark and tokens drive the product UI
(`web/src/components/icons.tsx`, `web/src/styles.css`), so what you see in the
app is what you see here.

## The mark

A shield (guard, security) carrying a rank chevron (command, the marshal) with
a brass star at its crown (the marshal's badge). It is a flat silhouette that
reads at 16 px and works as a single ink.

| File | Use |
| --- | --- |
| `mark.svg` | On light backgrounds. Cobalt shield, white chevron, brass star. |
| `mark-on-dark.svg` | On dark backgrounds. Same, with the brighter Brass Glow star. |
| `mark-mono.svg` | One colour (Graphite). The chevron and star are knocked out, so it works on any light surface. |
| `mark-reversed.svg` | One colour (white) for dark surfaces, print reversals, embroidery. |

## Wordmark and lockups

"PrintMarshal" is set in **Barlow Bold** (SIL Open Font License) with
−1.5 % tracking, "Print" in Graphite and "Marshal" in Cobalt Steel. The text is
converted to outlines in every SVG, so no font needs to be installed.

| File | Use |
| --- | --- |
| `logo-horizontal.svg` / `-on-dark.svg` | Primary lockup: README headers, web pages, slides, banners. |
| `logo-horizontal-mono.svg` / `-reversed.svg` | One-colour versions of the primary lockup. |
| `logo-stacked.svg` / `-on-dark.svg` | Square-ish spaces: cards, splash screens, posters. |
| `wordmark.svg` / `-on-dark.svg` / `-mono.svg` / `-reversed.svg` | Text only, where the mark already appears nearby. |
| `app-icon.svg` | App icon, favicon source, **GitHub organisation avatar**: the mark on a Graphite tile. |
| `app-icon-cobalt.svg` | Alternate tile in Cobalt Steel for places where the graphite tile disappears. |
| `social-preview.svg` | GitHub social preview (1280 × 640): lockup and anchor line on Graphite. |

Rendered PNGs are in `png/`: the organisation avatar at 1024 px, the app icon
from 512 px down to 16 px, the mark at 512 px, both lockups at 1600 / 1200 px
and the social preview. `favicon-32.png` is the mark alone for tab strips.

## Colour: Steel & Signal

| Role | Name | Hex |
| --- | --- | --- |
| Ink, darkest | Graphite | `#14171E` |
| Steel surface | Gunmetal | `#2A313D` |
| Mid neutral | Slate | `#5B6676` |
| Light surface | Mist | `#E8ECF2` |
| **Primary brand** | **Cobalt Steel** | `#2E54E6` |
| Primary depth | Deep Cobalt | `#1E3AA8` |
| Primary on dark | Cobalt Tint | `#8DA4F2` |
| **Badge accent** | **Brass** | `#D7A442` |
| Badge highlight | Brass Glow | `#EFC065` |

Cobalt Steel is the brand colour. Brass belongs to the badge: use it for the
star and nothing else, so it is never confused with the product's amber
*warning* state. The operational status colours (good, warning, critical,
info, offline) live in the UI tokens and are not identity colours.

## Typography

- Display and wordmark: **Barlow** 600–700.
- UI and body: **IBM Plex Sans**; code and identifiers: **IBM Plex Mono**.

## Rules

- **Clear space:** keep a margin of at least the star's height (one quarter
  of the mark's height) free around every lockup.
- **Minimum size:** mark 16 px; horizontal lockup 120 px wide; stacked lockup
  96 px wide. Below that, use the mark alone.
- Do not recolour the shield, rotate the mark, add effects, outline the
  wordmark or set the name in another typeface. Do not place the full-colour
  mark on Cobalt; use the reversed mark there.
- The name is written **PrintMarshal**, one word, capital P and M; "PM" is
  acceptable in running text after the first mention.

## Regenerating

The SVGs and PNGs are produced by a small Python script that shapes the
wordmark with HarfBuzz, traces the glyphs with fontTools and renders the PNGs
with headless Chrome. The mark's three paths are the source of truth and are
identical to the ones in `web/src/components/icons.tsx`; change them in both
places together.

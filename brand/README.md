# PrintMarshal brand

The visual identity for PrintMarshal: the **app icon** (a layered multifunction
printer carrying a shield with a P), the lowercase **wordmark**, the lockups
and the "Steel & Signal" colour system. Everything here is original work for
this project; it references no printer vendor or other company. The same icon
and tokens drive the product UI (`web/src/components/icons.tsx`,
`web/src/styles.css`) and the website (`site/`), so what you see in the app is
what you see here.

## The icon

A multifunction printer drawn as white blocks, with the icon background showing
through the gaps as its outline: document feeder, scanner lid, side control
panel, output slot and two paper drawers. A second, translucent machine sits
behind it (a fleet, not one device). A shield at the bottom-left corner carries
the letter **P**. Three colourways share one drawing:

| File | Use |
| --- | --- |
| `icons/app-icon-d1.svg` (= `app-icon.svg`, `mark.svg`) | **Cobalt.** The default: app icon, favicon source, lockups, organisation avatar. |
| `icons/app-icon-d2.svg` (= `app-icon-mist.svg`) | **Mist.** Light docks and light pages where the Cobalt tile would shout. |
| `icons/app-icon-d3.svg` (= `app-icon-black.svg`, `mark-on-dark.svg`) | **Black.** Dark docks and the console's own dark surface. |
| `icons/glyph-shield-p*.svg` (= `mark-mono.svg`, `mark-reversed.svg`) | The shield-P alone, one ink, for favicons, tab bars, the sidebar and print. |
| `icons/org-avatar.svg` | The Cobalt icon without rounded corners, for places that round the corners themselves (GitHub). |

The letter is outlined from Barlow Bold, so no font is needed to render the
icon. Rendered PNGs are in `icons/png/` (1024 px down to 16 px) and `png/`.
From 32 px down the full icon is still legible; at 16 px use the glyph.

## Wordmark and lockups

The wordmark is two words on one baseline with no camel-case bump:
**print** in IBM Plex Mono Medium, set in the surface ink (Graphite on light,
Mist on dark), and **Marshal** in Barlow SemiBold, set in Cobalt (Cobalt Tint
on dark). Both are SIL Open Font License faces; the SVGs carry outlines.

| File | Use |
| --- | --- |
| `logo-horizontal.svg` / `-on-dark.svg` | Primary lockup: README headers, web pages, slides, banners. |
| `logo-horizontal-mono.svg` / `-reversed.svg` | One-colour versions, with the shield-P glyph in place of the icon. |
| `logo-stacked.svg` / `-on-dark.svg` | Square-ish spaces: cards, splash screens, posters. |
| `wordmark.svg` / `-on-dark.svg` / `-mono.svg` / `-reversed.svg` | Text only, where the icon already appears nearby. |
| `social-preview.svg` | GitHub social preview (1280 × 640): lockup and anchor line on Graphite. |

In HTML the wordmark is live text (`.wm-print` / `.wm-marshal` in the app and
the site) so it follows the surface colour and the user's font rendering.

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
| Status accent | Brass | `#D7A442` |

Cobalt Steel is the brand colour and the only colour in the identity besides
the neutrals. Brass is not an identity colour any more: it stays in the UI
tokens for the product's amber *warning* state and must not appear on the icon
or the wordmark.

## Typography

- Wordmark: **IBM Plex Mono** 500 for "print", **Barlow** 600 for "Marshal".
- Display: **Barlow** 600–700. UI and body: **IBM Plex Sans**; code and
  identifiers: **IBM Plex Mono**.

## Rules

- **Clear space:** keep a margin of at least a quarter of the icon's height
  free around every lockup.
- **Minimum size:** icon 24 px (glyph below that); horizontal lockup 140 px
  wide; stacked lockup 96 px wide.
- Do not recolour the icon, rotate it, add effects beyond its own shading,
  outline the wordmark or set the name in another typeface. Do not place the
  Cobalt icon on Cobalt; use the Mist or Black colourway there.
- The name is written **PrintMarshal**, one word, when it is typed; the
  wordmark alone shows it as `print` + `Marshal`.

## Regenerating

```sh
python3 -m venv v && v/bin/pip install fonttools uharfbuzz
python3 brand/tools/gen_icons.py brand/icons && sh brand/tools/export_icons.sh brand/icons
FONTS=/dir/with/IBMPlexMono-Medium.ttf+Barlow-SemiBold.ttf v/bin/python brand/tools/gen.py brand
```

`gen_icons.py` draws the icons (the P is already outlined in the script);
`export_icons.sh` renders their PNGs with headless Chrome and `sips`; `gen.py`
sets the wordmark from the two font files and composes the lockups and the
social preview. `concepts/` keeps the sketch rounds that led here.

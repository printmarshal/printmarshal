---
name: steel-and-signal-brand
description: Apply the PrintMarshal brand. Use when editing the website in site/, the SPA's visual layer in web/src, anything in brand/, the README header, or when producing icons, lockups, screenshots or marketing copy.
---

# Steel & Signal brand

Read `brand/README.md` first; it is the source of truth. The rules that matter most:

- **Palette.** Graphite `#14171E`, Gunmetal `#2A313D`, Slate `#5B6676`, Mist `#E8ECF2`, Cobalt
  Steel `#2E54E6`, Deep Cobalt `#1E3AA8`, Cobalt Tint `#8DA4F2` (accent on dark). Brass is a
  product *warning* state, never an identity colour.
- **Icon.** The layered multifunction printer with the shield-P (`brand/icons/app-icon-d1.svg`
  Cobalt, `-d2` Mist, `-d3` Black); the shield-P glyph alone below 24 px. Never the retired
  Guard Chevron shield.
- **Wordmark.** Lowercase `print` in IBM Plex Mono Medium in the surface ink, beside `Marshal`
  in Barlow SemiBold in Cobalt (Tint on dark), one baseline, no camel-case bump. In HTML it is
  live text: `.wm-print` / `.wm-marshal`.
- **Surfaces.** Light sections on Mist/white with Graphite ink, dark sections on Graphite with
  Mist ink, alternating; Cobalt is the action colour on both.
- **Type.** Barlow 600–700 display, IBM Plex Sans body, IBM Plex Mono only for code, setting
  keys, commands and the wordmark.
- **Copy.** Sentence case, active voice, end-user perspective; a CTA says what happens. No
  all-caps tracked eyebrows, no middle-dot meta strings, no arrows appended to buttons.
- **Generated files.** Regenerate with `brand/tools/gen_icons.py`, `export_icons.sh` and
  `gen.py`; never hand-edit the PNGs.
- **Nothing sensitive.** Concept notes describe the marks; naming, roadmap and company asides
  stay out of the repo.

# Design: the public status board

The design pass from `plan.md` step 1. The page's one job: someone standing up from their desk,
or glancing at a screen in the print room, learns in a second whether the printer they want is
ready, has paper and has toner. The reader is not an administrator and has no account; the page
must read at arm's length on a phone and at three metres on a wall.

## Tokens
Colour (the console's variables; the page sets the light theme and ignores the viewer's dark
preference, because wall screens and phones in bright rooms need the Mist surface)
- Page: white `#FFFFFF`; the header strip Mist `#E8ECF2`; ink Graphite `var(--text)`;
  secondary `var(--text-2)`.
- The one accent: Cobalt `var(--accent)` for the up edge, the wordmark and the paging dot.
- States, from the console: `var(--ok)`, `var(--warn)`, `var(--crit)`, `var(--off)` with their
  `-bg` pairs for pills. Never brass as identity; `--glint` is not used here.
- Toner: the supply colours `var(--sup-c)`, `var(--sup-m)`, `var(--sup-y)`, `var(--sup-k)`;
  other colorants fall back to Slate.

Type (two families, each with one job)
- Barlow 600 for printer labels and the page title. Scale: label 22 px desktop, 20 px phone,
  36 px wall; title 26 px desktop, 32 px wall. Letter-spacing −0.01em.
- IBM Plex Sans for everything else: 15 px / 1.5 body on desktop, 14 px phone, 20 px wall;
  pills 13 px (17 px wall) weight 500. No mono anywhere except the wordmark.
- No capitals for labels; sentence case throughout; no eyebrow labels above rows.

Layout
- A single column, 880 px wide at most, left aligned, 24 px side gutters (16 px on phones).
- Header strip: the shield-P glyph at 28 px, the board title, and on the right "Updated 12 s
  ago" in `--text-2`. The strip is the only Mist surface on the page.
- Each printer is a row, not a card: no border, no shadow, no radius. Its structure is a 6 px
  vertical edge on the left, 100% of the row's height, Cobalt when the printer is up, `--crit`
  when down, `--off` when up/down is not shown. The edge is the status; nothing else repeats it.
- Row content: label on the first line (with the optional state word after it in `--text-2`:
  "sleeping", "printing"), location on the second line in `--text-2`, then one line of signals.
- Signals, left to right, only the enabled ones: paper as one pill per input tray ("Tray 1 ok",
  "Tray 2 low", "Tray 3 empty" in the matching tone); toner as the memorable element, a small
  stacked gauge per supply: four 8 px bars for C, M, Y, K in their own colours with the fill
  length as the level and the percentage after the last bar when known, a hatched grey bar
  when unknown; alerts as pills in severity tone with the category word ("paper jam",
  "cover open", "toner low", "needs service", "not reachable", "needs attention").
- Rows are separated by 20 px of white; a hairline is not needed because the edges and the
  spacing carry the rhythm.

```
┌──────────────────────────────────────────────────────────────────────┐
│ ▣ Printer status                                   Updated 12 s ago  │  Mist strip
└──────────────────────────────────────────────────────────────────────┘
▌ Library ground floor, colour  sleeping
▌ Building B, floor 2
▌ Tray 1 ok   Tray 2 low      ▆▆▆▆▆▆▆▆ C  ▆▆▆▆ M  ▆▆▆▆▆▆ Y  ▆▆ K 12%     toner low

▌ Student print room, mono
▌ Building A, ground floor
▌ Tray 1 empty                  ▆▆▆▆▆▆▆▆▆▆ K 88%                        out of paper
  (edge in --crit when down; the row's text stays Graphite, nothing greys out)
```

Phone (under 600 px): the same rows; the signal line wraps, toner gauges go to a second line
with the bars 6 px tall; the header's "Updated" moves under the title.

Wall mode (`?wall=1`): a grid of tiles, two columns from 1100 px and three from 1600 px; each
tile is the same row scaled by the wall type sizes with a 10 px edge, 28 px padding; twelve per
page, paging every 10 seconds with a row of small dots under the grid, the current one Cobalt;
no other motion. Type at 36 px labels reads at three metres on a 1080p screen.

## Motion
One moment only: when a fetch changes a printer's up/down or a tray or alert tone, that row's
edge or pill crossfades over 300 ms. No entrance animation, no hover effects (nothing on the page
is clickable except nothing; there are no links). `prefers-reduced-motion` disables the
crossfade and the page switch in wall mode becomes a cut.

## Copy
- Title default: "Printer status" (the administrator can change it).
- Freshness: "Updated 12 s ago", "Updated 3 min ago", counted from the data's own time (the newest
  poll among the listed printers), not from the page's last fetch; when stale: a Mist band under the header,
  "Last update 25 min ago. This may be out of date."
- Up/down words after the label are not shown; the edge says it. A down printer gets one pill
  in `--crit`: "not reachable", so the state is readable without colour.
- Empty: "No printers are listed yet." Disabled: "This board is not available." Error after three
  failed fetches: "Status unavailable. Trying again." Each with the shield-P glyph above it,
  centred, nothing else on the page.
- Footer: the wordmark at 13 px in `--text-3`, right aligned, the only place the product name
  appears.

## Accessibility
- Every colour has a word: tones carry their text ("low", "empty", "not reachable"); toner bars
  carry the percentage or "unknown" as visually hidden text when the number is not shown.
- Contrast: Graphite on white and Mist passes AAA; pill text on `-bg` pairs passes AA as in the
  console.
- The page announces freshness changes with `aria-live="polite"` on the "Updated" text only.
- No focusable elements, so no focus order to manage; the document still has a `<main>` and an
  `<h1>`.

## Review against the generic defaults
- Cards with identical shadows and radius: rejected; rows with a status edge instead.
- Hairline broadsheet: rejected; spacing carries structure.
- All-caps labels, eyebrows, middle dots: none; locations are written as sentences.
- A big number with a small label as the hero: rejected; the hero is the first printer row,
  which is the content.
- The memorable element is the toner gauge in the real supply colours; everything else is quiet.

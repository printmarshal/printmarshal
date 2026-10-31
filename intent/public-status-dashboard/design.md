# Design: the public status board (version 2)

## What changed and why
Version 1 was drawn against a made-up fleet of two trays and one alert. On a real fleet (six HP
multifunction printers, five trays, four toners and two alerts each) every printer became two or
three lines of look-alike pills, the toner gauges shrank to bars with letters, the alerts repeated
what the trays had just said, and the up/down edge disappeared in the noise. The page read as an
admin list. The reader has one question, "can I print there now?", and nothing on the page
answered it.

Version 2 answers it with one dominant element per printer: a solid state block in the state's
colour carrying the state word, set larger than the printer's own name. Paper is summarised to a
sentence, toner to one four-bar element with one number, and alerts appear only when they add
something the state word and the sentence do not already say. The up/down edge is gone; up/down is
folded into the state.

## The reader
Someone standing up from a desk, or glancing at the print-room screen, decides whether to walk to
a printer. They know which printer is near them; they do not know its state. So the state is the
biggest thing on the page and the name is second. No account, no links; the page must read at
arm's length on a phone and at three metres on a wall.

## The state
One word per printer, derived from the signals the administrator has switched on, in this fixed
precedence (first match wins):

| State word | Tone | When |
| --- | --- | --- |
| Not reachable | `--off` | `online` is false |
| No status | `--off` | `unconfirmed`: the up/down switch is off and the printer did not answer its last poll |
| Needs service | `--crit` when critical, `--warn` otherwise | an alert in category maintenance |
| Paper jam | `--crit` | an alert in category jam |
| Cover open | `--crit` | an alert in category door |
| Out of paper | `--crit` | trays are listed, none is ok and at least one is empty; or no trays listed and a critical paper alert |
| Toner empty | `--crit` | a supply with status empty; or no supplies listed and a critical toner alert |
| Low paper | `--warn` | a tray low or empty while another is ok; or no trays listed and a paper alert |
| Toner low | `--warn` | a supply with status low; or no supplies listed and a toner alert |
| Needs attention | `--warn` | an alert in category other |
| Ready | `--ok` | anything else when at least one signal is on |
| No status | `--off` | no signal is on (label only) |

"Not reachable" is grey, not red: the monitor cannot see the printer, which is different from the
printer saying it cannot print. Paper and toner alerts never add anything when the trays or supplies
are listed, so they are dropped there; maintenance, jam, door and other alerts that did not become
the state word are listed after the paper and toner detail in their own words.

## Tokens
Colour (the console's light variables; the page forces the light theme because wall screens and
phones in bright rooms need the Mist surface)
- Page: Mist `#E8ECF2`. Printer rows: white `#FFFFFF`. Ink: Graphite `var(--text)`, secondary
  `var(--text-2)`.
- State blocks: solid `var(--ok)` `#13703A`, `var(--warn)` `#94540A`, `var(--crit)` `#B3261E`,
  `var(--off)` `#555E6B`, white word on each (all four pass AA for normal text, AAA for the state
  word's size). The pale `-bg` pairs are not used; there are no pills on the page.
- The one accent: Cobalt `var(--accent)` for the wordmark, the shield-P and the paging dot. It
  never means a state.
- Toner: the supply colours `var(--sup-c)`, `var(--sup-m)`, `var(--sup-y)`, `var(--sup-k)`;
  the track is `var(--track)`; another colorant falls back to Slate.
- Stale band: `var(--warn-bg)` with `var(--warn)` text.

Type (two families, one job each)
- Barlow 700 for the state word: 26 px desktop, 20 px phone, 40 px wall. Barlow 600 for the
  printer name: 22 / 19 / 32 px, and for the page title: 22 / 20 / 28 px. Letter-spacing −0.01em.
- IBM Plex Sans for everything else: 15 px desktop, 14 px phone, 20 px wall; the toner number
  weight 500, tabular. No mono except the wordmark.
- Sentence case throughout; no eyebrows, no capitals, no middle dots, no arrows.

## Layout
Rows on desktop, left aligned, one column 880 px wide at most, 24 px gutters (16 px on phones).
Each printer is a white block of two parts: the state block on the left, 184 px wide, the full
height of the row, with the word in white; then the body: the name, the optional location and
state verb ("sleeping"), and one detail line. The blocks line up into a column of lamps that reads
down the page as colour before any word is read.

```
▣ Printer status                                            Updated 12 s ago

┌──────────────┬──────────────────────────────────────────────────────────┐
│              │ Library ground floor                                     │
│ Needs        │ Building B, floor 2                                      │
│ service      │ Tray 1 and Tray 4 empty    ▌▌▌▌ 57%                      │
└──────────────┴──────────────────────────────────────────────────────────┘
┌──────────────┬──────────────────────────────────────────────────────────┐
│              │ Reception                                                │
│ Low paper    │ Tray 1 empty               ▌▌▌▌ 30%                      │
└──────────────┴──────────────────────────────────────────────────────────┘
┌──────────────┬──────────────────────────────────────────────────────────┐
│ Ready        │ Workshop                                                 │
│              │ Paper ok                   ▌▌▌▌ 50%                      │
└──────────────┴──────────────────────────────────────────────────────────┘
                                                                printMarshal
```

The detail line, left to right, only the enabled signals, separated by space and not by marks:
- Paper, as a sentence: "Paper ok"; "Tray 1 empty"; "Tray 1 and Tray 3 empty"; "Tray 2 low, Tray 3
  empty"; "Tray 1, Tray 3 empty, +1 more" when more than two trays need naming; "Paper unknown"
  when no tray reports.
- Toner, as one element: four vertical bars of equal size (10 by 30 px on desktop, 8 by 24 px on
  phones) in the real supply colours, filled to the level from the bottom (never less than 2 px), and one number, the lowest known percentage. A supply that reports
  no level is a hatched bar and the number is left out when none is known; an empty supply is
  named instead of numbered ("Magenta empty"). Visually hidden text carries every level for
  screen readers.
- Alerts that are neither the state word nor paper nor toner, in words: "cover open", "paper jam",
  "needs service", "needs attention".

Phone (under 640 px): the state block becomes a band across the top of the white block, the word
at 20 px on the left; the body sits under it; the detail line wraps and the toner bars go to their
own line. The header's "Updated" moves under the title. Nothing overflows at 360 px.

Wall mode (`?wall=1`): tiles in a grid, two columns from 1100 px and three from 1600 px; each tile
is the phone arrangement at wall sizes: a 40 px state word in a band across the top, the name at
32 px, the detail line at 20 px with 16 px wide, 40 px tall bars. Twelve per page, paging every 10 seconds
with small dots under the grid, the current one Cobalt. A 1080p wall at three metres reads the
state words from across a room.

## Motion
One moment only: when a fetch changes a state, the block's colour crossfades over 300 ms and the
toner bars ease to their new level. In wall mode the page change is a 300 ms fade. No entrance
animation and no hover effects: nothing on the page is clickable. `prefers-reduced-motion` turns
the crossfade off and the page change into a cut.

## Copy
- Title default: "Printer status" (the administrator can change it).
- Freshness: "Updated 12 s ago", "Updated 3 min ago", counted from the data's own time, in a
  polite live region. When stale: a band under the header, "Last update 25 min ago. This may be
  out of date."
- Empty: "No printers are listed yet." Disabled: "This board is not available." Error after three
  failed fetches: "Status unavailable. Trying again." Each with the shield-P glyph above it,
  centred, nothing else on the page.
- Footer: the wordmark at 13 px, right aligned, the only place the product name appears.

## Accessibility
- Every colour has a word: the state block carries its word; toner bars carry their levels as
  hidden text; the stale band is a sentence.
- When the up/down switch is on, each printer also says "Online" or "Offline" beside its name, in
  the state verb's style. When the switch is off, the card says neither.
- Contrast: white on each state tone passes AA for normal text and AAA at the state word's size;
  Graphite on white and Mist passes AAA.
- No focusable elements; the document has a `<main>`, an `<h1>` for the title and an `<h2>` per
  printer, with the state word read before the name.

## Review against the generic defaults
- The status-page default (a list with a small coloured dot and a word on the right): rejected.
  The state is the largest type on the page and sits before the name, because the reader's
  question is the state, not the name.
- Cards with identical shadows and one radius on everything: rejected; white blocks on Mist with
  no shadow, whose only structure is the state block.
- Hairline broadsheet: rejected; the Mist gap carries the rhythm.
- Pills for every value: rejected; prose for paper, one element for toner, words for alerts.
- Big number with a small label as a hero: rejected; the first row is the hero.
- The memorable element is the column of state blocks. The toner bars, the type and the surfaces
  stay quiet so that it is the only loud thing.

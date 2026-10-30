# Website design audit — 30 October 2026

Audit of `site/` (printmarshal.app, main at 43a21d9) against Anthropic's `frontend-design` skill,
used as the rubric. Reviewer: Claude (Fable 5.1); Agnes's run on Opus 5.5 was blocked by her
subscription quota and only her screenshot capture survived, so the evidence set is her twelve
captures in `audit-shots/` (every page at 1440 and 1024) plus measurements taken in a browser at
1440 and 375 px against a local copy. Nothing in `site/` was changed.

## Verdict

The site is coherent, legible and credible for its audience, and it spends its one bold move in
the right place: the product itself, shown as a full console capture in the hero. The dual-tone
rhythm works and the new wordmark sits well. What holds it back is template chrome rather than
structure: every section opens with a tracked-out monospace all-caps eyebrow and a square bullet,
meta strings are joined with middle dots forty-six times on the home page alone, section anchors
are numbered as if they were steps, and buttons end in an arrow. These are the exact tells the
skill lists, and together they make a well-built page read as generated. The quality floor is
mostly met: no horizontal overflow at 375 px, keyboard focus is visible everywhere, reduced motion
is respected, and body text passes AA on both surfaces. The captures still show the old shield
mark in the console sidebar, which is now the most visible inconsistency on every page.

## The token system as found

- **Colour.** Graphite `#14171E` (ink on light, ground on dark), Mist `#E8ECF2` (ground on light,
  ink on dark), Cobalt `#2E54E6` (action and accent on light), Cobalt Tint `#8DA4F2` (accent on
  dark), Slate `#5B6676` (muted text on both). Dark sections sit on `#12161F`, light on `#FFFFFF`
  over a `#F3F5F9` canvas. State colours (ok `#34B37A`, warn `#E8A33A`, crit `#EC5B53`) exist
  only in their dark-surface tuning. A `--brass` token survives with a comment reserving it for
  the retired Guard Chevron star (`site/assets/site.css:16`).
- **Type.** Three families: Barlow for display, IBM Plex Sans for body, IBM Plex Mono for the
  wordmark, code, and also eyebrows, chips, captions, table headers and capture title bars. The
  skill asks for one or two families; the third is justified only where it is code or the
  wordmark.
- **Layout.** A 1240 px container, left-aligned text, hero of headline, lead, two buttons and a
  framed capture; sections alternate light and dark from a light hero; features and security use
  a two-column text-plus-capture rhythm with definition-list facts; get-started uses two step
  cards side by side; FAQ is a single column of disclosures. Captures are framed as windows with
  three traffic-light dots and a mono title bar.
- **Principles the pages follow.** Evidence over claims (file paths and setting keys on marketing
  pages), unknown is never pass, read-only by default, simulator first.

## Findings

Severity: **must** (hurts credibility or the quality floor), **should** (a skill principle clearly
violated), **could** (a refinement).

### Global

1. **must — Captures show the old brand.** Every console capture under `site/assets/screens/`
   carries the Guard Chevron and the camel-case wordmark in the sidebar (`home-1440.png`, the hero
   capture, top-left). The page header shows the new icon and wordmark two hundred pixels above.
   Recapture every screen from a build of main.
2. **should — Mono all-caps eyebrow on every section.** `.eyebrow` (`site.css:312–326`) is IBM
   Plex Mono, 0.76 rem, uppercase, 0.08 em tracking, Cobalt, with a square `::before` bullet; the
   home page has seven, features four, get-started five, security four, FAQ two. This is the
   skill's "tracked-out ALL-CAPS eyebrow above every heading" tell in full, and the hero eyebrow
   is long enough to wrap to two lines at 375 px ("PRINTER FLEET SECURITY & CONFIGURATION
   CONSOLE", `index.html:73`). Change: keep an eyebrow only where it carries information the
   heading does not (the walkthrough's "01 · Read-only" mode labels do; "Technical FAQ" above
   "Direct answers on protocol scope" does not), set it in Plex Sans 500, sentence case, no
   tracking, no bullet; drop the hero eyebrow on every page, the headline already says it.
3. **should — Middle-dot meta strings.** Forty-six " · " joins in `index.html`, eight on each
   inner page: the brand tag "Open source · Self-hosted" (`index.html:34`), capture captions
   "Fleet security overview · Simulator data", protocol lists "SNMP v1/v2c/v3 · IPP/IPPS · mDNS ·
   WS-Discovery", and the step modes. The skill names this as template chrome. Change: keep the
   dot only inside the brand tag and the capture title bars, where it is a label; turn protocol
   lists into comma lists in Plex Sans and the walkthrough modes into a visible word ("Read-only
   step") rather than a dotted suffix.
4. **should — Numbered chips that are not a sequence.** The section navigation on features
   ("1. Discovery & Inventory … 7. Full Capability Matrix"), security ("1. Baseline Policy &
   Unknowns … 4. Encryption, RBAC & Audit") and get-started ("1. Evaluate with pm-sim …") numbers
   anchors that can be read in any order. The skill allows numbering only for real sequences.
   Change: unnumbered chips in Plex Sans; keep the numbers in the home walkthrough and the
   "Five checks" list, which are sequences.
5. **should — Arrows appended to button text.** Nine "→" in `index.html` (`:646`, `:700`, `:720`
   on the capability split buttons; "Next →" on the stepper at `:207`). Change: plain button
   labels; keep the ↗ glyph only on external links, where it is information.
6. **should — Monospace carries labels, not just code.** `--font-mono` is applied in twelve rules
   (`site.css:291, 317, 395, 589, 639, 793, 834` and more): eyebrows, chips, capture title bars,
   table headers, captions. Setting keys, file paths, commands and the wordmark are code and
   belong in mono; the rest is the skill's "monospace face for small data labels" tell. Change:
   mono only for `code`, `pre`, the wordmark's "print" and setting keys.
7. **should — Line length.** Measured at 1440: the two capture captions on the home page run
   about 165 characters at 13.4 px (the full 1115 px capture width); the get-started intro
   paragraph about 143 characters at 16 px; FAQ answers about 104 characters at 16 px. The skill's
   limit is 80. Change: `max-width: 68ch` on captions, section intros and `.faq` answers; captions
   at 14 px minimum.
8. **could — Three traffic-light dots on every capture frame.** The macOS window chrome
   (`index.html` capture frames) is decoration; the mono title bar already labels the capture.
   Removing the dots quiets every frame and makes the "Enlarge" control the only affordance.
9. **could — Stale token and comment.** `--brass` and its "Reserved strictly for the Guard
   Chevron star" comment (`site.css:16`) outlive the mark they describe; remove both so a future
   editor does not reintroduce brass.

### Home

10. **should — The hero follows the default recipe.** Eyebrow, headline, lead, primary and
    secondary button, framed screenshot, then a four-tile reassurance strip. The capture is the
    right hero for this product, and the headline is good. The band stacking is the issue: light
    hero, dark capture frame, light reassurance strip, dark walkthrough section gives four tone
    changes in the first screen (`home-1440.png`). Change: let the capture bleed into the first
    dark section, or make the reassurance row part of the hero's light surface without its own
    card border, so the first screen has two tones, not four.
11. **could — Reassurance strip is a card kit.** Four identical tiles, one radius, one border
    (`index.html:116–144`). The content is four short claims; a single line of four phrases with
    their icons, no tiles, says the same with less chrome.

### Features

12. **should — Numbered chips and dotted protocol strings** (findings 3 and 4 apply most here:
    "SNMP v1/v2c/v3 · IPP/IPPS · mDNS · WS-Discovery" beside every "Implemented" pill).
13. **could — Definition lists are the page's best device.** The DEDUPLICATION / OPERATOR FIELDS /
    LIFECYCLE STATES rows encode real structure and should stay; set the labels in Plex Sans
    sentence case rather than mono caps (finding 6).

### Security

14. **should — Table headers in mono caps** ("SETTING KEY", "EXPECTED RULE", `security/index.html`
    rule table). Keep the table; set headers in Plex Sans 600 sentence case.
15. **could — State pills are tuned for dark only.** Critical/High/Medium pills (`#EC5B53`,
    `#E8A33A` on their washes) read well on the dark table; if the table ever moves to a light
    section the same colours fall to 3.4:1 and 2.2:1 on white (see the quality floor). Define
    light-surface variants now so the choice of surface stays free.

### Get started

16. **should — The intro paragraph runs 143 characters** (finding 7) and the two step cards are
    the SaaS card pattern; the content is a real two-step sequence, so the numbering is right,
    but the cards could be two columns with a rule between them instead of two bordered boxes.
17. **could — Code blocks overflow horizontally at 375 px** inside their own scroll (pre
    `scrollWidth` 494–813 px against 291 px), which is correct behaviour, but the second block's
    longest line is the secret-key one-liner; wrapping it onto two lines removes the only
    sideways scroll a phone reader meets.

### FAQ

18. **should — Answers at 104 characters per line** (finding 7); otherwise the disclosure list is
    the most restrained page on the site and needs nothing else removed.

### 404

19. **could — Fine.** One heading, one sentence, one link; the eyebrow (finding 2) is the only
    chrome.

## The skill's generated-page tells

| Tell | Present | Where | Justified by the brief? |
| --- | --- | --- | --- |
| Cream background, serif display, terracotta accent | No | — | — |
| Near-black ground with one acid accent | Partly | dark sections `#12161F` with Cobalt Tint | Yes: the palette is the brand's, and the light sections break it |
| Broadsheet hairlines, zero radius, dense columns | No | — | — |
| SaaS card kit: identical rounded cards, one radius, same grey shadow, gradient washes | Partly | home reassurance strip, get-started step cards; radii vary (4/6/10/14 px) and shadows are few | Partly: two places, not everywhere |
| Tracked-out ALL-CAPS eyebrow above every heading | Yes | `.eyebrow`, 22 instances across the pages | No |
| Meta strings joined with middle dots | Yes | 46 on home, 8 per inner page | Only the brand tag and capture title bars |
| "WORD — fragment" labels with a spaced em dash | Partly | capture title bars "Terminal 1 — Build & start pm-sim" | Borderline: it is a title, not a label |
| Tinted near-black standing in for black | Partly | `#12161F`, `#0D1016` grounds | Yes: Graphite is the brand ink |
| Monospace face for small data labels | Yes | eyebrows, chips, captions, table headers | Only for code, keys and the wordmark |
| "→" appended to link and button text | Yes | 9 on home | No |

## Quality floor

- **Responsive.** `scrollWidth` equals the viewport on all six pages at 375 px; the security
  rule table and the get-started code blocks scroll inside their containers (`overflow-x: auto`),
  not the page. The mobile menu toggle is present. Twenty-eight interactive elements on the home
  page are under 32 px tall at 375 px (chips, inline links); raise the chip and nav-link hit
  height to 40 px.
- **Keyboard focus.** A global `:focus-visible` outline, 2 px Cobalt/Tint with 3 px offset
  (`site.css:218–224`), applies to all 67 focusable elements on the home page. Visible on both
  surfaces.
- **Reduced motion.** `@media (prefers-reduced-motion: reduce)` zeroes animation and transition
  durations and disables smooth scroll (`site.css:2184–2196`). There are no keyframe animations
  at all; the only motion is 0.1–0.15 s colour and transform transitions on hover and the
  stepper. The skill's restraint on motion is met.
- **Contrast (WCAG ratio, text on its section ground).** Dark: primary 16.6, secondary 9.7,
  muted 5.3, dim 3.1 (fails AA for small text; `--text-dim` is not used as a text colour, only
  for borders), Tint links 7.5, code 15.3, ok 6.8, warn 8.4, crit 5.3. Light on white: primary
  17.9, secondary 10.0, muted 5.8, Cobalt links and eyebrows 6.0 (5.5 on the `#F3F5F9` canvas,
  5.0 on Mist), Deep hover 9.4; state colours as text fail on white: ok 2.7, warn 2.2, crit 3.4.
  White on the Cobalt button 6.0. Every pair in use passes AA; the state colours need
  light-surface variants before they are used as text on a light section.
- **Palette harmony across surfaces.** Consistent: the two surfaces swap ink and ground and keep
  Cobalt as the action colour; the only hue outside the system is the state set.

## Restraint

The boldness is spent on the console capture in the hero, which is correct for an audience that
wants to see the tool. The accessory to remove, per page: home, the reassurance strip's tile
borders; features and security, the numbered chips; get-started, the step-card borders; FAQ,
the eyebrow; 404, the eyebrow.

## Prioritised change plan

| # | Change | Effort |
| --- | --- | --- |
| 1 | Recapture all console screens from a build of main with the new icon and wordmark; replace `site/assets/screens/*` | 2 h |
| 2 | Replace `.eyebrow`: Plex Sans 500, sentence case, no tracking, no bullet; delete the hero eyebrow on every page | 1 h |
| 3 | Remove numbering from the features, security and get-started section chips; keep numbers in the walkthrough and "Five checks" | 30 min |
| 4 | `max-width: 68ch` for captions, section intros and FAQ answers; captions 14 px | 30 min |
| 5 | Drop "→" from button labels; keep ↗ on external links only | 20 min |
| 6 | Confine `--font-mono` to `code`, `pre`, setting keys and the wordmark; chips, captions, table headers and definition labels in Plex Sans | 1 h |
| 7 | Replace dotted protocol and meta strings with comma lists or words; keep the dot in the brand tag and capture title bars | 45 min |
| 8 | Add light-surface state tokens (darker ok/warn/crit) and use them on light sections | 30 min |
| 9 | Raise chip and mobile nav-link hit height to 40 px; wrap the long secret-key line in the get-started terminal block | 30 min |
| 10 | Remove the `--brass` token and Guard Chevron comments; drop the traffic-light dots from capture frames | 20 min |

## Not verified

- Real-device rendering at 390 px (measured at 375 px in an emulated viewport; no narrower
  headless capture was possible).
- Font loading from Google Fonts under a content blocker; the fallback stacks were not
  screenshotted.
- Colour as perceived on the printed page or with colour-vision deficiency; ratios only.
- The 1024 px captures were taken but not reviewed line by line; nothing in them contradicts the
  1440 findings.

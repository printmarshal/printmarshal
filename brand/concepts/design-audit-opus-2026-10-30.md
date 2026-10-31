# PrintMarshal design audit (frontend-design rubric)

> Captures: converted to JPEG at 1000 px wide for the repository; the 1024 px and most 390 px site captures were dropped as duplicates of the 1440 px set. Reviewer: an Opus 5.5 subagent, 31 October 2026, from the `frontend-design` skill.

Date: 30 October 2026 (captures taken 30–31 October, local time).
Auditor: Claude (Opus), against Anthropic's `frontend-design` skill (`.claude/skills/frontend-design/SKILL.md`) and `brand/README.md`.
Scope: the marketing site in `site/` (home, features, security, get-started, faq, 404) and the product console in `web/`
(login, dashboard, devices, device detail, security, settings, discovery). Audit only: nothing under `site/` or `web/` was changed.
Screenshots: `brand/concepts/audit-shots-opus/` (names are cited below as `[shot-name]`).

How the evidence was gathered: the site was served with `python3 -m http.server 19301`; the console was built (`npm ci`,
`make web`, `make build`) and run against `pm-sim` (`generic-laser.snmp`, SNMP 19361, IPP 19331) on 127.0.0.1:19302 with a
fresh scratch data directory and a generated test password. One discovery of 127.0.0.1 (zero-config off) produced one
device; one Baseline assessment was run against it (loopback only). Captures were taken with headless Chrome driven over the
DevTools protocol, which allowed true 390 px and 400 px mobile emulation (the CLI's ~500 px floor did not apply). Contrast
ratios were computed from the token values with the WCAG 2.x relative-luminance formula, compositing alpha washes onto
their real ground.

---

## (a) Verdicts

**Website.** The site has a real subject and mostly earns it: the headline "Know where your printer fleet is exposed." is
specific, the hero is the product itself rather than a stat block, and the evidence trace (device, observation, finding,
next action, switchable between fail, unknown and pass) is the single most characteristic thing in this subject's world and
is genuinely good. The problem is everything around it. Every section is dressed in the same template chrome the skill
lists as generated-page tells: 23 tracked-out mono all-caps eyebrows that restate the heading beneath them, middle-dot meta
strings, "→" on every secondary link, numbered markers on content that is not a sequence, a single rounded-card kit for all
content, and radial gradient washes on every dark band. The type system has 35 distinct font sizes. Line lengths break the
80-character rule badly at 1024 px (103–124 characters) and in the FAQ at every desktop width (100–106). Two factual
problems undercut a site whose whole pitch is honesty: the hero and walkthrough captures predate the brand change (old
shield and camel-case "PrintMarshal" in the sidebar), and the copy promises results the current build does not produce
(SNMPv1/v2c "failing" on `pm-sim`, and "all web fonts in the console are bundled locally"). The quality floor (contrast,
focus, reduced motion, no horizontal scroll) is solid.

**Console.** The console is a competent, dense, quiet operations UI: tabular numerals, shape-plus-colour status pills,
honest empty states, sticky table heads, and a light/dark token set whose text pairs all pass AA. It does not yet look like
PrintMarshal, though. The brand's display face (Barlow) appears only inside the word "Marshal"; every heading is 20 px IBM
Plex Sans 600, so the screens read as a generic admin kit. The dashboard, the first screen after sign-in, says nothing about
security posture, which is the product's reason to exist. The Security screen's bold element is a green "100.0% compliant"
ring on a device where 10 of 14 Baseline items are unknown, which contradicts the brand promise "unknown is never counted as
pass" at the level of visual emphasis even though the arithmetic is right. Quality-floor gaps are real: no `:focus-visible`
styling (the jump search and filter chips remove the outline entirely), no `prefers-reduced-motion` handling, form-control
borders at 1.5:1, the destructive confirm button at 2.28:1 in dark mode, and Google Fonts loaded from the network by a
product sold as self-hosted.

---

## (b) Token systems as found

### Website (`site/assets/site.css`)

Colour, as actually used (not as declared):

| Name (role) | Hex | Where |
| --- | --- | --- |
| Graphite (ink on light) | `#14171E` | headings and body ink on light sections (`site.css:124`) |
| Mist ground (light sections) | `#F3F5F9` | light section ground, Mist lifted halfway to white (`site.css:117`); cards are `#FFFFFF` |
| Night (dark sections) | `#0D1016` → `#11151E` gradient, footer `#0A0D12` | dark bands (`site.css:2210-2215`, `1812`). Not the brand's Graphite `#14171E` |
| Cobalt (accent, light) | `#2E54E6` | links, eyebrows, primary button, focus ring |
| Cobalt Tint (accent, dark) | `#8DA4F2` | links, eyebrows, focus ring on dark |
| Slate (muted) | `#5B6676` | captions, capture frame |

Plus four status hues per surface (ok, warn, crit, info) and a dashed "unknown" treatment. In practice the palette is two
neutrals, two near-blacks, one accent in two tints, and a status set: about 12 colours on screen.

Type: Barlow 700 for h1/h2 (`letter-spacing:-0.02em`), Barlow 600 for h3/h4 and tab titles; IBM Plex Sans for body, buttons
and nav; IBM Plex Mono for code **and** for eyebrows, badges, sub-nav pills, footer headings, capture title bars, stepper and
trace stage labels. Scale: h1 56 px, hero 51 px, h2 37.6 px, h2 overridden inline to 28 px on features and security, h3
20 px, lead 19.4 px, body 16 px, then 30 small sizes between 0.66 rem and 1.12 rem (`grep font-size site.css`: 35 distinct
values; 34 more inline `style="font-size:…"` across the HTML).

Layout: 1240 px container, 24 px gutters (16 px under 760 px), everything left-aligned except the 404. Full-bleed bands
alternate light (Mist) and dark, with the change of surface standing in for the divider. Within bands: a two-column
"copy | product capture" split that swaps sides, card grids of 2, 3 or 4, and data tables. Product captures always sit in a
dark "window" frame with fake traffic-light dots.

Principles the pages follow: show the real product (every capture is from the app running on `pm-sim`); state scope
honestly (Implemented / Device-dependent / Simulator-only / Requires vendor driver badges); separate fail from unknown
visually (dashed borders for unknown); be technical (file paths, API routes, RFC numbers inline).

### Console (`web/src/styles.css`)

| Name (role) | Light | Dark |
| --- | --- | --- |
| Ground | `#F5F6F8` | `#0D1015` |
| Surface | `#FFFFFF` (surface-2 `#F1F3F6`) | `#141920` (surface-2 `#1A2029`) |
| Ink | `#14171C` | `#E6E9EE` |
| Secondary / caption | `#454D5A` / `#5F6876` | `#B0B8C4` / `#8E98A6` |
| Accent | Cobalt `#2E54E6` | Cobalt Tint `#8DA4F2` |
| Status | ok `#13703A`, warn `#94540A`, crit `#B3261E`, info `#0B6191`, off `#555E6B` | `#5ACF86`, `#F1B65A`, `#FF8A80`, `#64B9EC`, `#A0A9B6` |

Plus CMYK supply hues, five categorical chart hues (including an orange `#E07B39`), and an unused `--glint` Brass token
(`styles.css:12,22`; no component reads it).

Type: IBM Plex Sans for everything, 13 px base (`styles.css:26`), `.h1` 20 px/600, `.h2` 14 px/600, KPIs 24 px, captions
12 px, labels 11 px. Measured on screen: 11, 12, 13, 14, 15, 20, 22, 24 px, plus inline 10, 16 and 18 px (56 inline
`fontSize: 11` in TSX). IBM Plex Mono for models, IPs, serials, keys (justified: identifiers). Barlow appears only in
`.wm-marshal` (`styles.css:36`).

Layout: 224 px sticky sidebar (collapsible to 56 px, a drawer under 768 px), a 10 px-padded top bar with the jump search,
content capped at 1600 px with 24 px padding. Everything is `.card` (8 px radius, 1 px border, no shadow); grids are
`auto-fit` with 168/340/440 px minimums.

Principles: shape + colour + label for every status; read-only by default, writes previewed; operator-set vs discovered
provenance markers; unknown stays visible.

### Brand drift between the three sources

Three near-white grounds (`#E8ECF2` brand Mist, `#F3F5F9` site, `#F5F6F8` console) and four near-blacks (`#14171E`
Graphite, `#14171C` console ink, `#0D1016` site, `#0D1015` console) that differ by one or two units. None of the dark
surfaces uses Graphite or Gunmetal, the two neutrals the brand names.

---

## (c) Findings

Severity: **must** (wrong, broken, inaccessible or untrue), **should** (clear quality gap against the rubric), **could**
(polish).

### Global (both surfaces)

G1. **must — Brand: product captures show the retired mark.** Every console capture on the site (`site/assets/screens/*.png`;
visible in `[site-home-1440-fold]`) shows the old shield and camel-case "PrintMarshal" in the sidebar. Principle: build with
the brief's real content. Change: re-shoot all 19 captures from the current build (the sidebar now shows the MFP icon and
lowercase wordmark, `[console-dashboard-1440]`), same crops and filenames, so no HTML changes.

G2. **must — Truthfulness: the console loads Google Fonts.** `web/index.html` links `fonts.googleapis.com`; the built console
requested it in this run. `site/security/index.html:342` says "All web fonts and assets in the application console are
bundled locally" and "zero outbound cloud calls". On an air-gapped management VLAN the console silently falls back to
`system-ui`, so the type system the brand specifies never renders where the product actually lives. Change: self-host the
three families (`@fontsource/ibm-plex-sans`, `@fontsource/ibm-plex-mono`, `@fontsource/barlow`, or the woff2 files under
`web/src/assets/fonts` with `@font-face`), remove the two `preconnect` links and the stylesheet link.

G3. **should — One token source.** See "Brand drift" above. Change: in both `site.css` and `styles.css` set the light ground
to one value (recommend `#F3F5F9`, already documented as Mist-lifted), ink to Graphite `#14171E`, the dark ground to
Graphite `#14171E` and dark surfaces to steps between Graphite and Gunmetal (`#1B2029`, `#222834`, Gunmetal `#2A313D` for
borders/hover). Delete `--glint` and the `--brass` comment "Reserved strictly for the Guard Chevron star" (`site.css:16`,
header comment line 3), both references to the retired mark.

G4. **should — "Catalog" vs "catalogue".** The site says "research catalog" (e.g. `index.html:321`), the console says
"Catalogue" (`App.tsx:135`, `pages/Catalog.tsx:45`). Principle: the vocabulary of an interface is its signposting. Change:
pick one spelling for both surfaces (the console and README use British spelling elsewhere: "Colour", "behaviour"; so
"catalogue" on the site).

### Website, global

S1. **must — Copy promises a result the current build does not produce.** `get-started/index.html:184` tells the evaluator to
"Observe `SNMP v1/v2c enabled` failing (`× Fail`)"; home (`index.html:397-407`), security (`security/index.html:103`) and FAQ
make the same claim. On this build with `generic-laser.snmp`, the device assesses as 4/4 passing with 10 unknown and the
SNMP v1/v2c item is `unknown` ("setting not readable on this device", from `/api/devices/{id}/security`;
`[console-device-security-1440]`). Either this is a product regression (likely related to the SNMP identification change in
PR #29) or the copy is stale; the first-success checklist must match what a new user sees. Change: fix the product, or
rewrite step 3 to "Observe Telnet, FTP, HTTPS and raw-9100 passing and 10 items reported Unknown; nothing is counted as pass
that the printer did not report."

S2. **should — Eyebrows on every section (tell: "unnecessary typographic labels", "all caps for labels").** 23 `.eyebrow`
elements, all mono, uppercase, `letter-spacing:0.08em`, with a Cobalt square (`site.css:313-333`). Most restate the heading:
"CORE CAPABILITIES" over "Discover printers. Investigate risk. Review changes." (`index.html:624`), "TECHNICAL FAQ" over the FAQ
title (`faq/index.html:69`), "HTTP 404 · ROUTE NOT FOUND" (`404.html:59`). Change: delete every eyebrow except the hero's,
and set that one in sentence case, Plex Sans 500 14 px, Slate, no square: `.eyebrow{font-family:var(--font-sans);
text-transform:none;letter-spacing:0;font-size:.875rem;color:var(--text-muted)}`, `.eyebrow::before{display:none}`.

S3. **should — Numbered markers on content that is not a sequence.** Sequences (keep): walkthrough steps 01–05, the evidence
trace stages 01–04, get-started steps 1–5. Not sequences (remove the numbers): features sub-nav "1. Discovery & Inventory" …
"7." (`features/index.html:75-81`) and the matching "1. Discover printers…" h2s; security sub-nav and h2s
(`security/index.html:75-78, 94, 276`) and panel cards "1. Port & TLS Probes" (`:224-238`); home capability h3s
"1. / 2. / 3." (`index.html:638, 692, 712`); FAQ questions "1." … "8." (`faq/index.html:81-167`); and the circled
`.annotation-num` 1–5 under captures, which point at nothing in the image. The walkthrough also double-numbers: tab "01"
then h3 "1. Discover printers…" (`index.html:173, 214`).

S4. **should — Status badges used as decoration.** `cap-implemented` (green, "Implemented") labels "Single Binary"
(`index.html:804`); `cap-driver` (dashed, "Requires vendor driver") labels "Extensible" (`:824`); `cap-info` is used for
"Documentation" and "Cleanup" (`index.html:814`, `get-started/index.html:203`) but has no CSS rule, so it renders as bare mono
text (`[site-home-1440]`, open-source band). Principle: visual structure is information; the badge vocabulary is the site's
best device and is diluted when it decorates. Change: remove these three badges; keep the four status badges for status only.

S5. **should — Line length over 80 characters.** Measured characters per line (text length ÷ rendered lines):
home at 1024 px 103–107 (`.capability-copy p`), features at 1024 px 114–124, security at 1024 px 108–120 (including
`dd` at 120), get-started 124 at both 1024 and 1440 (the warn callout), FAQ 100–106 at 1024 and 1440 (container 920 px),
leads 85–89 on features and get-started. Change:
`.capability-copy > p, .capability-copy .spec-dl, .disclosure-body > *, .callout p, .panel-card p { max-width: 68ch; }`,
`.lead { max-width: 60ch; }`, and set the FAQ container to `max-width: 760px`.

S6. **should — Type scale.** 35 distinct `font-size` values in `site.css` and 34 inline overrides, including the h2 forced to
`1.75rem` seven times (`features/index.html:97,161,189,256,284`, `security/index.html:94,276`). The Elements of Typographic
Style scale the skill points to would be 6–7 steps. Change: define `--fs--1: .8125rem; --fs-0: 1rem; --fs-1: 1.25rem;
--fs-2: 1.5625rem; --fs-3: 1.953rem; --fs-4: 2.441rem; --fs-5: 3.052rem` (a 1.25 ratio) and map every rule onto it; give
the split-section h2 a class (`.h2-split{font-size:var(--fs-3)}`) instead of inline styles.

S7. **should — Template chrome (skill tell 5).** Middle-dot meta strings everywhere: capture title bars ("Fleet inventory ·
Simulator data"), badges ("Implemented · Read-Only", `index.html:213`), brand tag "Open source · Self-hosted"
(`index.html:34`), footer "No external analytics · Zero telemetry · Simulator-verified captures" (`index.html:914`). "→"
appended to every secondary button and many links ("Discovery & monitoring details →", `index.html:646, 700, 720, 784,
809`) and as the walkthrough bullet glyph (`site.css:1127-1133`). Spaced-em-dash labels: "Terminal 1 — Build & start pm-sim"
(`get-started/index.html:104`), lightbox captions "Step 1 — Fleet inventory" (`index.html:226`). Change: drop the arrows from
button labels (the button shape already says "go"); replace walkthrough bullets with `list-style: disc` in Slate; write
title bars as plain names ("Fleet inventory, simulator data" or just "Fleet inventory"); use a colon in code-block headers
("Terminal 1: build and start pm-sim").

S8. **should — Gradient washes as decoration (tell 4).** Radial Cobalt glow on every dark band (`site.css:2211-2213`), on
the hero and page heroes (`:2219-2221`), a linear wash on `.page-hero` (`:1665`), two different glows on `.closing-cta`
(`:1799`, `:2253-2255`), and tinted gradients on trace nodes (`:1259-1272`). Change: flat surfaces. `.surface-dark{background:
var(--graphite)}`, `.hero.surface-light,.page-hero.surface-light{background:#F3F5F9}`, closing CTA flat Graphite; keep only
the trace nodes' 3 px top rule (`:1376`), which encodes state.

S9. **should — One card kit for everything (tell 4).** `.panel-card`, `.problem-card`, `.walkthrough-copy`, `.trace-node`,
`details.disclosure`, `.table-wrap`, `.callout` and `.hero-reassurance` all share `border:1px; border-radius:10px` and, on
light, the same `0 1px 2px` shadow (`site.css:2229-2238`). Change: reserve the bordered card for things that are objects
(captures, code blocks, the trace); render prose groups (open-source trio, platform trio, FAQ) as unboxed columns separated
by space, with the h3 doing the grouping.

S10. **should — Repetitive CTAs.** "Evaluate locally with the built-in simulator." is the get-started h1 and the closing CTA
headline on home, features, security and FAQ (`index.html:844`, `features/index.html:444`, `security/index.html:357`,
`faq/index.html:186`). The features h1 "What works with your printers today." repeats the home h2 (`index.html:751`). Change:
one closing CTA per page, written for that page: features "Try every capability above on pm-sim"; security "Run a Baseline
assessment on pm-sim"; FAQ: remove the CTA band and end on the last answer plus a "Ask on GitHub Discussions" link.

S11. **should — Header has two primary buttons in view at once.** Header "Get started" and hero "Explore the product" are both
filled Cobalt (`[site-home-1440-fold]`), and the hero's primary CTA only scrolls to `#walkthrough`. Principle: a CTA says
exactly what happens. Change: hero primary "Try it on the simulator" → `/get-started/#simulator`; demote "Explore the
product" to a text link "See the walkthrough"; drop the hero's secondary button.

S12. **could — Mono for small non-data labels (tell 5).** Sub-nav pills, footer h4s, stepper, capture title bars, badges and
the brand tag are mono (`site.css:394-403, 1680-1689, 1825-1832`). The brand reserves Plex Mono for "code and identifiers"
(`brand/README.md`, Typography). Change: these to Plex Sans 500; keep mono for code, keys, IPs and the "print" in the wordmark.

S13. **could — Fake browser chrome on captures.** `.console-dots` traffic lights and a mono title bar on all 15 captures, plus
an "Enlarge" button on each. Change: remove the dots (they are the "accessory" in the restraint test), keep a 1 px Slate frame
and the caption below; the whole image is already clickable for the lightbox.

S14. **could — Inaccurate alt text.** `index.html:268` says "showing 5 non-compliant devices"; the image shows 3 compliant and
2 non-compliant. `index.html:104` and `:269` cite "81.8 score" for images that show 92.7/60%. Change: rewrite alt text when the
captures are re-shot (G1).

S15. **could — Leftover markers.** "*before*" with literal asterisks renders in `security/index.html:286`; `site.css:2196` says
"DRAFT 2026-10-30, not live" about rules that are live. Change: `<em>before</em>`; update the comment.

### Website, per page

**Home** (`[site-home-1440]`, `[site-home-1440-fold]`, `[site-home-1024]`, `[site-home-390]`)

H1. **should — The bold element is in the wrong place.** The memorable, subject-specific thing is the evidence trace
(`index.html:390-614`), currently the second half of the second section. The hero spends its weight on a full-width
screenshot whose content is small and dense at 1440 px. Change: move the trace into the hero, directly under the lead, as
the one bold element (it already explains "fail vs unknown vs pass" in one glance); move the compliance screenshot into
walkthrough step 2 where it already appears.

H2. **should — Accessory to remove:** the four-item "hero-reassurance" strip (`index.html:116-153`). Its four claims repeat
the walkthrough's scope notes and the open-source section; at 390 px it adds a fifth stacked card before the first section
ends (`[site-home-390]`).

H3. **could — Trace stage label wraps beside its badge** ("04 · NEXT / ACTION" on two lines next to "VENDOR DRIVER" at 1440,
`[site-home-1440]`). Change: put the badge on its own line under the stage label inside `.trace-stage` (`flex-wrap:wrap`).

**Features** (`[site-features-1440]`, `[site-features-1024]`, `[site-features-390]`)

F1. **should — Bold element:** the capability matrix (`features/index.html:358-437`) is the page's most useful artefact and
sits last. Change: put the matrix directly after the page hero, then the six detailed sections. **Accessory to remove:** the
numbered pill sub-nav (`:74-82`), which the matrix replaces as a table of contents.

F2. **should — Spec lists are dense prose in a two-column `dl`** with file paths inline (`:101-108`), reaching 124 characters
per line at 1024. Change: cap with S5; move source paths to a trailing "Source:" line per item in Slate.

**Security** (`[site-security-1440]`, `[site-security-1024]`)

SE1. **should — Severity painted with pass/fail colours.** The Baseline table renders "Critical (3.0)" as `status-fail` red
and "High (2.0)" as `status-warn` amber (`security/index.html:140-200`), the same pills used for Fail and Device-dependent
elsewhere. Principle: colour encodes one thing. Change: render severity as plain text with a weight number, or a neutral
outline pill, never the fail/warn pills.

SE2. **should — Bold element:** the 14-rule table (`:126-205`) and its "How PrintMarshal observes it today" column, which is
the page's argument. **Accessory to remove:** the second appearance of `crop-device-security.png` (it is already on home twice).

**Get started** (`[site-get-started-1440]`, `[site-get-started-390]`)

GS1. **must — see S1** (step 3 promises a failing check).

GS2. **should — "Five checks" grid has six cards.** `get-started/index.html:160-207`: five numbered steps plus an unnumbered
"Stop background processes" card with the unstyled `cap-info` badge. Change: make the steps an ordered list (`<ol>`) in one
column (it is a sequence; a 3-column grid breaks the reading order at 1440 into 1-2-3 / 4-5-cleanup) and move cleanup to a
one-line note after it.

GS3. **could — Eyebrow "Path 1 · Recommended First Step"** (`:89`) and "Path 2 · Lab VLAN or Production Deployment" (`:218`).
These do encode a choice of path; keep them as plain sentence-case h2 prefixes ("Path 1: evaluate on the simulator").

**FAQ** (`[site-faq-1440]`)

FQ1. **should — All eight disclosures are `open`** (`faq/index.html:80-166`), so the `+/−` affordance and the summary styling
do nothing, and the page is one 3,700 px wall at 100–106 characters per line. Change: close all but the first; cap measure
(S5); drop the numbering (S3). **Bold element:** the first question and answer ("Does discovery or polling ever write to my
printers? No."). **Accessory to remove:** the closing CTA band (S10).

**404** (`[site-404-1440]`)

N1. **should — Headline names the wrong product surface.** "That page is not in the console." (`404.html:60`); this is the
website. Change: "That page does not exist." Lead: "Check the address, or go to the overview." **Accessory to remove:** the
eyebrow "HTTP 404 · Route Not Found". Three buttons are one too many: keep "Go to the overview" (primary) and "Get started".

### Console, global

C1. **must — No visible focus on two primary controls.** There is no `:focus-visible` rule in `styles.css`. Most controls fall
back to Chrome's default `auto 1px` ring (measured on sidebar links; visible, `[console-devices-1440-focus]`), but
`.search input{outline:none}` (`styles.css:60`) and `.chip select{outline:none}` (`:117`) remove it and nothing replaces it:
the jump search and every filter chip have no focus indicator (measured: input outline `none`, wrapper border unchanged).
Change: `:focus-visible{outline:2px solid var(--accent);outline-offset:2px}` globally;
`.search:focus-within,.chip:focus-within{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}`.

C2. **must — Destructive confirm button fails contrast in dark mode.** `.btn.dngf{background:var(--crit);color:#fff}`
(`styles.css:129`) gives white on `#FF8A80` = **2.28:1**. This is the confirm button in Remediate, Revert, Apply and token
revoke dialogs (`components/security/common.tsx:282`, `components/config/RevertPanel.tsx:71`, and five more). Change: in the
dark token set add `--on-crit:#2A0E0C` and use `.btn.dngf{color:var(--on-crit)}` (dark ink on `#FF8A80` ≈ 8.5:1), keeping
`#fff` in light (6.54:1).

C3. **must — Form-control borders below 3:1** (WCAG 1.4.11). Inputs, selects, chips and segmented controls use
`--border-strong`: `#CBD1D9` on white = **1.54:1** (light), `#353E4B` on `#141920` = **1.63:1** (dark). Change: a dedicated
`--control-border` of `#8A93A1` (light, 3.2:1 on white) and `#6B7686` (dark, 3.3:1 on `#141920`) for `.fin`, `.chip`,
`.seg`, `.qty`, `.search`, checkboxes; leave card borders as they are (decorative, exempt).

C4. **must — No reduced-motion handling.** `@keyframes pulse` runs infinitely on skeletons (`styles.css:224-225`) and the
drawer slides (`:79`); under emulated `prefers-reduced-motion: reduce` the sidebar transition still measured 0.18 s. Change:
`@media (prefers-reduced-motion: reduce){*,*::before,*::after{animation:none!important;transition:none!important}}`.

C5. **should — Brand display face absent from the product.** Barlow is loaded but used only in `.wm-marshal`
(`styles.css:36`); the computed scale on the dashboard is all IBM Plex Sans (11–24 px). Principle: typography carries the
personality. Change: `.h1{font-family:var(--display);font-size:24px;font-weight:600;letter-spacing:-.015em}` and the KPI/
compliance numbers (`.kv`, `.stat .v`) in Barlow 600 with `font-variant-numeric:tabular-nums` (Barlow has tabular figures);
leave everything else in Plex Sans.

C6. **should — Mark usage against the brand rules.** The sidebar and login show the full Cobalt app icon (`App.tsx:129`,
`Login.tsx:41`); `brand/README.md` assigns the shield-P glyph to "the sidebar" and the Black colourway to "the console's own
dark surface". The `dark` prop of `Mark` is accepted and ignored (`icons.tsx:70`). Change: sidebar uses
`glyph-shield-p` at 22 px in `--accent`; when `data-theme="dark"`, any full icon uses `app-icon-black.svg`. The committed
`internal/web/dist/index.html` still carries the old favicon with the Brass star; anyone building the Go binary without
`make web` ships the retired mark, so rebuild and commit it.

C7. **should — Raw system strings in the UI.** Activity feed shows "admin auth.login · signed i" and "admin security.assess ·
fd74a9eb" (`Dashboard.tsx:176`, default case slices the target to 8 characters); the jobs card titles a job "security.assess"
(`Dashboard.tsx:120`). Principle: name things by what users understand. Change: add cases: `auth.login` → "signed in",
`security.assess` → "ran a Baseline assessment on 1 device", and a job-title map (`security.assess` → "Security
assessment"); the default should be "made a change (`action`)" without the truncated id.

C8. **should — Pluralisation.** "1 devices" (`Dashboard.tsx:46`, `Devices.tsx:63`, `Security.tsx:41`), "1 rows"
(`Devices.tsx:185`), "+0 in 1 days" (`DeviceDetail.tsx` counters), "1 policy · 1 devices". Change: a `plural(n,'device')`
helper used everywhere.

C9. **should — Middle-dot meta subtitles under every h1** ("1 policy · 1 devices · every write runs as a dry run first",
`Security.tsx:41`; "Secrets are encrypted at rest and never returned by the API · device and group scopes take precedence over
global", `Credentials.tsx:37`; `Firmware.tsx:254` puts a nine-state lifecycle in a card subtitle). Each mixes a count with a
policy statement. Change: the subtitle carries counts only; move the statements into the empty state or the relevant
dialog.

C10. **should — "→" on every card-header link** ("All alerts →", "Jobs →", "Audit trail →", "Consumables →",
`Dashboard.tsx:72-137`; "open device →", `Discovery.tsx:112`). Change: drop the arrow; name the destination ("View all
alerts", "Open audit trail").

C11. **should — Uppercase labels.** `.navsep` "MANAGE" and `.lbl` (`styles.css:37,56`) in drawers: "EDIT POLICY", "REMEDIATE ·
DRY RUN", "ENROLL / RENEW CERTIFICATE" above the drawer's h2 (`PolicyEditor.tsx:82`, `RemediateFlow.tsx:48`,
`EnrollDrawer.tsx:45`), plus `.badge` uppercase. Change: `.lbl,.navsep{text-transform:none;letter-spacing:0;font-size:12px}`;
in drawers drop the label and make the h2 say it ("Edit policy: Baseline").

C12. **should — Double chevron on every filter chip.** `ChipSelect` renders a native `<select>` (with its own arrow) and an
`Ic.chevronDown` (`ui.tsx:63-67`); visible as "Any ⌄ ⌄" in `[console-devices-1440]`. Change: `.chip select{appearance:none}`.

C13. **should — Tables clip inside cards at 1440.** "Last seen" is cut to "Last" in Recent alerts (`[console-dashboard-1440]`)
and the Remediation column is cut mid-word on Security (`[console-security-1440]`). Change: on the dashboard, drop the
Source column from Recent alerts (the tag is redundant with the alert text) or let the grid go to one column below 1600 px;
on Security give the Remediation cell `white-space:normal;min-width:160px`.

C14. **could — Light-mode amber is brown.** `--warn:#94540A` fills the 14 px fleet-health bar (`[console-dashboard-1440]`) as a
solid brown band. It passes text contrast but reads as mud next to Cobalt. Change: keep `#94540A` for text, add
`--warn-fill:#D99A2B` for bars and segments.

C15. **could — Viewport overflow at 400 px on Settings → Appearance.** The theme segmented control overflows the card; the page
measured 450 px wide at a 400 px viewport (`[console-settings-appearance-400]`). Change: `.srow2` to one column under 480 px
(`grid-template-columns:1fr`).

### Console, per screen

**Login** (`[console-login-1440]`, `[console-login-1440-error]`, `[console-login-400]`, `[console-login-1440-dark]`)

L1. **should — Error copy.** Wrong password shows "invalid credentials", lowercase and unspecific (`Login.tsx:33` fallback
"login failed"). Principle: explain what went wrong and how to fix it. Change: "That username and password don't match. Check
both, or ask an admin to reset your password."

L2. **should — The first-run hint is always shown** ("First run: the admin password comes from PM_ADMIN_PASSWORD…",
`Login.tsx:60`), on every sign-in for the life of the install. Change: show it only when the server reports no sign-in has
ever succeeded (add a `firstRun` boolean to `/api/auth/providers`), and put the env var in mono.

L3. **could — Bold element / accessory.** The screen has no bold element: a 380 px card on blank grey. Give the brand its one
moment here: the full lockup (icon + wordmark) above the card at 40 px, title "Sign in", and remove the inline wordmark from
the title sentence ("Sign in to print Marshal" sets the brand mark as running text). Remove the mono host line
"127.0.0.1:19302 · self-hosted" (accessory).

**Dashboard** (`[console-dashboard-1440]`, `[console-dashboard-1440-dark]`, `[console-dashboard-400]`)

D1. **must — No security posture on the first screen.** Six KPIs, fleet health, alerts, attention, supplies, jobs, activity,
stock, reorder: nothing about policy compliance, failing items or unknowns, which is the product's headline. Change: replace
the six KPI cards with one posture strip at the top: "Baseline: 1 compliant, 0 failing, 10 items unknown on 1 device · last
assessed 8 min ago · Open security", where *unknown* gets the dashed treatment, and keep device health as the second row.

D2. **should — Big-number KPI row (the skill's named default).** Six identical cards, each a small label, a 24 px number and a
caption (`Dashboard.tsx:52-59`). Two are about consumables on a security product's home. Change: after D1, keep three
numbers (devices online, active alerts, supplies low) inline in the page subtitle or a single compact stat bar; move
"Consumables low on stock" and "Upcoming replacements & reorder forecast" to the Consumables page (accessory to remove).

D3. **could** — "Auto-refresh every 30 s" with a clock icon beside a Refresh button (`Dashboard.tsx:48`): remove the label;
put "updated 12 s ago" in the subtitle.

**Devices** (`[console-devices-1440]`, `[console-devices-400]`)

DV1. **should** — C12 (double chevron) and C8 ("1 rows", "1 devices"). Bold element: the table, correctly. Accessory to
remove: the footer note "Decommissioned devices are hidden unless you filter by that lifecycle state" on every visit; put it in
the Lifecycle chip's option list ("Any (excludes decommissioned)").

**Device detail** (`[console-device-overview-1440]`, `[console-device-security-1440]`, `[console-device-overview-1440-dark]`)

DD1. **should — Column imbalance.** The left column ends at ~650 px while the right column (Status, Identity, Support &
evidence) runs to ~2,400 px. Change: move "Support & evidence" under Trays in the left column, or make Identity a collapsible
section.

DD2. **should — Duplicate line.** "make and enterprise number are not in the catalogue" prints twice in Support & evidence
(`components/catalog/SupportCard.tsx:31` and the line after). Change: show it once.

DD3. **should — Security tab: "4 / 4 items passing · Compliant" over a list of 10 unknown items.** The progress bar is full
green; the unknown list is grey 12 px text. Change: show the denominator honestly: "4 passing, 0 failing, 10 unknown (not
readable over SNMP/IPP)", with a three-segment bar (green / red / dashed grey) instead of "4/4".

**Security** (`[console-security-1440]`, `[console-security-1440-dark]`, `[console-security-400]`)

SC1. **must — The bold element over-claims.** The ring reads "100.0% compliant" in green and "Compliant devices 1 — all
evaluated items pass" (`Security.tsx:122-131`) while 10 of 14 Baseline items are unknown on the only device; "unknown" is not
in the KPI row at all. "Not assessed 0" refers to devices, not items, which reads as "nothing unknown". This is the brand
promise ("unknown is never counted as pass") contradicted by emphasis. Change: make the KPI row Passing / Failing / Unknown
item counts across the fleet, with Unknown in the dashed style; label the ring "of assessed items readable" or replace it with
a stacked bar pass/fail/unknown. Also the "100.0%" label overruns its ring (`ComplianceRing`): drop the decimal.

SC2. **should — Permanent amber banner** ("Remediation can change how devices are reached…", `Security.tsx:138`) on every
visit, 150+ characters per line at 1440. Banner blindness, and it competes with SC1 for attention. Accessory to remove: move
the text into the Remediate dialog's first step, where it is actionable.

SC3. **could** — The empty "Trend · last 30 days" card occupies a third of the width on a new install. Hide it until two
assessments exist.

**Settings** (`[console-settings-1440]`, `[console-settings-400]`, `[console-settings-appearance-1440]`)

ST1. **should — Placeholders as labels.** "username", "password (min 8)" in the Add user form (`Settings.tsx` Users) have no
visible labels; the hint disappears as you type. Change: visible labels above each field, sentence case ("Username",
"Password, at least 8 characters").

ST2. **could** — Subtitle "PrintMarshal · self-hosted at 127.0.0.1:19302" (`Settings.tsx:38`): accessory to remove; the host is
in the address bar. C15 (Appearance overflow at 400 px).

**Discovery** (`[console-discovery-1440]`)

DS1. **could** — "Zero-config listen" option copy "mDNS / DNS-SD and WS-Discovery announcements · passive; needs the same L2
network as the printers" mixes a dot and a semicolon; write two sentences.

---

## (d) Generated-page tells

| Tell (skill) | Website | Console |
| --- | --- | --- |
| 1. Warm cream + high-contrast serif + terracotta | Absent | Absent |
| 2. Near-black ground with one bright accent | Partly: dark bands `#0D1016`→`#11151E` with Cobalt Tint as the only accent (`site.css:2210`). Brief justifies a dark/Cobalt pairing, but asks for Graphite, which is not used | Partly: dark theme `#0D1015` + Tint; theme is user-selected, brand-justified |
| 3. Broadsheet hairlines, zero radius, dense columns | Absent | Absent |
| 4a. Content chopped into identical rounded cards | Present: one 10 px card kit for prose, trace, FAQ, tables, callouts (`site.css:942, 1097, 1232, 1574, 1697`) | Present: every region is `.card` 8 px; six identical KPI cards (`Dashboard.tsx:52-59`) |
| 4b. Same soft shadow under each card | Present on light: `0 1px 2px rgba(20,23,30,.06)` on eight component types (`site.css:2229-2238`) | Absent (borders, no shadow) |
| 4c. Gradient washes as decoration | Present: dark bands, hero, page hero, closing CTA, trace nodes | Absent |
| 5a. Tracked-out ALL-CAPS eyebrow above every heading | Present: 23 eyebrows, mono, 0.08em (`site.css:313`) | Partly: `.lbl` above drawer h2s, `.navsep` "MANAGE", uppercase `.badge` |
| 5b. Meta strings joined with middle dots | Present: capture title bars, badges, brand tag, footer | Present: nearly every h1 subtitle and card subtitle |
| 5c. "WORD — fragment" spaced em-dash labels | Present: code-block headers, lightbox captions | Partly: search placeholder "Jump to device — name, IP…", "Drift — {profile}" (`Drift.tsx:66`), tooltips |
| 5d. Tinted near-black standing in for black | Present: `#0D1016`, `#0A0D12`, `#0B0D12`; the brand's own dark is Graphite `#14171E`, so a tinted dark is justified, but not these values | Present: `#0D1015`; same note |
| 5e. Monospace for small data labels | Present beyond justification: eyebrows, badges, sub-nav, footer heads, stepper (brand reserves mono for code and identifiers) | Mostly justified: models, IPs, serials, keys; a few `.cap.mono` captions (`Login.tsx:42`) |
| 5f. "→" appended to links and buttons | Present: secondary buttons, card links, bullet glyph | Present: every card-header link |
| Accent on one word in a headline | Absent (the two-colour wordmark is brand-mandated) | Absent |
| All caps for labels | Present (eyebrows, table heads, `dt`, footer h4, trace stages) | Partly (`.navsep`, `.lbl`, `.badge`; table heads are sentence case, good) |
| Unnecessary typographic labels above content | Present (eyebrows restating h2s) | Partly (drawer `.lbl` above h2) |
| Big number + small label + stats (+ gradient) as default hero | Absent on the site hero (product capture instead) | Present: dashboard KPI row; Security ring + four stats |
| Numbered markers on non-sequences | Present: features/security sub-navs and h2s, capability h3s, FAQ, annotation circles. Justified: walkthrough 01–05, trace 01–04, get-started steps | Absent |
| Fade-and-slide entrances, hover on every card | Absent (no entrance motion; hover only on controls) | Absent (pulse skeleton only) |

---

## (e) Quality floor

**Responsive.** Website: no horizontal scroll at 390, 1024 or 1440 on any of the six pages (`scrollWidth` equal to the
viewport in every case; true 390 px via device emulation). Mobile swaps desktop captures for focused crops, and the walkthrough
becomes a 5-up stepper with Prev/Next, both good. Console: no overflow at 400 px on dashboard, devices, device detail,
security, settings (users) and discovery; **overflow on Settings → Appearance** (450 px, C15). Tables scroll inside their
cards on mobile, acceptable for an operations console.

**Keyboard focus.** Website: global `:focus-visible` 2 px ring, Cobalt on light (5.46:1 against the ground) and Tint on dark
(7.59:1); measured on skip link, brand, nav, walkthrough tab. Skip link present. Console: no custom focus style; browser
default ring on links and buttons; **no indicator on the jump search or filter chips** (C1).

**Reduced motion.** Website: handled (`site.css:2184-2193`; measured transitions 0.00001 s and `scroll-behavior:auto` under
`reduce`). Console: **not handled** (C4).

**Contrast (WCAG 2.x, computed).** Alpha washes composited onto their actual ground.

Website, light sections (ground `#F3F5F9`, card `#FFFFFF`):

| Pair | Colours | Ratio | AA |
| --- | --- | --- | --- |
| Body (`--text-secondary`) | `#3A4352` on `#F3F5F9` | 9.14:1 | pass |
| Body on card | `#3A4352` on `#FFFFFF` | 9.98:1 | pass |
| Secondary/muted (Slate) | `#5B6676` on `#F3F5F9` | 5.33:1 | pass |
| Link and eyebrow (Cobalt) | `#2E54E6` on `#F3F5F9` | 5.46:1 | pass |
| Link on card | `#2E54E6` on `#FFFFFF` | 5.95:1 | pass |
| Link hover (Deep Cobalt) | `#1E3AA8` on `#F3F5F9` | 8.64:1 | pass |
| Primary button | `#FFFFFF` on `#2E54E6` | 5.95:1 | pass |
| Secondary button | `#14171E` on `#FFFFFF` | 17.93:1 | pass |
| Brand tag | `#2E54E6` on composited `#DEE4FB` | 4.71:1 | pass (narrow) |
| Badges: Implemented / Device-dependent / Fail | `#11744A` / `#855000` / `#B3261E` on their washes | 5.18 / 6.03 / 5.78:1 | pass |

Website, dark sections (ground `#11151E` worst case, card `#171C27`):

| Pair | Colours | Ratio | AA |
| --- | --- | --- | --- |
| Body | `#B4BECE` on `#11151E` | 9.74:1 | pass |
| Body on card | `#B4BECE` on `#171C27` | 9.09:1 | pass |
| Muted | `#7F8B9E` on `#11151E` / `#171C27` / panel `#1C2230` | 5.30 / 4.94 / 4.61:1 | pass (panel is narrow) |
| Link and eyebrow (Tint) | `#8DA4F2` on `#11151E` | 7.59:1 | pass |
| Primary button | `#FFFFFF` on `#2E54E6` | 5.95:1 | pass |
| Secondary button | `#F2F5FA` on `#171C27` | 15.60:1 | pass |
| Footer muted | `#7F8B9E` on `#0A0D12` | 5.64:1 | pass |
| Badges: Implemented / Device-dependent / Fail | on composited washes | 8.69 / 8.40 / 6.68:1 | pass |

Console, light (ground `#F5F6F8`, surface `#FFFFFF`, surface-2 `#F1F3F6`):

| Pair | Colours | Ratio | AA |
| --- | --- | --- | --- |
| Body | `#14171C` on `#FFFFFF` | 17.96:1 | pass |
| Secondary (`--text-2`, table heads, nav) | `#454D5A` on `#F1F3F6` | 7.67:1 | pass |
| Caption (`--text-3`) | `#5F6876` on `#FFFFFF` / `#F5F6F8` / `#F1F3F6` | 5.63 / 5.21 / 5.07:1 | pass |
| Link | `#2E54E6` on `#FFFFFF` | 5.95:1 | pass |
| Active nav | `#2E54E6` on `#EEF2FE` | 5.32:1 | pass |
| Primary button | `#FFFFFF` on `#2E54E6` | 5.95:1 | pass |
| Danger filled button | `#FFFFFF` on `#B3261E` | 6.54:1 | pass |
| Warn button / pill | `#94540A` on `#FCF1DE` | 5.32:1 | pass |
| Pills ok / crit / info / off | | 5.42 / 5.63 / 5.82 / 5.65:1 | pass |
| **Input border** (non-text, 3:1) | `#CBD1D9` on `#FFFFFF` | **1.54:1** | **fail** |

Console, dark (ground `#0D1015`, surface `#141920`, surface-2 `#1A2029`):

| Pair | Colours | Ratio | AA |
| --- | --- | --- | --- |
| Body | `#E6E9EE` on `#141920` | 14.50:1 | pass |
| Secondary | `#B0B8C4` on `#1A2029` | 8.19:1 | pass |
| Caption | `#8E98A6` on `#141920` / `#0D1015` / `#1A2029` | 6.05 / 6.53 / 5.61:1 | pass |
| Link (Tint) | `#8DA4F2` on `#141920` | 7.33:1 | pass |
| Active nav | `#8DA4F2` on `#1B2550` | 6.11:1 | pass |
| Primary button | `#0D1015` on `#8DA4F2` | 7.92:1 | pass |
| **Danger filled button** | `#FFFFFF` on `#FF8A80` | **2.28:1** | **fail** |
| Pills ok / warn / crit / info | | 7.87 / 8.51 / 7.13 / 7.16:1 | pass |
| **Input border** (non-text, 3:1) | `#353E4B` on `#141920` | **1.63:1** | **fail** |

Card borders on both surfaces are 1.16–1.40:1; they are decorative (the card is identified by its content), so not a
1.4.11 failure, but on light the site relies on them plus a 6 % shadow to separate white cards from a near-white ground.

**Palette harmony.** Website: harmonious; one cool neutral family plus Cobalt; status hues are muted enough to sit beside
Cobalt. The dark bands' blue glow tints every dark section slightly violet against the Graphite brand neutral. Console: the
light-mode warn `#94540A` reads brown as a fill (C14); the categorical orange `#E07B39` is the only warm hue and appears only in
charts (fine). Across surfaces, the near-duplicate neutrals (G3) are the main disharmony.

---

## Restraint test (one bold element, one accessory to remove)

| Page / screen | The bold element (now → recommended) | Remove |
| --- | --- | --- |
| Home | Hero screenshot → the evidence trace, moved into the hero | Hero reassurance strip |
| Features | (none) → capability matrix, moved to the top | Numbered sub-nav pills |
| Security (site) | (none) → the 14-rule Baseline table | The repeated device-security crop |
| Get started | Terminal code blocks (right choice) | "Path 1 · Recommended First Step" eyebrow |
| FAQ | (none) → the first answer, "No." | Closing CTA band |
| 404 | Headline (right choice, wrong words) | The "HTTP 404 · Route Not Found" eyebrow |
| Login | (none) → full lockup above the form | Mono host line |
| Dashboard | Six KPIs → a security posture strip | Consumables stock and reorder cards |
| Devices | The table (right) | Decommissioned footnote |
| Device detail | Supplies (fine) | Duplicate catalogue line |
| Security (console) | "100.0%" ring → pass / fail / unknown counts | Permanent amber banner |
| Settings | (none needed) | Host subtitle |

---

## (f) Prioritised change plan

### Website (at most ten)

| # | Change | Findings | Effort |
| --- | --- | --- | --- |
| 1 | Re-shoot all 19 captures from the current build; fix their alt text | G1, S14 | M (half a day with `pm-sim` and the existing crop list) |
| 2 | Make the copy match the build: SNMPv1/v2c claims (or fix the product), "fonts bundled locally" (or fix the console, console #1) | S1, G2 | S (copy) / M (if product fix) |
| 3 | Remove 22 of 23 eyebrows; restyle the hero's as sentence-case Plex Sans | S2 | S (1 h) |
| 4 | Remove numbering from non-sequences; fix walkthrough double numbering | S3 | S (1 h) |
| 5 | Cap measure: `max-width:68ch` on split copy, disclosures, callouts, panel text; FAQ to 760 px; close all but the first FAQ item | S5, FQ1 | S (1 h) |
| 6 | Flatten surfaces: drop gradient washes, set dark bands to Graphite; unify neutrals with the console | S8, G3 | S (2 h) |
| 7 | Move the evidence trace into the home hero; drop the reassurance strip; one primary CTA that goes to the simulator | H1, H2, S11 | M (half a day) |
| 8 | Collapse the type scale to seven steps; delete inline `style=` font sizes | S6 | M (half a day) |
| 9 | Strip template chrome: arrows on buttons, middle-dot title bars, em-dash labels, mono on non-code labels, traffic-light dots | S7, S12, S13 | S (2 h) |
| 10 | Page-specific fixes: severity pills on the security table, `cap-info` badges, matrix first on features, ordered list on get-started, 404 copy, CTA copy per page, literal asterisks | SE1, S4, F1, GS2, N1, S10, S15 | M (half a day) |

### Console (at most ten)

| # | Change | Findings | Effort |
| --- | --- | --- | --- |
| 1 | Self-host the three font families; remove Google Fonts links | G2 | S (1 h) |
| 2 | Accessibility floor: global `:focus-visible`, focus-within on search and chips, reduced-motion block, `--control-border`, dark `--on-crit` | C1–C4 | S (2 h) |
| 3 | Security screen: pass / fail / unknown as the headline numbers; no "100%" when most items are unknown; device tab three-segment bar | SC1, DD3 | M (half a day) |
| 4 | Dashboard: add the security posture strip; cut the KPI row to three; move consumables cards to Consumables | D1, D2 | M (1 day) |
| 5 | Barlow for h1 and headline numbers; unify the scale to 12/13/15/20/24 px and remove inline font sizes | C5 | M (half a day) |
| 6 | Human copy for audit and job events; pluralisation helper; login error and first-run hint | C7, C8, L1, L2 | S (2–3 h) |
| 7 | Mark usage per brand rules (glyph in sidebar, Black icon on dark); rebuild and commit `internal/web/dist/index.html` | C6 | S (1 h) |
| 8 | Template chrome: arrows on links, uppercase `.lbl`/`.navsep`, subtitles carry counts only | C9–C11 | S (2 h) |
| 9 | Layout fixes: chip double chevron, clipped table columns, device-detail column balance, Appearance overflow at 400 px | C12, C13, DD1, C15 | S (2–3 h) |
| 10 | Unify tokens with the site; warn fill colour; remove `--glint`; duplicate catalogue line; visible labels in Add user; remove the permanent remediation banner | G3, C14, DD2, ST1, SC2 | S (2–3 h) |

---

## (g) What I could not verify

- **Live site.** I audited the files in this worktree served locally, not https://printmarshal.app; I did not compare them.
- **Real hardware.** By rule, no real printer or network device was contacted. All console screens show one `pm-sim` device,
  so multi-device states (non-compliant fleets, many alerts, long tables, the fleet compliance trend) were not seen live; the
  site's captures show some of them but are pre-rebrand.
- **SNMPv1/v2c unknown (S1).** I observed it on this build and fixture via the UI and `/api/devices/{id}/security`; I did not
  diagnose whether it is a regression or a fixture change.
- **Screen readers and zoom.** No VoiceOver or NVDA pass; no 200 % zoom or text-spacing test; colour-blind simulation not run
  (status shapes and dashed borders suggest it would hold).
- **Other console screens** (consumables, alerts, groups, catalogue, configuration, firmware, reports, jobs, credentials, and
  most Settings sections) were not captured; findings there come from code reading only.
- **Fonts offline.** The console's fallback rendering with Google Fonts blocked was inferred from the CSS stacks, not
  captured.
- **Headless Chrome CLI at 390 px.** I used DevTools device emulation (true 390 and 400 px) instead of `--window-size`; the
  numbers above are from emulation.
- **Screenshots size.** 54 PNGs, 28 MB, mostly the full-page 390 px site captures; consider dropping the 1024 px full pages
  before committing if repository size matters.

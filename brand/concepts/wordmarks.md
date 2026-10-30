# Wordmark directions — 30 October 2026 (draft)

Direction from the owner: lowercase "print" in a typewriter / monospace voice, plain Graphite on
light and Mist on dark; "Marshal" in a different, modern typeface and a different colour, since
Marshal will be the company name. No camel-case bump between the words; both words share one
baseline. Spelling stays PrintMarshal / Marshal, one L.

Three options, each as `wordmark-X.svg` (on light) and `wordmark-X-dark.svg` (on dark). The SVGs use
live text with Google Fonts imports so the type can still be tuned; before production, outline the
text (Illustrator / Inkscape "object to path") so the files render without network access.

| Option | "print" | "Marshal" | Colour for Marshal | Notes |
| --- | --- | --- | --- | --- |
| **A** | IBM Plex Mono 500, 100 px, tracking 0 | Barlow 600, 100 px, tracking −1 | Cobalt #2E54E6 (tint #8DA4F2 on dark) | Both faces are already in the brand and the UI, so nothing new to load. The quietest of the three; Barlow's narrow width keeps the lockup short. |
| **B** | JetBrains Mono 400, 90 px | Space Grotesk 600, 100 px, tracking −2 | Cobalt / tint | More "engineering": JetBrains Mono's lower x-height makes "print" read as code, Space Grotesk gives Marshal a rounder, friendlier face. Two new fonts. |
| **C** | Courier Prime 400, 120 px, tracking −3 | Sora 600, 100 px, tracking −2 | Brass #D7A442 | The most literal typewriter; Courier Prime is scaled up because its x-height is small. Brass makes Marshal warm but loses the blue that the UI and site already use for action. |

Gap between the words: the width of one "print" character cell minus 10 px, so the two words read as
one name with a visible seam, not two words in a sentence.

Header use at about 28 px: all three stay legible; A holds best at that size because Plex Mono 500 is
heavier than JetBrains Mono 400 and Courier Prime 400, which thin out.

**Recommendation: Option A.** It satisfies the brief with faces the brand already owns, it is the
most legible at header size, and it keeps Marshal in Cobalt, the colour the product uses for every
primary action, which ties the company name to the "do" colour. Option B is the alternative if a more
developer-tool feel is wanted. C is included to show the Brass route; it reads warmer but weaker next
to the UI.

The current Guard Chevron mark is still shown beside the wordmark in the site draft only as a
placeholder until a new mark is chosen from `logo-concepts-2026-10-30.md`.

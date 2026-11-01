# Plan: the console serves its own fonts

From `intent/self-hosted-fonts/intent.md`. Spec skipped: a contained build change.

## Files that change
- `web/package.json`, `web/package-lock.json`: add the `@fontsource` packages for Barlow, IBM Plex Sans
  and IBM Plex Mono. Adding them is the task, so the lockfile changes here, with its integrity hashes.
- `web/src/fonts.ts` (new), `web/src/main.tsx`: import the same weights as before, Latin and Latin
  Extended. Vite bundles the files into the embedded assets.
- `web/index.html`: removes the Google links.
- `web/public/licenses/fonts-OFL.txt` (new): the SIL Open Font License notices the fonts require.
- `internal/web/thirdparty_test.go` (new): fails if the page names a third-party host again.

## Order of work
1. Failing test, committed. 2. The packages and imports. 3. Check in the browser that every request stays
on the server and the fonts load.

## Risks
The binary grows by the size of the font files. Names in other scripts, such as Arabic or Cyrillic, use
the system font; Google's stylesheet included Cyrillic and Greek but also did not include Arabic.

## Proof
`TestPageNamesNoThirdPartyHost` fails first and passes without being edited; the browser's network
log shows no request off the server; `document.fonts` reports the three families loaded.

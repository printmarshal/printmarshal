# Plan: a searchable settings picker with plain explanations

From `intent/settings-picker/intent.md`. Spec skipped: one component whose behaviour the intent states.
The web has no test runner, so proof there is typecheck, build and screenshots.

## Files that change
- `internal/model/settings_help.go` (new): one plain sentence for every catalogue key.
- `internal/api/config.go`: `GET /api/config/catalog` returns `help`.
- `web/src/components/config/SettingPicker.tsx` (new): search across name, key, explanation and
  area, area filters, click or Enter to add, drag to the profile.
- `web/src/components/config/ProfileEditor.tsx`: uses the picker and shows the explanation on rows.
- `web/src/types-config.ts`, `web/src/styles.css`.

## Order of work
1. Failing test that every setting has an explanation and the API returns it, committed.
2. The explanations and the API, then the picker.

## Risks
The explanations must be true across vendors, so they say what a setting means and not how a given
printer behaves. Dragging is a mouse convenience: every action also works by click and keyboard.
The security policy editor has a similar dropdown and is left for a later change.

## Proof
`TestEverySettingHasHelp` and `TestConfigCatalogReturnsHelp` fail first and pass without being edited.
Screenshots of searching, adding by click and adding by drag.

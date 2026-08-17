# ViPlay agent guide

This file applies to the entire repository. Keep it concise and store details in the linked files.

## Session start

1. Read `.agents/CONTEXT.md` once.
2. Read `.agents/DECISIONS.md` only when an architecture, media-processing, dependency, or localization decision is needed.
3. Search relevant files with `rg`; do not repeatedly scan the entire repository.
4. Preserve existing user changes and do not touch unrelated files.

## Working rules

- The user may communicate in Turkish, but all Markdown additions, code comments, identifiers, and developer-facing documentation must be written in English.
- User-facing UI text and messages must never be hardcoded in components or backend operations. Add every feature with both English (`en`) and Turkish (`tr`) entries in `locales/catalogs.json`.
- `locales/catalogs.json` is the only localization source. English is the default and fallback locale; adding another language requires only catalog metadata and messages there.
- Implement change requests with reasonable assumptions; ask only when a material product decision is missing.
- For diagnosis-only requests, do not edit code unless the user also requests a fix.
- Do not repeat the same information across comments, documentation, and responses.
- Use sub-agents only when explicitly requested by the user.
- Update `.agents/DECISIONS.md` in the same task when a durable technical decision changes.
- Update `.agents/CONTEXT.md` when the file map or validation commands change.

## Stable product decisions

- FFmpeg/ffprobe are system dependencies and are not bundled with the application.
- Video splitting uses FFmpeg stream-copy at the nearest keyframe.
- Contact sheets use FFmpeg; if `drawtext` is unavailable, timestamps are omitted without failing the operation.
- Long user-facing errors wrap, scroll, and remain readable.
- The pure Go H.264 decoder remains only for lightweight thumbnail previews.
- The complete localization policy is recorded in `.agents/DECISIONS.md`.

## Required validation

After every completed code change, run tests and the full application build:

```bash
env GOCACHE=/private/tmp/viplay-go-build go test ./...
npm --prefix frontend run check:i18n
env GOCACHE=/private/tmp/viplay-go-build PATH=/Users/ct/go/bin:/Users/ct/.local/bin:/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin /Users/ct/go/bin/wails3 task build
git diff --check
```

- macOS linker-version warnings are known environment warnings when tests/builds pass.
- When FFmpeg behavior changes, run the synthetic-media integration test with the system FFmpeg.
- When frontend behavior changes, verify the rendered UI when tooling is available; otherwise disclose the limitation.
- Audit shared-catalog key parity and search for user-facing hardcoded strings when UI copy changes.

## Delivery

In the final response, report only the outcome, changed behavior, validation status, and any real remaining risk.

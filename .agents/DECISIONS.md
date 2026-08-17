# ViPlay durable decisions

Update this file only when a decision changes. New entry format: `DNN · YYYY-MM-DD · decision — rationale`.

## Active decisions

- **D01 · 2026-08-17 · FFmpeg is not bundled.** Installation and updates remain managed by the user/system; the app searches `PATH`, plus `/opt/homebrew/bin` and `/usr/local/bin` on macOS.
- **D02 · 2026-08-17 · Split uses stream-copy.** This avoids quality loss and long encoding times; the split may move to the nearest suitable keyframe.
- **D03 · 2026-08-17 · Split output keeps the source container extension.** Both parts are written to a temporary directory beside the source and moved to final names after success.
- **D04 · 2026-08-17 · Contact sheets use an FFmpeg filter chain.** `ffprobe` supplies duration; `fps`, `scale`, `pad`, and `tile` are the required filters.
- **D05 · 2026-08-17 · `drawtext` is optional.** Some FFmpeg distributions omit it; timestamps are skipped and the contact sheet still succeeds.
- **D06 · 2026-08-17 · FFmpeg errors must be actionable.** Missing dependencies include platform install commands; operation stderr is capped at 800 characters and shown in a wrapping UI surface.
- **D07 · 2026-08-17 · The pure Go decoder remains only for thumbnails.** User-triggered split and contact-sheet operations belong to FFmpeg.
- **D08 · 2026-08-17 · Tests and a Wails build are required after every completed code change.** Sandbox runs use `/private/tmp/viplay-go-build` as the Go cache.
- **D09 · 2026-08-17 · English is the default and fallback application locale.** The initial release supports English and Turkish, and the selected locale persists between sessions.
- **D10 · 2026-08-17 · Every feature must ship with English and Turkish UI copy.** User-facing strings live in locale catalogs, never inline in components or operations. Additional languages are added through a catalog and locale-list entry.
- **D11 · 2026-08-17 · Repository prose is English-only.** Markdown, code comments, identifiers, and developer documentation remain English even when user instructions are Turkish; Turkish is valid inside translation catalogs.

## Superseding a decision

Do not delete an old entry. Mark it with `Superseded by: DNN` and add the replacement as a new numbered decision so future sessions retain the rationale.


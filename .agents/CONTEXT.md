# ViPlay quick context

## Stack

- Go 1.25 and Wails v3 beta.8
- Vue 3, Vite, and Lucide
- Desktop targets: macOS, Windows, and Linux
- External runtime tools: `ffmpeg` and `ffprobe`
- Built-in locales: English (`en`, default/fallback) and Turkish (`tr`)

## File map

- `app.go`: Wails API models, locale state, native dialogs, and user operations
- `i18n.go`: backend English/Turkish message catalogs and locale fallback
- `main.go`: application startup and authorised local media server
- `media_analysis.go`: MP4 analysis, thumbnails, and recent-store logic
- `media_tools.go`: FFmpeg split/contact-sheet logic and H.264 thumbnail decoder
- `frontend/src/App.vue`: application state, user flows, and template
- `frontend/src/i18n.js`: frontend locale list, catalogs, interpolation, and persistence
- `frontend/scripts/check-i18n.mjs`: locale-list, key-parity, and non-empty-value validation
- `frontend/src/styles.css`: visual styling, notifications, and language selector
- `main_test.go`: backend, localization, FFmpeg errors, and media integration tests
- `Taskfile.yml`, `build/`: Wails build/package tasks

## Fast routing

- Split or contact sheet: inspect `media_tools.go`, then the related `App.vue` handler.
- Backend/native copy: update both catalogs in `i18n.go`.
- Frontend copy: update both catalogs in `frontend/src/i18n.js`; do not hardcode it in components.
- Notifications: inspect `notify()` and `.operation-notice`.
- Media authorization: preserve `mediaServer.isAllowed` checks for new file operations.
- Outputs are created beside the source with collision-free names and moved atomically from temporary files.
- Run FFmpeg with `exec.Command` arguments, never with a shell command string.

## Current behavior

- The selected locale is saved in local storage and synchronized to the Wails backend.
- Unknown locale codes fall back to English.
- Split does not re-encode; `ffprobe` measures the actual split time from the first part.
- Contact sheets distribute frames evenly into a four-column JPEG grid.
- FFmpeg filters are detected at runtime; `drawtext`, which may be absent in Homebrew FFmpeg, is optional.
- Missing FFmpeg errors contain platform-specific installation commands in the active language.
- Error notifications remain for 15 seconds; details wrap, scroll, and can be selected.

## Working tree

The repository may be dirty. Check `git status --short` and never revert user changes. Build outputs are generated under `bin/` and `frontend/dist/`.

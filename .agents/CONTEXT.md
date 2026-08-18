# ViPlay quick context

## Stack

- Go 1.25 and Wails v3 beta.8
- Vue 3, Vite, and Lucide
- Desktop targets: macOS, Windows, and Linux
- External runtime tools: `ffmpeg` and `ffprobe`
- Built-in locales: English (`en`, default/fallback) and Turkish (`tr`)

## File map

- `app.go`: Wails API models, locale state, localization endpoint, native dialogs, and user operations
- `locales/catalogs.json`: the only English/Turkish localization source for frontend and backend
- `i18n.go`: embedded catalog loading, validation, locale fallback, and payload creation
- `main.go`: application startup and authorised local media server
- `media_analysis.go`: MP4 analysis, thumbnails, and recent-store logic
- `thumbnail_jobs.go`: sequential thumbnail generation state, pause/stop controls, and cache cleanup
- `media_tools.go`: FFmpeg split/contact-sheet/transcode logic and H.264 thumbnail decoder
- `frontend/src/App.vue`: application state, user flows, and template
- `frontend/src/i18n.js`: dynamic backend catalog loading, interpolation, and preference persistence
- `frontend/scripts/check-i18n.mjs`: embedded JSON key-parity, usage, and non-empty-value validation
- `frontend/src/styles.css`: visual styling, notifications, and language selector
- `main_test.go`: backend, localization, FFmpeg errors, and media integration tests
- `Taskfile.yml`, `build/`: Wails build/package tasks

## Fast routing

- Split or contact sheet: inspect `media_tools.go`, then the related `App.vue` handler.
- All user-facing copy: update English and Turkish entries in `locales/catalogs.json`; do not hardcode it in components or Go operations.
- Notifications: inspect `notify()` and `.operation-notice`.
- Media authorization: preserve `mediaServer.isAllowed` checks for new file operations.
- Outputs are created beside the source with collision-free names and moved atomically from temporary files.
- Run FFmpeg with `exec.Command` arguments, never with a shell command string.

## Current behavior

- Vue requests the selected catalog from the Wails backend before mounting; the same call activates the backend locale.
- The selected locale is saved in local storage, while catalogs remain embedded only in the Go binary.
- Unknown locale codes fall back to English.
- Split does not re-encode; `ffprobe` measures the actual split time from the first part.
- Contact sheets distribute frames evenly into a four-column JPEG grid.
- Re-encoding recommends available HEVC/AV1 software encoders, writes Matroska, validates codec/duration/size, then removes the source.
- Playlist thumbnails are generated sequentially with progress, cooperative pause/stop controls, and user-triggered cache cleanup.
- FFmpeg filters are detected at runtime; `drawtext`, which may be absent in Homebrew FFmpeg, is optional.
- Missing FFmpeg errors contain platform-specific installation commands in the active language.
- Error notifications remain for 15 seconds; details wrap, scroll, and can be selected.

## Working tree

The repository may be dirty. Check `git status --short` and never revert user changes. Build outputs are generated under `bin/` and `frontend/dist/`.

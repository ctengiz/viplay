# ViPlay

ViPlay is a cross-platform desktop video player built with Go, Wails v3, and Vue 3 for macOS, Linux, and Windows.

## Requirements

- Go 1.24+
- Node.js 20+
- Wails platform dependencies: https://wails.io/docs/gettingstarted/installation
- System-installed `ffmpeg` and `ffprobe` for video splitting and contact sheets

FFmpeg is not bundled with the application. Install it with `brew install ffmpeg` on macOS, `winget install Gyan.FFmpeg` on Windows, or the distribution package manager on Linux. Restart ViPlay after installation.

## Development

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
cd frontend && npm install && cd ..
wails3 dev
```

## Build

For the active operating system:

```bash
wails3 task package
```

Distribution artifacts are written under `bin`. Windows builds must run on Windows, macOS application bundles on macOS, and Linux packages on Linux; the CI matrix covers all three platforms.

## Localization

English is the default and fallback language. Turkish is included and can be selected from the application header; the choice persists between sessions.

- Frontend messages: `frontend/src/i18n.js`
- Backend errors and native dialog text: `i18n.go`

Every user-facing feature must include matching English and Turkish translation keys. Add another language by adding its catalog and locale-list entry; application components should not require changes.

## Features

- Multi-file local playback queue
- Play/pause, seeking, previous/next navigation, playback speed, volume, and fullscreen
- WebVTT subtitle loading
- Keyboard shortcuts: `Space`, `←`, `→`, `F`, `M`, `⌘←` / `⌘→` for folder navigation, and `⌘⌫` for disk deletion
- Lossless FFmpeg stream-copy splitting at keyframes
- FFmpeg contact-sheet generation
- Video/audio codec, container, dimensions, FPS, and file-size information
- Authorized local media streaming with HTTP range requests
- Persistent English and Turkish UI

Codec playback support depends on the operating system WebView media engine. H.264/AAC in MP4 provides the broadest common compatibility.

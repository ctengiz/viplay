# Third-party notices

ViPlay is licensed under the GNU General Public License, version 3 or (at your option) any later version (`GPL-3.0-or-later`); see `LICENSE`. The components below are distributed with or compiled into ViPlay under their own licenses, all of which are compatible with GPL-3.0-or-later.

## Compiled into the application binary

| Component | License |
|---|---|
| [Wails v3](https://github.com/wailsapp/wails) | MIT |
| [mp4ff](https://github.com/Eyevinn/mp4ff) | MIT |
| [goh264](https://github.com/thesyncim/goh264) | LGPL-2.1 |
| [xdg](https://github.com/adrg/xdg) | MIT |
| [Vue](https://github.com/vuejs/core) | MIT |
| [Lucide](https://github.com/lucide-icons/lucide) | ISC |
| [@wailsio/runtime](https://github.com/wailsapp/wails) | MIT |

## Bundled media components (macOS packages)

macOS packages include [mpv](https://mpv.io) (`libmpv`, GPL-2.0-or-later), [FFmpeg](https://ffmpeg.org) (`ffmpeg`, `ffprobe`, and its libraries, built with `--enable-gpl --enable-version3`, GPL-3.0-or-later), and the libraries they depend on, including x264 and x265 (GPL-2.0-or-later). The binaries come from Homebrew and are copied by `build/darwin/bundle_media.py`.

Each package lists every bundled component with its exact version, SPDX license, and upstream source archive in `ViPlay.app/Contents/Resources/licenses/bundled-components.json`, next to the license files and SPDX bills of materials of each component.

## Corresponding source

The source code of ViPlay is published at the repository this file belongs to. For the bundled components, every GitHub release that contains a macOS package also carries their corresponding source: the exact upstream archive of each bundled version together with the Homebrew formula (`<name>.rb`) that holds its build recipe and patches. CI collects them with `build/darwin/fetch_sources.py`, which refuses to run if Homebrew's source version differs from the bundled one. On request through the repository's issue tracker, the maintainers also provide that corresponding source for at least three years after the release that contained it.

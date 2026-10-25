package main

import (
	"errors"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// playlistMenuName matches the --custom-contextmenu value on playlist rows.
const playlistMenuName = "playlist-item"

// buildPlaylistMenu (re)creates the native row menu in the active language; clicks are forwarded to the
// frontend with the row's encoded path, which owns playback and deletion state.
func (a *App) buildPlaylistMenu() {
	if a.application == nil {
		return
	}
	menu := application.NewContextMenu(playlistMenuName)
	add := func(labelKey, action string) {
		menu.Add(a.tr(labelKey)).OnClick(func(ctx *application.Context) {
			a.application.Event.Emit("playlist:action", map[string]string{"action": action, "data": ctx.ContextMenuData()})
		})
	}
	add("playlistMenu.play", "play")
	if runtime.GOOS == "darwin" {
		add("playlistMenu.revealFinder", "reveal")
	} else {
		add("playlistMenu.reveal", "reveal")
	}
	add("pathCopy.action", "copy")
	menu.AddSeparator()
	add("playlistMenu.delete", "delete")
	menu.Update()
}

// RevealInFolder opens the system file manager with the file selected.
func (a *App) RevealInFolder(path string) error {
	if !a.server.isAllowed(path) {
		return errors.New(a.tr("error.unauthorised"))
	}
	if a.application == nil {
		return errors.New("application unavailable")
	}
	return a.application.Env.OpenFileManager(path, true)
}

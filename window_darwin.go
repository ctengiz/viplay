//go:build darwin

package main

/*
#include <stdbool.h>
void viplaySetTitleVisible(void *nsWindow, bool visible);
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// showTitleInFullscreen reveals the window title (the playing file) in the fullscreen menu-bar overlay;
// in windowed mode the title stays hidden because the custom top bar already shows it.
func showTitleInFullscreen(win *application.WebviewWindow) {
	setVisible := func(visible bool) {
		application.InvokeSync(func() { C.viplaySetTitleVisible(win.NativeWindow(), C.bool(visible)) })
	}
	win.OnWindowEvent(events.Mac.WindowWillEnterFullScreen, func(*application.WindowEvent) { setVisible(true) })
	win.OnWindowEvent(events.Mac.WindowWillExitFullScreen, func(*application.WindowEvent) { setVisible(false) })
}

//go:build darwin

package main

/*
#cgo CFLAGS: -I/opt/homebrew/include -I/usr/local/include -DGL_SILENCE_DEPRECATION
#cgo LDFLAGS: -L/opt/homebrew/lib -L/usr/local/lib -lmpv -framework Cocoa -framework OpenGL -Wl,-headerpad_max_install_names
#include <stdlib.h>
#include <mpv/client.h>

void viplayAttachMpvView(void *nsWindow, mpv_handle *mpv);

static mpv_event_property *viplayEventProperty(mpv_event *event) { return (mpv_event_property *)event->data; }
static mpv_event_end_file *viplayEventEndFile(mpv_event *event) { return (mpv_event_end_file *)event->data; }
*/
import "C"

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// nativePlayer drives libmpv; the video view is attached beneath the webview of the main window.
type nativePlayer struct {
	handle   *C.mpv_handle
	emit     func(PlayerEvent)
	attached sync.Once
}

func newNativePlayer(emit func(PlayerEvent)) (*nativePlayer, error) {
	handle := C.mpv_create()
	if handle == nil {
		return nil, errors.New("mpv_create failed")
	}
	player := &nativePlayer{handle: handle, emit: emit}
	for _, option := range [][2]string{
		{"vo", "libmpv"},
		{"hwdec", "auto-safe"},
		{"keep-open", "yes"},
		{"idle", "yes"},
		{"osd-level", "0"},
		{"input-default-bindings", "no"},
		{"input-vo-keyboard", "no"},
		{"terminal", "no"},
		{"config", "no"},
		{"audio-client-name", "ViPlay"},
	} {
		if err := player.setOption(option[0], option[1]); err != nil {
			C.mpv_terminate_destroy(handle)
			return nil, err
		}
	}
	if code := C.mpv_initialize(handle); code < 0 {
		C.mpv_terminate_destroy(handle)
		return nil, mpvError(code)
	}
	for _, property := range []struct {
		name   string
		format C.mpv_format
	}{
		{"time-pos", C.MPV_FORMAT_DOUBLE},
		{"duration", C.MPV_FORMAT_DOUBLE},
		{"pause", C.MPV_FORMAT_FLAG},
		{"eof-reached", C.MPV_FORMAT_FLAG},
		{"dwidth", C.MPV_FORMAT_INT64},
		{"dheight", C.MPV_FORMAT_INT64},
	} {
		name := C.CString(property.name)
		C.mpv_observe_property(handle, 0, name, property.format)
		C.free(unsafe.Pointer(name))
	}
	go player.eventLoop()
	return player, nil
}

func mpvError(code C.int) error {
	return fmt.Errorf("mpv: %s", C.GoString(C.mpv_error_string(code)))
}

func (p *nativePlayer) setOption(name, value string) error {
	cName, cValue := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cValue))
	if code := C.mpv_set_option_string(p.handle, cName, cValue); code < 0 {
		return fmt.Errorf("%s=%s: %w", name, value, mpvError(code))
	}
	return nil
}

func (p *nativePlayer) setProperty(name, value string) error {
	cName, cValue := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cValue))
	if code := C.mpv_set_property_string(p.handle, cName, cValue); code < 0 {
		return fmt.Errorf("%s=%s: %w", name, value, mpvError(code))
	}
	return nil
}

// command passes arguments as an argv array, so paths never need quoting.
func (p *nativePlayer) command(args ...string) error {
	argv := make([]*C.char, len(args)+1)
	for i, arg := range args {
		argv[i] = C.CString(arg)
		defer C.free(unsafe.Pointer(argv[i]))
	}
	if code := C.mpv_command(p.handle, &argv[0]); code < 0 {
		return fmt.Errorf("%s: %w", args[0], mpvError(code))
	}
	return nil
}

// attach must run on the main thread.
func (p *nativePlayer) attach(nsWindow unsafe.Pointer) {
	if nsWindow == nil {
		return
	}
	p.attached.Do(func() { C.viplayAttachMpvView(nsWindow, p.handle) })
}

func (p *nativePlayer) eventLoop() {
	var lastTime time.Time
	for {
		event := C.mpv_wait_event(p.handle, -1)
		switch event.event_id {
		case C.MPV_EVENT_SHUTDOWN:
			return
		case C.MPV_EVENT_FILE_LOADED:
			p.emit(PlayerEvent{Type: "loaded"})
		case C.MPV_EVENT_END_FILE:
			endFile := C.viplayEventEndFile(event)
			if endFile.reason == C.MPV_END_FILE_REASON_ERROR {
				p.emit(PlayerEvent{Type: "error", Message: C.GoString(C.mpv_error_string(endFile.error))})
			}
		case C.MPV_EVENT_PROPERTY_CHANGE:
			property := C.viplayEventProperty(event)
			if property.data == nil {
				continue
			}
			name := C.GoString(property.name)
			switch property.format {
			case C.MPV_FORMAT_DOUBLE:
				value := float64(*(*C.double)(property.data))
				if name == "time-pos" {
					// Matches the cadence of HTML timeupdate events without flooding the IPC bridge.
					if time.Since(lastTime) < 100*time.Millisecond {
						continue
					}
					lastTime = time.Now()
				}
				p.emit(PlayerEvent{Type: name, Value: value})
			case C.MPV_FORMAT_FLAG:
				p.emit(PlayerEvent{Type: name, Flag: *(*C.int)(property.data) != 0})
			case C.MPV_FORMAT_INT64:
				p.emit(PlayerEvent{Type: name, Value: float64(*(*C.int64_t)(property.data))})
			}
		}
	}
}

func (p *nativePlayer) load(path string, play bool) error {
	if err := p.setProperty("pause", mpvFlag(!play)); err != nil {
		return err
	}
	return p.command("loadfile", path, "replace")
}

func (p *nativePlayer) stop() error { return p.command("stop") }

func (p *nativePlayer) seek(seconds float64) error {
	return p.command("seek", strconv.FormatFloat(seconds, 'f', 3, 64), "absolute")
}

func (p *nativePlayer) setSubtitle(path string) error {
	if path == "" {
		return p.setProperty("sid", "no")
	}
	return p.command("sub-add", path, "select")
}

func (a *App) attachPlayer() {
	application.InvokeSync(func() { a.player.attach(a.window.NativeWindow()) })
}

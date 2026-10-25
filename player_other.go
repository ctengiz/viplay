//go:build !darwin

package main

import "errors"

// shortcut: libmpv is wired up only on macOS; Windows and Linux keep HTML5 playback with FFmpeg proxies until their native views exist.
type nativePlayer struct{}

var errNativePlayerUnavailable = errors.New("native player is not available on this platform")

func newNativePlayer(func(PlayerEvent)) (*nativePlayer, error) {
	return nil, errNativePlayerUnavailable
}

func (*nativePlayer) load(string, bool) error          { return errNativePlayerUnavailable }
func (*nativePlayer) stop() error                      { return errNativePlayerUnavailable }
func (*nativePlayer) seek(float64) error               { return errNativePlayerUnavailable }
func (*nativePlayer) setSubtitle(string) error         { return errNativePlayerUnavailable }
func (*nativePlayer) setProperty(string, string) error { return errNativePlayerUnavailable }

func (a *App) attachPlayer() {}

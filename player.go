package main

import (
	"errors"
	"math"
	"strconv"
)

// PlayerEvent mirrors the HTML media events the frontend consumes from the native engine.
type PlayerEvent struct {
	Type    string  `json:"type"`
	Value   float64 `json:"value"`
	Flag    bool    `json:"flag"`
	Message string  `json:"message"`
}

func mpvFlag(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// PlayerEngine reports "mpv" when the native engine is available and attaches its view; otherwise "html5".
func (a *App) PlayerEngine() string {
	if a.player == nil || a.window == nil {
		return "html5"
	}
	a.attachPlayer()
	return "mpv"
}

func (a *App) nativePlayer() (*nativePlayer, error) {
	if a.player == nil {
		return nil, errors.New("native player unavailable")
	}
	return a.player, nil
}

func (a *App) PlayerLoad(path string, play bool) error {
	if !a.server.isAllowed(path) {
		return errors.New(a.tr("error.unauthorised"))
	}
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.load(path, play)
}

func (a *App) PlayerStop() error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.stop()
}

func (a *App) PlayerSetPause(paused bool) error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.setProperty("pause", mpvFlag(paused))
}

func (a *App) PlayerSeek(seconds float64) error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.seek(math.Max(0, seconds))
}

func (a *App) PlayerSetVolume(volume float64) error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.setProperty("volume", strconv.FormatFloat(math.Min(1, math.Max(0, volume))*100, 'f', 1, 64))
}

func (a *App) PlayerSetMuted(muted bool) error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.setProperty("mute", mpvFlag(muted))
}

func (a *App) PlayerSetSpeed(speed float64) error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.setProperty("speed", strconv.FormatFloat(math.Min(4, math.Max(.25, speed)), 'f', 2, 64))
}

func (a *App) PlayerSetSubtitle(path string) error {
	if path != "" && !a.server.isAllowed(path) {
		return errors.New(a.tr("error.unauthorised"))
	}
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	return player.setSubtitle(path)
}

// PlayerSetVideoArea confines the picture to the stage element; values are margin ratios of the window.
func (a *App) PlayerSetVideoArea(left, top, right, bottom float64) error {
	player, err := a.nativePlayer()
	if err != nil {
		return err
	}
	for name, value := range map[string]float64{"left": left, "top": top, "right": right, "bottom": bottom} {
		if err := player.setProperty("video-margin-ratio-"+name, strconv.FormatFloat(math.Min(1, math.Max(0, value)), 'f', 4, 64)); err != nil {
			return err
		}
	}
	return nil
}
